package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"
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
