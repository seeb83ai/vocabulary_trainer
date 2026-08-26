package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestImages_NotConfigured(t *testing.T) {
	s := openTestDB(t)
	id := seedWordFull(t, s, int64(2), "猫", "māo", []string{"cat"}, nil, nil)

	testUnsplashAccessKey = ""
	testUnsplashBaseURL = ""
	defer func() { testUnsplashAccessKey, testUnsplashBaseURL = "", "" }()
	r := newRouter(s)

	rec := do(t, r, "GET", fmt.Sprintf("/api/words/%d/image", id), nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
}

func TestImages_FetchesAndCaches(t *testing.T) {
	s := openTestDB(t)
	id := seedWordFull(t, s, int64(2), "猫", "māo", []string{"cat"}, nil, nil)

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if got := r.URL.Query().Get("query"); got != "cat" {
			t.Errorf("query = %q, want cat", got)
		}
		if got := r.Header.Get("Authorization"); got != "Client-ID test-unsplash-key" {
			t.Errorf("authorization header = %q", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"urls": map[string]string{"regular": "https://images.example/cat.jpg"}},
			},
		})
	}))
	defer srv.Close()

	testUnsplashAccessKey = "test-unsplash-key"
	testUnsplashBaseURL = srv.URL
	defer func() { testUnsplashAccessKey, testUnsplashBaseURL = "", "" }()
	r := newRouter(s)

	path := fmt.Sprintf("/api/words/%d/image", id)
	rec := do(t, r, "GET", path, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ImageURL string `json:"image_url"`
	}
	decodeJSON(t, rec, &resp)
	if resp.ImageURL != "https://images.example/cat.jpg" {
		t.Errorf("image_url = %q, want https://images.example/cat.jpg", resp.ImageURL)
	}
	if callCount != 1 {
		t.Fatalf("unsplash calls = %d, want 1", callCount)
	}

	// Second request must be served from cache, not hit Unsplash again.
	rec2 := do(t, r, "GET", path, nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200: %s", rec2.Code, rec2.Body.String())
	}
	decodeJSON(t, rec2, &resp)
	if resp.ImageURL != "https://images.example/cat.jpg" {
		t.Errorf("cached image_url = %q, want https://images.example/cat.jpg", resp.ImageURL)
	}
	if callCount != 1 {
		t.Errorf("unsplash calls after cache hit = %d, want still 1", callCount)
	}
}

func TestImages_FallsBackToGermanTranslation(t *testing.T) {
	s := openTestDB(t)
	id := seedWordFull(t, s, int64(2), "猫", "māo", nil, []string{"Katze"}, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("query"); got != "Katze" {
			t.Errorf("query = %q, want Katze", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"urls": map[string]string{"regular": "https://images.example/katze.jpg"}},
			},
		})
	}))
	defer srv.Close()

	testUnsplashAccessKey = "test-unsplash-key"
	testUnsplashBaseURL = srv.URL
	defer func() { testUnsplashAccessKey, testUnsplashBaseURL = "", "" }()
	r := newRouter(s)

	rec := do(t, r, "GET", fmt.Sprintf("/api/words/%d/image", id), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestImages_UpstreamErrorReturnsBadGateway(t *testing.T) {
	s := openTestDB(t)
	id := seedWordFull(t, s, int64(2), "猫", "māo", []string{"cat"}, nil, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	testUnsplashAccessKey = "test-unsplash-key"
	testUnsplashBaseURL = srv.URL
	defer func() { testUnsplashAccessKey, testUnsplashBaseURL = "", "" }()
	r := newRouter(s)

	rec := do(t, r, "GET", fmt.Sprintf("/api/words/%d/image", id), nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", rec.Code, rec.Body.String())
	}
}
