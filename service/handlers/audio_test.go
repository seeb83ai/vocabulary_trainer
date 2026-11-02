package handlers_test

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"vocabulary_trainer/handlers"

	"github.com/go-chi/chi/v5"
)

// TestAudio_NotImmutable verifies that the audio handler does NOT mark
// responses as immutable. Marking them so means a regenerated MP3
// (e.g. after a zh_text edit) is served stale for up to a year. The
// existing handler set this and was flagged in the security review.
func TestAudio_NotImmutable(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	tmpDir := t.TempDir()
	audioH := &handlers.AudioHandler{Store: s, AudioDir: tmpDir}
	if err := os.WriteFile(tmpDir+"/"+fmt.Sprint(id)+".mp3", []byte("fake-mp3"), 0644); err != nil {
		t.Fatalf("seed mp3: %v", err)
	}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/{id}", audioH.ServeAudio)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/audio/%d", id), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	cc := rec.Header().Get("Cache-Control")
	if cc == "" {
		t.Fatal("Cache-Control header should be set")
	}
	if strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control must not include 'immutable' (causes stale audio after regeneration): %q", cc)
	}
	if !strings.Contains(cc, "must-revalidate") && !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control should require revalidation (must-revalidate or no-cache): %q", cc)
	}
}

// TestServeAudio_OtherUserForbidden verifies that the cached-audio path enforces
// word ownership: a user must not be able to fetch a cached <id>.mp3 for a word
// they do not own (IDOR). The ownership check must run BEFORE serving the file.
func TestServeAudio_OtherUserForbidden(t *testing.T) {
	s := openTestDB(t)
	// Word belongs to user 2 (seedWord default owner).
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	tmpDir := t.TempDir()
	audioH := &handlers.AudioHandler{Store: s, AudioDir: tmpDir}
	// Pre-seed a cached MP3 so the lazy-generation branch is skipped.
	if err := os.WriteFile(tmpDir+"/"+fmt.Sprint(id)+".mp3", []byte("fake-mp3"), 0644); err != nil {
		t.Fatalf("seed mp3: %v", err)
	}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(999)) // a different user
	r.Get("/api/audio/{id}", audioH.ServeAudio)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/audio/%d", id), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for non-owner, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "fake-mp3") {
		t.Errorf("must not serve cached audio to non-owner")
	}
}

// TestGenerateAsync_LogsErrorOnFailure asserts the fire-and-forget TTS helper
// logs a failure with word-ID context instead of swallowing the error.
func TestGenerateAsync_LogsErrorOnFailure(t *testing.T) {
	// Force generate() to fail deterministically: point AudioDir at a path whose
	// parent is a regular file, so os.MkdirAll cannot create the directory.
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	audioH := &handlers.AudioHandler{AudioDir: filepath.Join(blocker, "audio")}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	audioH.GenerateAsync(4242, "你好")

	if !strings.Contains(buf.String(), "async tts generate word 4242") {
		t.Fatalf("expected async tts error log with word id, got: %q", buf.String())
	}
}

// TestServeAudio_TTSFailureLogged verifies that a TTS synthesis failure is
// logged with word-ID context (and surfaced as 503) instead of being swallowed.
func TestServeAudio_TTSFailureLogged(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	tmpDir := t.TempDir()
	audioH := &handlers.AudioHandler{
		Store:    s,
		AudioDir: tmpDir,
		Synth:    func(string) ([]byte, error) { return nil, fmt.Errorf("tts boom") },
	}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/{id}", audioH.ServeAudio)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/audio/%d", id), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
	logged := buf.String()
	if !strings.Contains(logged, fmt.Sprint(id)) || !strings.Contains(logged, "tts boom") {
		t.Errorf("expected TTS failure logged with word-ID context, got: %q", logged)
	}
}

func TestServeComponentAudio_ServesPreCachedFile(t *testing.T) {
	tmpDir := t.TempDir()
	audioH := &handlers.AudioHandler{Store: openTestDB(t), AudioDir: tmpDir}

	// Pre-seed the cached file using the expected c_{hex}.mp3 naming pattern.
	// 木 = U+6728
	cachedPath := filepath.Join(tmpDir, "c_6728.mp3")
	if err := os.WriteFile(cachedPath, []byte("fake-mp3-wood"), 0644); err != nil {
		t.Fatalf("seed mp3: %v", err)
	}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/component/{char}", audioH.ServeComponentAudio)

	req := httptest.NewRequest("GET", "/api/audio/component/"+url.PathEscape("木"), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "fake-mp3-wood") {
		t.Errorf("want cached content, got %q", rec.Body.String())
	}
}

func TestServeComponentAudio_GeneratesOnDemand(t *testing.T) {
	tmpDir := t.TempDir()
	synthCalled := ""
	audioH := &handlers.AudioHandler{
		Store:    openTestDB(t),
		AudioDir: tmpDir,
		Synth:    func(text string) ([]byte, error) { synthCalled = text; return []byte("synth-mp3"), nil },
	}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/component/{char}", audioH.ServeComponentAudio)

	req := httptest.NewRequest("GET", "/api/audio/component/"+url.PathEscape("女"), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if synthCalled != "女" {
		t.Errorf("want synth called with 女, got %q", synthCalled)
	}
	// File must be written with c_{hex}.mp3 pattern (女 = U+5973).
	if _, err := os.Stat(filepath.Join(tmpDir, "c_5973.mp3")); err != nil {
		t.Errorf("expected c_5973.mp3 to exist after generation: %v", err)
	}
}

func TestServeComponentAudio_InvalidChar(t *testing.T) {
	audioH := &handlers.AudioHandler{Store: openTestDB(t), AudioDir: t.TempDir()}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/component/{char}", audioH.ServeComponentAudio)

	// Multi-character value must be rejected with 400.
	req := httptest.NewRequest("GET", "/api/audio/component/"+url.PathEscape("木火"), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("multi-char: want 400, got %d", rec.Code)
	}
}

func TestServeComponentAudio_FilenameDifferentFromWordIDs(t *testing.T) {
	// Ensure component files (c_{hex}.mp3) cannot collide with word audio files
	// ({integer}.mp3). A hex codepoint like "6728" must NOT be a valid word ID
	// file — verified by checking the naming prefix distinguishes them.
	wordFile := "42.mp3"
	componentFile := fmt.Sprintf("c_%04x.mp3", []rune("木")[0]) // c_6728.mp3
	if wordFile == componentFile {
		t.Error("component filename pattern must not match word id filename pattern")
	}
	if !strings.HasPrefix(componentFile, "c_") {
		t.Error("component filename must start with c_")
	}
}

func TestServeComponentAudio_RadicalUsesCanonicalFormForTTS(t *testing.T) {
	// Radical variant characters (e.g. 扌 U+624C) should have TTS generated
	// using the canonical/pronounceable character (手 U+624B), while the cached
	// file is still named after the actual component codepoint (c_624c.mp3).
	cases := []struct {
		radical   string // the radical variant shown in the quiz
		canonical string // what TTS should receive
		wantFile  string // expected cached filename
	}{
		{"扌", "手", "c_624c.mp3"}, // hand radical
		{"氵", "水", "c_6c35.mp3"}, // water (3-dot)
		{"亻", "人", "c_4ebb.mp3"}, // person radical
		{"讠", "言", "c_8ba0.mp3"}, // speech radical
	}

	for _, tc := range cases {
		t.Run(tc.radical, func(t *testing.T) {
			tmpDir := t.TempDir()
			var synthGot string
			audioH := &handlers.AudioHandler{
				Store:    openTestDB(t),
				AudioDir: tmpDir,
				Synth:    func(text string) ([]byte, error) { synthGot = text; return []byte("synth-mp3"), nil },
			}

			r := chi.NewRouter()
			r.Use(handlers.WithUserID(2))
			r.Get("/api/audio/component/{char}", audioH.ServeComponentAudio)

			req := httptest.NewRequest("GET", "/api/audio/component/"+url.PathEscape(tc.radical), nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if synthGot != tc.canonical {
				t.Errorf("TTS called with %q, want canonical %q", synthGot, tc.canonical)
			}
			if _, err := os.Stat(filepath.Join(tmpDir, tc.wantFile)); err != nil {
				t.Errorf("expected %s to exist after generation: %v", tc.wantFile, err)
			}
		})
	}
}

// serveTextAudio sends GET /api/audio/text?zh=<zh> to the handler (issue #638).
func serveTextAudio(t *testing.T, audioH *handlers.AudioHandler, zh string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/text", audioH.ServeTextAudio)
	req := httptest.NewRequest("GET", "/api/audio/text?zh="+url.QueryEscape(zh), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestServeTextAudio_GeneratesOnceAndCaches(t *testing.T) {
	var synthCalls []string
	audioH := &handlers.AudioHandler{
		Store:    openTestDB(t),
		AudioDir: t.TempDir(),
		Synth: func(text string) ([]byte, error) {
			synthCalls = append(synthCalls, text)
			return []byte("synth-mp3"), nil
		},
	}

	for i := 0; i < 2; i++ {
		rec := serveTextAudio(t, audioH, "这个")
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d: %s", i, rec.Code, rec.Body.String())
		}
		if rec.Body.String() != "synth-mp3" {
			t.Errorf("request %d: want synthesised audio, got %q", i, rec.Body.String())
		}
	}
	if len(synthCalls) != 1 || synthCalls[0] != "这个" {
		t.Errorf("want one synth call with 这个, got %q", synthCalls)
	}
}

func TestServeTextAudio_SentenceWithPunctuation(t *testing.T) {
	var synthGot string
	audioH := &handlers.AudioHandler{
		Store:    openTestDB(t),
		AudioDir: t.TempDir(),
		Synth:    func(text string) ([]byte, error) { synthGot = text; return []byte("synth-mp3"), nil },
	}
	rec := serveTextAudio(t, audioH, " 你好，我是学生。 ")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if synthGot != "你好，我是学生。" {
		t.Errorf("want trimmed sentence sent to TTS, got %q", synthGot)
	}
}

func TestServeTextAudio_RejectsNonChineseText(t *testing.T) {
	cases := map[string]string{
		"empty":      "",
		"latin":      "abc",
		"mixed":      "这a",
		"pinyin":     "zhè",
		"only punct": "。",
		"path chars": "../这",
		"too long":   strings.Repeat("这", 41),
	}
	for name, zh := range cases {
		t.Run(name, func(t *testing.T) {
			synthCalled := false
			audioH := &handlers.AudioHandler{
				Store:    openTestDB(t),
				AudioDir: t.TempDir(),
				Synth:    func(string) ([]byte, error) { synthCalled = true; return []byte("x"), nil },
			}
			rec := serveTextAudio(t, audioH, zh)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("want 400, got %d", rec.Code)
			}
			if synthCalled {
				t.Error("TTS must not be called for rejected text")
			}
		})
	}
}

func TestServeTextAudio_TTSFailure(t *testing.T) {
	audioH := &handlers.AudioHandler{
		Store:    openTestDB(t),
		AudioDir: t.TempDir(),
		Synth:    func(string) ([]byte, error) { return nil, fmt.Errorf("tts boom") },
	}
	rec := serveTextAudio(t, audioH, "这")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", rec.Code)
	}
}
