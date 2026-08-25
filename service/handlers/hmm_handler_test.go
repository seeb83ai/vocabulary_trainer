package handlers_test

import (
	"net/http"
	"testing"
)

// TestHMMBreakdown_Empty exercises HMMHandler.GetBreakdown over HTTP; it lives
// in a separate handlers_test-package file from hmm_test.go (package handlers,
// unit tests for unexported helpers like parsePinyin/collectRadicalDefs) since
// it needs the shared router_test.go fixtures (newRouter, openTestDB, do, decodeJSON).
func TestHMMBreakdown_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodGet, "/api/hmm/breakdown", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	items, _ := resp["breakdown"].([]any)
	if len(items) != 0 {
		t.Errorf("want empty breakdown on fresh DB, got %d items", len(items))
	}
}
