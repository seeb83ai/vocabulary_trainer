package handlers_test

import (
	"net/http"
	"testing"
)

// ── GET /api/admin/overview ─────────────────────────────────────────────────

func TestAdminOverview_AdminGetsData(t *testing.T) {
	s := openTestDB(t)
	r := newRouterWithUserID(s, 1) // seeded admin user

	rec := do(t, r, "GET", "/api/admin/overview", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if _, ok := resp["users"]; !ok {
		t.Errorf("expected 'users' key in response, got %v", resp)
	}
}

func TestAdminOverview_ExcludeSeedParam(t *testing.T) {
	s := openTestDB(t)
	r := newRouterWithUserID(s, 1) // seeded admin user

	rec := do(t, r, "GET", "/api/admin/overview?exclude_seed=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Users struct {
			Total int `json:"total"`
		} `json:"users"`
	}
	decodeJSON(t, rec, &resp)
	// Only the two seed accounts (id 1, 2) exist in the fresh test DB, so
	// excluding them should leave zero users.
	if resp.Users.Total != 0 {
		t.Errorf("expected 0 users with exclude_seed=1 on a fresh DB, got %d", resp.Users.Total)
	}
}

func TestAdminOverview_NonAdminForbidden(t *testing.T) {
	s := openTestDB(t)
	r := newRouterWithUserID(s, 2) // seeded plus user, not admin

	rec := do(t, r, "GET", "/api/admin/overview", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
