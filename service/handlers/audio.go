package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"vocabulary_trainer/tts"

	"github.com/go-chi/chi/v5"
)

type AudioHandler struct {
	Store    audioStore
	AudioDir string // absolute path where MP3 files are stored, e.g. /data/audio
	// Synth synthesises speech for the given text. Defaults to tts.Synthesize
	// when nil; overridable in tests to exercise the failure path.
	Synth func(string) ([]byte, error)
}

// ServeAudio handles GET /api/audio/{id}.
// It serves the cached MP3 for the given zh word ID, generating it on demand if missing.
// Files are stored as {word_id}.mp3 (e.g. 42.mp3).
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
		if err := h.generateToPath(mp3Path, wd.ZhText); err != nil {
			log.Printf("tts generate word %d: %v", id, err)
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

// radicalToCanonical maps radical-only Unicode codepoints (which TTS engines
// often mispronounce or cannot handle) to the standalone character they derive
// from and whose pronunciation the TTS should use instead.
var radicalToCanonical = map[rune]rune{
	'亻': '人', // person radical
	'刂': '刀', // knife radical
	'扌': '手', // hand radical
	'氵': '水', // water radical (3-dot)
	'灬': '火', // fire radical (bottom form)
	'犭': '犬', // dog radical
	'礻': '示', // spirit/altar radical
	'纟': '糸', // silk radical (simplified)
	'衤': '衣', // clothing radical
	'讠': '言', // speech radical (simplified)
	'钅': '金', // metal/gold radical (simplified)
	'饣': '食', // food radical (simplified)
}

// canonicalForTTS returns the pronounceable standalone character for TTS. If r
// is a known radical-only variant, it returns the canonical form; otherwise r.
func canonicalForTTS(r rune) rune {
	if c, ok := radicalToCanonical[r]; ok {
		return c
	}
	return r
}

// ServeComponentAudio handles GET /api/audio/component/{char}.
// It serves TTS audio for a single component character, generating on demand.
// Files are stored as c_{hex_codepoint}.mp3 (e.g. c_6728.mp3 for 木) so they
// cannot collide with word audio files which use {word_id}.mp3 (e.g. 42.mp3).
// Radical-variant characters (e.g. 扌) are synthesised using their canonical
// pronounceable form (e.g. 手) while the cached file stays keyed to the actual
// component codepoint.
func (h *AudioHandler) ServeComponentAudio(w http.ResponseWriter, r *http.Request) {
	char := strings.TrimSpace(chi.URLParam(r, "char"))
	runes := []rune(char)
	if len(runes) != 1 {
		writeError(w, http.StatusBadRequest, "char must be a single character")
		return
	}

	mp3Path := filepath.Join(h.AudioDir, fmt.Sprintf("c_%04x.mp3", runes[0]))

	if _, err := os.Stat(mp3Path); os.IsNotExist(err) {
		ttsText := string(canonicalForTTS(runes[0]))
		if err := h.generateToPath(mp3Path, ttsText); err != nil {
			writeError(w, http.StatusServiceUnavailable, "tts unavailable")
			return
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
	http.ServeFile(w, r, mp3Path)
}

// GenerateAsync runs TTS generation in a fire-and-forget context, logging any
// failure with word-ID context instead of silently discarding the error.
func (h *AudioHandler) GenerateAsync(wordID int64, zhText string) {
	mp3Path := filepath.Join(h.AudioDir, fmt.Sprintf("%d.mp3", wordID))
	if err := h.generateToPath(mp3Path, zhText); err != nil {
		log.Printf("async tts generate word %d: %v", wordID, err)
	}
}

// RegenerateAsync deletes the cached file and regenerates it in a fire-and-forget
// context (used when zh_text changes).
func (h *AudioHandler) RegenerateAsync(wordID int64, zhText string) {
	mp3Path := filepath.Join(h.AudioDir, fmt.Sprintf("%d.mp3", wordID))
	os.Remove(mp3Path) // ignore error — file may not exist yet
	if err := h.generateToPath(mp3Path, zhText); err != nil {
		log.Printf("async tts regenerate word %d: %v", wordID, err)
	}
}

// generateToPath calls the TTS service and writes the result to mp3Path.
func (h *AudioHandler) generateToPath(mp3Path, text string) error {
	if err := os.MkdirAll(h.AudioDir, 0755); err != nil {
		log.Printf("tts mkdir %s: %v", h.AudioDir, err)
		return err
	}
	synth := h.Synth
	if synth == nil {
		synth = tts.Synthesize
	}
	data, err := synth(text)
	if err != nil {
		return err
	}
	if err := os.WriteFile(mp3Path, data, 0644); err != nil {
		log.Printf("tts write %q: %v", mp3Path, err)
		return err
	}
	return nil
}
