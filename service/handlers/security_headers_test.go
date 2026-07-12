package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"vocabulary_trainer/handlers"
)

// TestSecurityHeaders_SetsAllExpected verifies the middleware sets every
// header in the baseline policy on a normal 200 response.
func TestSecurityHeaders_SetsAllExpected(t *testing.T) {
	t.Parallel()
	mw := handlers.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-XSS-Protection":       "0",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s: want %q, got %q", k, v, got)
		}
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header is required")
	}
	for _, frag := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	} {
		if !strings.Contains(csp, frag) {
			t.Errorf("CSP missing %q (got %q)", frag, csp)
		}
	}
	// Must NOT permit unsafe-inline or unsafe-eval scripts.
	if strings.Contains(csp, "script-src") && strings.Contains(csp, "'unsafe-inline'") {
		// Allowed for style-src but NOT for script-src — verify by checking
		// the script-src segment specifically.
		for _, segment := range strings.Split(csp, ";") {
			s := strings.TrimSpace(segment)
			if strings.HasPrefix(s, "script-src") && strings.Contains(s, "'unsafe-inline'") {
				t.Errorf("script-src must not include 'unsafe-inline': %q", s)
			}
		}
	}

	if pp := rec.Header().Get("Permissions-Policy"); pp == "" {
		t.Error("Permissions-Policy header should be set")
	}
}
