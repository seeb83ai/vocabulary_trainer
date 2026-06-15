package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"vocabulary_trainer/db"
	"vocabulary_trainer/tts"
)

type AudioHandler struct {
	Store    *db.Store
	AudioDir string // absolute path where MP3 files are stored, e.g. /data/audio
	// Synth synthesises speech for the given text. Defaults to tts.Synthesize
	// when nil; overridable in tests to exercise the failure path.
	Synth func(string) ([]byte, error)
}

// ServeAudio handles GET /api/audio/{id}.
// It serves the cached MP3 for the given zh word ID, generating it on demand if missing.
func (h *AudioHandler) ServeAudio(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	// Enforce word ownership BEFORE serving any cached file. Running this check
	// only in the lazy-generation branch left an IDOR: a cached <id>.mp3 was
	// served to any authenticated user regardless of who owns the word.
	wd, err := h.Store.GetWordByID(r.Context(), UserIDFromContext(r.Context()), id)
	if err != nil {
		internalError(w, err)
		return
	}
	if wd == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}

	mp3Path := filepath.Join(h.AudioDir, fmt.Sprintf("%d.mp3", id))

	// Generate lazily if the file doesn't exist yet
	if _, err := os.Stat(mp3Path); os.IsNotExist(err) {
		if err := h.generate(id, wd.ZhText); err != nil {
			// TTS unavailable — tell the client so it can fall back
			writeError(w, http.StatusServiceUnavailable, "tts unavailable")
			return
		}
	}

	// Cache the audio for an hour, but require revalidation afterwards so a
	// regenerated MP3 (e.g. after a zh_text edit) is picked up promptly.
	// http.ServeFile populates Last-Modified, so the revalidation hits 304
	// when the file hasn't actually changed.
	w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
	http.ServeFile(w, r, mp3Path)
}

// GenerateAsync runs generate in a fire-and-forget context, logging any
// failure with word-ID context instead of silently discarding the error.
func (h *AudioHandler) GenerateAsync(wordID int64, zhText string) {
	if err := h.generate(wordID, zhText); err != nil {
		log.Printf("async tts generate word %d: %v", wordID, err)
	}
}

// RegenerateAsync runs regenerate in a fire-and-forget context, logging any
// failure with word-ID context instead of silently discarding the error.
func (h *AudioHandler) RegenerateAsync(wordID int64, zhText string) {
	if err := h.regenerate(wordID, zhText); err != nil {
		log.Printf("async tts regenerate word %d: %v", wordID, err)
	}
}

// regenerate deletes the cached file and regenerates it (used when zh_text changes).
func (h *AudioHandler) regenerate(wordID int64, zhText string) error {
	mp3Path := filepath.Join(h.AudioDir, fmt.Sprintf("%d.mp3", wordID))
	os.Remove(mp3Path) // ignore error — file may not exist yet
	return h.generate(wordID, zhText)
}

// generate calls the Edge TTS service to produce an MP3 file.
func (h *AudioHandler) generate(wordID int64, zhText string) error {
	if err := os.MkdirAll(h.AudioDir, 0755); err != nil {
		log.Printf("tts mkdir %s: %v", h.AudioDir, err)
		return err
	}
	synth := h.Synth
	if synth == nil {
		synth = tts.Synthesize
	}
	data, err := synth(zhText)
	if err != nil {
		log.Printf("tts generate word %d: %v", wordID, err)
		return err
	}
	mp3Path := filepath.Join(h.AudioDir, fmt.Sprintf("%d.mp3", wordID))
	if err := os.WriteFile(mp3Path, data, 0644); err != nil {
		log.Printf("tts write word %d: %v", wordID, err)
		return err
	}
	return nil
}
