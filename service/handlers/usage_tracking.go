package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
	"vocabulary_trainer/db"

	"github.com/go-chi/chi/v5"
)

// skippedUsagePrefixes excludes high-frequency, low-signal routes (audio
// streaming) from usage tracking.
var skippedUsagePrefixes = []string{
	"/api/audio/",
	"/api/pinyin-quiz/audio/",
}

// UsageTracking is a middleware that records one usage_events hit per request
// against the matched chi route pattern (e.g. "GET /train"), keyed by the
// authenticated user (0 for anonymous). It never fails the request: tracking
// errors are logged and swallowed.
func UsageTracking(store *db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)

			pattern := chi.RouteContext(r.Context()).RoutePattern()
			if pattern == "" || pattern == "/*" {
				return
			}
			for _, prefix := range skippedUsagePrefixes {
				if strings.HasPrefix(pattern, prefix) {
					return
				}
			}

			name := r.Method + " " + pattern
			userID := UserIDFromContext(r.Context())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := store.RecordUsageEvent(ctx, userID, name); err != nil {
				log.Printf("usage tracking: %v", err)
			}
		})
	}
}
