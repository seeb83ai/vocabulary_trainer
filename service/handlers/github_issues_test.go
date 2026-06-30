package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"

	"github.com/go-chi/chi/v5"
)

// mockGitHub returns an httptest server that mimics the GitHub endpoints the
// issue handler uses, recording the issue-creation payload it receives.
func mockGitHub(t *testing.T, gotBody *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues"):
			if gotBody != nil {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, gotBody)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"number":101,"html_url":"https://github.com/owner/repo/issues/101"}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"content":{"download_url":"` + "http://example.test/raw.png" + `"}}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGitHubIssue_CreateValid(t *testing.T) {
	s := openTestDB(t)
	var gotBody map[string]any
	srv := mockGitHub(t, &gotBody)

	prev := testGitHubAPIBaseURL
	testGitHubAPIBaseURL = srv.URL
	defer func() { testGitHubAPIBaseURL = prev }()

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/github/issues", map[string]any{
		"category":    "bug",
		"title":       "Something broke",
		"description": "Steps to reproduce…",
		"page_url":    "http://localhost:8080/train",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		IssueURL string `json:"issue_url"`
		Number   int    `json:"number"`
		ReportID string `json:"report_id"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Number != 101 || !strings.Contains(resp.IssueURL, "/issues/101") {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.ReportID == "" {
		t.Fatal("expected a report_id")
	}

	// Title is category-prefixed and the body carries the report UUID.
	if title, _ := gotBody["title"].(string); title != "[Bug] Something broke" {
		t.Fatalf("issue title = %q, want %q", title, "[Bug] Something broke")
	}
	if body, _ := gotBody["body"].(string); !strings.Contains(body, resp.ReportID) {
		t.Fatalf("issue body missing report id %q: %s", resp.ReportID, body)
	}

	// The UUID→user mapping is recorded privately in the audit log.
	entries, err := s.GetAuditLogForUser(context.Background(), int64(2), 10)
	if err != nil {
		t.Fatalf("GetAuditLogForUser: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == db.AuditGitHubIssue && strings.Contains(e.Detail, resp.ReportID) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no audit_log entry for report %s", resp.ReportID)
	}
}

func TestGitHubIssue_MissingTitle(t *testing.T) {
	s := openTestDB(t)
	srv := mockGitHub(t, nil)
	prev := testGitHubAPIBaseURL
	testGitHubAPIBaseURL = srv.URL
	defer func() { testGitHubAPIBaseURL = prev }()

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/github/issues", map[string]any{
		"category":    "bug",
		"description": "no title",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGitHubIssue_InvalidCategory(t *testing.T) {
	s := openTestDB(t)
	srv := mockGitHub(t, nil)
	prev := testGitHubAPIBaseURL
	testGitHubAPIBaseURL = srv.URL
	defer func() { testGitHubAPIBaseURL = prev }()

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/github/issues", map[string]any{
		"category":    "spam",
		"title":       "x",
		"description": "y",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGitHubIssue_NotConfigured(t *testing.T) {
	s := openTestDB(t)
	// Unconfigured handler (no token) must report disabled and reject submissions.
	ghH := &handlers.GitHubHandler{Store: s}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Post("/api/github/issues", ghH.Create)
	r.Get("/api/github/config", ghH.ConfigFlag)

	rec := do(t, r, http.MethodPost, "/api/github/issues", map[string]any{
		"category": "bug", "title": "x", "description": "y",
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}

	cfg := do(t, r, http.MethodGet, "/api/github/config", nil)
	var flag struct {
		Enabled bool `json:"enabled"`
	}
	decodeJSON(t, cfg, &flag)
	if flag.Enabled {
		t.Fatal("config flag should be disabled when not configured")
	}
}
