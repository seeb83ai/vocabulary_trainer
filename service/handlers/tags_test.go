package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"vocabulary_trainer/models"
)

func TestTagDetails_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/tags/details", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 0 {
		t.Errorf("want empty, got %v", tags)
	}
}

func TestTagDetails_ReturnsTags(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 2, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"greetings"})

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/tags/details", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 1 || tags[0].Name != "greetings" {
		t.Fatalf("want [{greetings ...}], got %v", tags)
	}
	if tags[0].Description != "" {
		t.Errorf("expected empty description, got %q", tags[0].Description)
	}
	if !tags[0].Importable {
		t.Errorf("expected importable=true by default")
	}
}

func TestTagDetails_DoesNotReturnOtherUserTags(t *testing.T) {
	s := openTestDB(t)
	// User 1 has a tag; user 2 (current user in tests) has none.
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"library"})

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/tags/details", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 0 {
		t.Errorf("want 0 tags for user 2, got %v", tags)
	}
}

func TestTagUpdate_SetsDescriptionAndImportable(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 2, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"hsk1"})

	r := newRouter(s)
	rec := do(t, r, "PUT", "/api/tags/hsk1", map[string]any{
		"description": "HSK level 1 words",
		"importable":  false,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	// Verify via GET /api/tags/details.
	rec2 := do(t, r, "GET", "/api/tags/details", nil)
	var tags []models.TagDetail
	decodeJSON(t, rec2, &tags)
	if len(tags) != 1 {
		t.Fatalf("want 1 tag, got %d", len(tags))
	}
	if tags[0].Description != "HSK level 1 words" {
		t.Errorf("expected description 'HSK level 1 words', got %q", tags[0].Description)
	}
	if tags[0].Importable {
		t.Errorf("expected importable=false after update")
	}
}

func TestTagUpdate_InvalidBody(t *testing.T) {
	r := newRouter(openTestDB(t))
	req := httptest.NewRequest("PUT", "/api/tags/hsk1", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}
