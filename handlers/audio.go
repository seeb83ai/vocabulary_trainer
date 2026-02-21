package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"vocabulary_trainer/db"
)

type AudioHandler struct {
	Store    *db.Store
	AudioDir string // absolute path where MP3 files are stored, e.g. /data/audio
	TTSScript string // absolute path to cmd/tts/generate.py
	VenvPython string // absolute path to venv python3, e.g. /data/tts-venv/bin/python3
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
		wd, err := h.Store.GetWordByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
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

// regenerate deletes the cached file and regenerates it (used when zh_text changes).
func (h *AudioHandler) regenerate(wordID int64, zhText string) error {
	mp3Path := filepath.Join(h.AudioDir, fmt.Sprintf("%d.mp3", wordID))
	os.Remove(mp3Path) // ignore error — file may not exist yet
	return h.generate(wordID, zhText)
}

// generate calls the edge-tts Python script to produce an MP3 file.
func (h *AudioHandler) generate(wordID int64, zhText string) error {
	python := h.VenvPython
	if python == "" {
		python = "python3"
	}
	cmd := exec.Command(python, h.TTSScript,
		fmt.Sprintf("%d", wordID),
		zhText,
		h.AudioDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("tts generate word %d: %v\n%s", wordID, err, out)
		return err
	}
	return nil
}
