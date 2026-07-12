package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"vocabulary_trainer/handlers"
)

// TestRateLimiter_AllowsBurstThenBlocks verifies the basic token-bucket
// behaviour: requests up to the burst capacity succeed, the next one is
// rejected with 429.
func TestRateLimiter_AllowsBurstThenBlocks(t *testing.T) {
	lim := handlers.NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !lim.Allow("ip-1") {
			t.Fatalf("request %d should be allowed (capacity=3)", i+1)
		}
	}
	if lim.Allow("ip-1") {
		t.Errorf("4th request should be rejected (capacity=3)")
	}
}

// TestRateLimiter_KeysAreIndependent confirms two callers don't share buckets.
func TestRateLimiter_KeysAreIndependent(t *testing.T) {
	lim := handlers.NewRateLimiter(2, time.Minute)
	for i := 0; i < 2; i++ {
		lim.Allow("ip-1")
	}
	if !lim.Allow("ip-2") {
		t.Error("a different key must not be affected by ip-1's bucket")
	}
}

// TestRateLimiter_Refills verifies tokens refill over time.
func TestRateLimiter_Refills(t *testing.T) {
	// 4 tokens per second window — one token refills every 250ms.
	lim := handlers.NewRateLimiter(4, time.Second)
	for i := 0; i < 4; i++ {
		lim.Allow("ip")
	}
	if lim.Allow("ip") {
		t.Fatal("bucket should be drained after 4 calls")
	}
	time.Sleep(300 * time.Millisecond) // ≥ 250ms to refill one token
	if !lim.Allow("ip") {
		t.Error("a token should have refilled after 300ms")
	}
}

// TestRateLimitMiddleware_Returns429 verifies the HTTP middleware returns
// 429 with a JSON error body after the burst is exceeded.
func TestRateLimitMiddleware_Returns429(t *testing.T) {
	lim := handlers.NewRateLimiter(2, time.Minute)
	mw := handlers.RateLimitMiddleware(lim, func(r *http.Request) string {
		return "fixed-key"
	})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i+1, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("want 429, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("want application/json, got %q", ct)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" {
		t.Error("Retry-After header should be set on 429")
	}
}

// TestRateLimitMiddleware_EmptyKeySkips verifies that when the key function
// returns an empty string, the request bypasses the limiter. This allows
// callers to opt out (e.g. trusted internal traffic) without forking the
// middleware.
func TestRateLimitMiddleware_EmptyKeySkips(t *testing.T) {
	lim := handlers.NewRateLimiter(1, time.Minute)
	mw := handlers.RateLimitMiddleware(lim, func(r *http.Request) string { return "" })
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 10 requests should all pass because key="" disables limiting.
	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i+1, rec.Code)
		}
	}
}

// TestClientIP_PrefersXRealIP verifies the helper uses X-Real-IP (set by the
// trusted reverse proxy) ahead of the connection's RemoteAddr.
func TestClientIP_PrefersXRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "203.0.113.5")
	req.RemoteAddr = "10.0.0.1:5555"

	got := handlers.ClientIP(req)
	if got != "203.0.113.5" {
		t.Errorf("want 203.0.113.5, got %q", got)
	}
}

// TestClientIP_IgnoresXFF verifies X-Forwarded-For is never consulted: a
// spoofable header must not influence the derived client IP. With no
// X-Real-IP present we fall back to RemoteAddr, ignoring X-Forwarded-For.
func TestClientIP_IgnoresXFF(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	req.RemoteAddr = "192.0.2.10:5555"

	got := handlers.ClientIP(req)
	if got != "192.0.2.10" {
		t.Errorf("X-Forwarded-For must be ignored; want 192.0.2.10, got %q", got)
	}
}

// TestClientIP_FallsBackToRemoteAddr verifies that without X-Real-IP we use
// the connection's remote address (host only, port stripped).
func TestClientIP_FallsBackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.10:5555"

	got := handlers.ClientIP(req)
	if got != "192.0.2.10" {
		t.Errorf("want 192.0.2.10, got %q", got)
	}
}

// TestRateLimitMiddleware_LoginEnforced verifies that the auth/IP limiter
// fires on /api/login after the configured number of requests from the
// same IP. This guards the brute-force protection on the credential
// endpoint.
func TestRateLimitMiddleware_LoginEnforced(t *testing.T) {
	s := openTestDB(t)
	authH, err := handlers.NewAuthHandler(s, nil, "http://localhost", "")
	if err != nil {
		t.Fatal(err)
	}
	lim := handlers.NewRateLimiter(3, time.Minute)
	mw := handlers.RateLimitMiddleware(lim, handlers.IPKey())

	mux := http.NewServeMux()
	mux.Handle("/api/login", mw(http.HandlerFunc(authH.Login)))

	// 3 requests with the same RemoteAddr should be allowed; the 4th
	// rejected with 429.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/api/login", nil)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.7:1234"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d: should not be rate limited yet", i+1)
		}
	}

	req := httptest.NewRequest("POST", "/api/login", nil)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.7:1234"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("4th request: want 429, got %d", rec.Code)
	}

	// A different IP should not be affected.
	req2 := httptest.NewRequest("POST", "/api/login", nil)
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "198.51.100.4:9999"
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code == http.StatusTooManyRequests {
		t.Error("a fresh IP should not be rate limited")
	}
}
