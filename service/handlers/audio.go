package handlers

import (
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"vocabulary_trainer/db"
	"vocabulary_trainer/tts"

	"github.com/go-chi/chi/v5"
)

type AudioHandler struct {
	Store    *db.Store
	AudioDir string // absolute path where MP3 files are stored, e.g. /data/audio
}

// ServeAudio handles GET /api/audio/{id}.
// It serves the cached MP3 for the given zh word ID, generating it on demand if missing.
func (h *AudioHandler) ServeAudio(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	mp3Path := filepath.Join(h.AudioDir, fmt.Sprintf("%d.mp3", id))

	// Generate lazily if the file doesn't exist yet
	if _, err := os.Stat(mp3Path); os.IsNotExist(err) {
		wd, err := h.Store.GetWordByID(r.Context(), UserIDFromContext(r.Context()), id)
		if err != nil {
			internalError(w, err)
			return
		}
		if wd == nil {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		if err := h.generate(id, wd.ZhText); err != nil {
			// TTS unavailable — tell the client so it can fall back
			writeError(w, http.StatusServiceUnavailable, "tts unavailable")
			return
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, mp3Path)
}

// ServeCharAudio handles GET /api/audio/char/{char}.
// It serves TTS audio for a raw Chinese text segment (used for components that have no word ID).
// The cache filename is the hex encoding of the UTF-8 bytes to ensure filesystem safety.
func (h *AudioHandler) ServeCharAudio(w http.ResponseWriter, r *http.Request) {
	zhText := chi.URLParam(r, "char")
	if zhText == "" {
		writeError(w, http.StatusBadRequest, "char is required")
		return
	}

	filename := hex.EncodeToString([]byte(zhText)) + ".mp3"
	mp3Path := filepath.Join(h.AudioDir, filename)

	if _, err := os.Stat(mp3Path); os.IsNotExist(err) {
		if err := os.MkdirAll(h.AudioDir, 0755); err != nil {
			log.Printf("tts mkdir %s: %v", h.AudioDir, err)
			writeError(w, http.StatusInternalServerError, "failed to create audio dir")
			return
		}
		data, err := tts.Synthesize(zhText)
		if err != nil {
			log.Printf("tts generate char %q: %v", zhText, err)
			writeError(w, http.StatusServiceUnavailable, "tts unavailable")
			return
		}
		if err := os.WriteFile(mp3Path, data, 0644); err != nil {
			log.Printf("tts write char %q: %v", zhText, err)
			writeError(w, http.StatusInternalServerError, "failed to write audio")
			return
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, mp3Path)
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
	data, err := tts.Synthesize(zhText)
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
