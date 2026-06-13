package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimiter is an in-memory token-bucket limiter keyed by an opaque string
// (IP, user ID, etc.). Each key gets its own bucket of size `capacity` that
// refills at a rate of `capacity` tokens per `window`.
type RateLimiter struct {
	capacity int
	window   time.Duration
	mu       sync.Mutex
	buckets  map[string]*bucket
	now      func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter creates a limiter that allows up to `capacity` requests per
// `window` per key.
func NewRateLimiter(capacity int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		capacity: capacity,
		window:   window,
		buckets:  make(map[string]*bucket),
		now:      time.Now,
	}
}

// Allow consumes one token from the bucket identified by key and returns
// whether the request is allowed.
func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: float64(l.capacity) - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	refill := elapsed * float64(l.capacity) / l.window.Seconds()
	b.tokens += refill
	if b.tokens > float64(l.capacity) {
		b.tokens = float64(l.capacity)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// RateLimitMiddleware returns an HTTP middleware that consults the given
// limiter using keyFn to derive the bucket key from each request. A key of
// "" disables limiting for that request (useful for unauthenticated routes
// where we cannot identify the user).
func RateLimitMiddleware(l *RateLimiter, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFn(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			if !l.Allow(key) {
				retry := int(l.window.Seconds()/float64(l.capacity)) + 1
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(retry))
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP returns the originating client IP for the request. It prefers the
// first entry of X-Forwarded-For (set by trusted reverse proxies) and falls
// back to the connection's RemoteAddr.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// IPKey returns a keyFn that buckets by client IP. Suitable for
// unauthenticated endpoints (login, register, verify-email).
func IPKey() func(*http.Request) string {
	return func(r *http.Request) string {
		return "ip:" + ClientIP(r)
	}
}

// UserOrIPKey returns a keyFn that buckets by the authenticated user ID
// (from the request context) if present, otherwise by client IP. Suitable
// for protected endpoints where most traffic carries a session.
func UserOrIPKey() func(*http.Request) string {
	return func(r *http.Request) string {
		if uid := UserIDFromContext(r.Context()); uid > 0 {
			return "u:" + strconv.FormatInt(uid, 10)
		}
		return "ip:" + ClientIP(r)
	}
}
