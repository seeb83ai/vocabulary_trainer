package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"vocabulary_trainer/handlers"
)

// This file holds handlers_test-package integration tests for the /api/config
// and /api/translate endpoints (TranslateHandler.Config / Translate exercised
// over HTTP with the shared router_test.go fixtures). translate_test.go
// (package handlers) covers the unexported pure helpers instead.

// TestConfig_PlusUserSeesAvailable verifies that user 2 (plus) gets deepl_available=true.
func TestConfig_PlusUserSeesAvailable(t *testing.T) {
	s := openTestDB(t)
	r := newRouterWithUserID(s, 2)
	rec := do(t, r, http.MethodGet, "/api/config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var cfg map[string]bool
	decodeJSON(t, rec, &cfg)
	if !cfg["deepl_available"] {
		t.Error("plus user: deepl_available should be true")
	}
	if !cfg["llm_available"] {
		t.Error("plus user: llm_available should be true")
	}
}

// TestConfig_FreeUserSeesConfiguredButNotAvailable verifies free users see configured=true, available=false.
func TestConfig_FreeUserSeesConfiguredButNotAvailable(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	freeID, err := s.CreateUser(ctx, "free@example.com", "hash", "tok-free", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	r := newRouterWithUserID(s, freeID)
	rec := do(t, r, http.MethodGet, "/api/config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var cfg map[string]bool
	decodeJSON(t, rec, &cfg)
	if !cfg["deepl_configured"] {
		t.Error("free user: deepl_configured should be true (key is set)")
	}
	if cfg["deepl_available"] {
		t.Error("free user: deepl_available should be false")
	}
	if !cfg["llm_configured"] {
		t.Error("free user: llm_configured should be true")
	}
	if cfg["llm_available"] {
		t.Error("free user: llm_available should be false")
	}
}

// TestTranslate_FreeUserForbidden verifies free users cannot call the translate endpoint.
func TestTranslate_FreeUserForbidden(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	freeID, err := s.CreateUser(ctx, "free2@example.com", "hash", "tok-free2", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	r := newRouterWithUserID(s, freeID)
	rec := do(t, r, http.MethodPost, "/api/translate", map[string]string{"zh_text": "你好"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("free user translate: want 403, got %d", rec.Code)
	}
}

// TestTranslate_PinyinOnlyAllowedForFreeUser verifies that the pinyin-only path
// (both zh_text and en_text provided) is not blocked for free users.
func TestTranslate_PinyinOnlyAllowedForFreeUser(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	freeID, err := s.CreateUser(ctx, "free3@example.com", "hash", "tok-free3", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	r := newRouterWithUserID(s, freeID)
	rec := do(t, r, http.MethodPost, "/api/translate", map[string]string{
		"zh_text":     "你好",
		"source_text": "hello",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("free user pinyin-only: want 200, got %d", rec.Code)
	}
	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if resp["pinyin"] == "" {
		t.Error("expected non-empty pinyin in response")
	}
}

// TestTranslate_PlusUserAllowed verifies plus users can call translate.
func TestTranslate_PlusUserAllowed(t *testing.T) {
	s := openTestDB(t)
	// user 2 is plus; the actual DeepL call will fail (no real key),
	// so we only check that we don't get 403.
	r := newRouterWithUserID(s, 2)
	rec := do(t, r, http.MethodPost, "/api/translate", map[string]string{"zh_text": "你好"})
	if rec.Code == http.StatusForbidden {
		t.Fatal("plus user should not be forbidden from translate")
	}
}

// TestTranslate_TargetLangNotSwapped is a regression test for issue #342:
// a request with target_lang=EN must come back with English text and a
// request with target_lang=DE must come back with German text — the two
// must never be swapped. It stubs DeepL with an httptest server (wired via
// TranslateHandler.BaseURL) so the assertion doesn't depend on a real key.
func TestTranslate_TargetLangNotSwapped(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text       []string `json:"text"`
			TargetLang string   `json:"target_lang"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("mock deepl: decode request: %v", err)
		}
		text := "Tools / Instruments / Equipment"
		if body.TargetLang == "DE" {
			text = "Werkzeuge"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"translations": []map[string]string{{"text": text}},
		})
	}))
	defer mock.Close()

	s := openTestDB(t)
	authH, _ := handlers.NewAuthHandlerWithEnv(s, nil, "http://localhost:8080", "", "dev")
	settingsH := handlers.NewSettingsHandler(s, authH.Secret())
	translateH := &handlers.TranslateHandler{
		Store:           s,
		APIKey:          "test-key",
		TargetLang:      "EN",
		SettingsHandler: settingsH,
		BaseURL:         mock.URL,
	}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2)) // user 2 is plus (see router_test.go)
	r.Post("/api/translate", translateH.Translate)

	enRec := do(t, r, http.MethodPost, "/api/translate", map[string]string{"zh_text": "工具", "target_lang": "EN"})
	if enRec.Code != http.StatusOK {
		t.Fatalf("EN translate: want 200, got %d", enRec.Code)
	}
	var enResp struct {
		Translations []string `json:"translations"`
	}
	decodeJSON(t, enRec, &enResp)
	if len(enResp.Translations) == 0 || enResp.Translations[0] != "Tools" {
		t.Fatalf("EN translate: want first translation %q, got %v", "Tools", enResp.Translations)
	}

	deRec := do(t, r, http.MethodPost, "/api/translate", map[string]string{"zh_text": "工具", "target_lang": "DE"})
	if deRec.Code != http.StatusOK {
		t.Fatalf("DE translate: want 200, got %d", deRec.Code)
	}
	var deResp struct {
		Translations []string `json:"translations"`
	}
	decodeJSON(t, deRec, &deResp)
	if len(deResp.Translations) != 1 || deResp.Translations[0] != "Werkzeuge" {
		t.Fatalf("DE translate: want [%q], got %v", "Werkzeuge", deResp.Translations)
	}
}
