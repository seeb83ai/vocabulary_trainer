package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"vocabulary_trainer/handlers"
)

// jsonEcho is a tiny handler that decodes a JSON object from the (size-capped)
// body and reports success or a 400 on decode failure — mirroring the pattern
// every real /api POST handler uses.
func jsonEcho(w http.ResponseWriter, r *http.Request) {
	var v map[string]any
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func TestMaxBytes_AllowsSmallBody(t *testing.T) {
	t.Parallel()
	mw := handlers.MaxBytes(64)(http.HandlerFunc(jsonEcho))
	req := httptest.NewRequest("POST", "/api/words", strings.NewReader(`{"a":1}`))
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("small body: status = %d, want 200", rec.Code)
	}
}

func TestMaxBytes_RejectsOversizedBody(t *testing.T) {
	t.Parallel()
	body := `{"a":"` + strings.Repeat("x", 4096) + `"}`
	mw := handlers.MaxBytes(64)(http.HandlerFunc(jsonEcho))
	req := httptest.NewRequest("POST", "/api/words", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: status = %d, want 400 or 413", rec.Code)
	}
}

// TestMaxBytes_LimitedReaderErrors confirms the wrapped body returns an error
// once the limit is exceeded, so downstream decoders can detect it.
func TestMaxBytes_LimitedReaderErrors(t *testing.T) {
	t.Parallel()
	var readErr error
	mw := handlers.MaxBytes(8)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/api/words", strings.NewReader(strings.Repeat("x", 100)))
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if readErr == nil {
		t.Fatal("expected read error when body exceeds limit, got nil")
	}
}
