package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"vocabulary_trainer/handlers"

	"github.com/go-chi/chi/v5"
)

func TestUsageTracking_RecordsAuthenticatedHit(t *testing.T) {
	s := openTestDB(t)
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Use(handlers.UsageTracking(s))
	r.Get("/train", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/train", nil))

	count, lastSeen, err := s.GetUsageEventForTest(context.Background(), 2, "GET /train")
	if err != nil {
		t.Fatalf("expected a recorded usage event, got error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
	if lastSeen == "" {
		t.Error("expected last_seen to be set")
	}
}

func TestUsageTracking_RecordsAnonymousHitWithZeroUserID(t *testing.T) {
	s := openTestDB(t)
	r := chi.NewRouter()
	r.Use(handlers.UsageTracking(s))
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	count, _, err := s.GetUsageEventForTest(context.Background(), 0, "GET /login")
	if err != nil {
		t.Fatalf("expected an anonymous usage event with user_id=0, got error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

func TestUsageTracking_IncrementsOnRepeatedHits(t *testing.T) {
	s := openTestDB(t)
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Use(handlers.UsageTracking(s))
	r.Get("/vocab", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/vocab", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/vocab", nil))

	count, _, err := s.GetUsageEventForTest(context.Background(), 2, "GET /vocab")
	if err != nil {
		t.Fatalf("expected a recorded usage event, got error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2 after two hits, got %d", count)
	}
}

func TestUsageTracking_SkipsStaticCatchAllRoute(t *testing.T) {
	s := openTestDB(t)
	r := chi.NewRouter()
	r.Use(handlers.UsageTracking(s))
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/app.js", nil))

	total, err := s.CountUsageEventsForTest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("expected no usage events for the static catch-all route, got %d", total)
	}
}

func TestUsageTracking_SkipsAudioRoutes(t *testing.T) {
	s := openTestDB(t)
	r := chi.NewRouter()
	r.Use(handlers.UsageTracking(s))
	r.Get("/api/audio/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/api/pinyin-quiz/audio/{filename}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/audio/5", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/pinyin-quiz/audio/x.mp3", nil))

	total, err := s.CountUsageEventsForTest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("expected audio routes to be excluded from tracking, got %d rows", total)
	}
}

func TestUsageTracking_DoesNotBreakRequestOnDBFailure(t *testing.T) {
	s := openTestDB(t)
	s.Close()

	r := chi.NewRouter()
	r.Use(handlers.UsageTracking(s))
	r.Get("/train", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/train", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected the request to still succeed despite a tracking failure, got status %d", rec.Code)
	}
}
