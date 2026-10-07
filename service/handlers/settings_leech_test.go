package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// patchSettingsFromCurrent reads the current settings, applies mutate and
// PATCHes them back, like the settings page does.
func patchSettingsFromCurrent(t *testing.T, r http.Handler, mutate func(map[string]any)) *httptest.ResponseRecorder {
	t.Helper()
	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	var st map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	mutate(st)
	return do(t, r, http.MethodPatch, "/api/settings", st)
}

func getSettingsMap(t *testing.T, r http.Handler) map[string]any {
	t.Helper()
	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	var st map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	return st
}

func TestSettings_LeechThreshold(t *testing.T) {
	r := newRouter(openTestDB(t))

	if got := getSettingsMap(t, r)["leech_threshold"]; got != float64(5) {
		t.Fatalf("default leech_threshold: want 5, got %v", got)
	}

	rec := patchSettingsFromCurrent(t, r, func(st map[string]any) { st["leech_threshold"] = 3 })
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}
	if got := getSettingsMap(t, r)["leech_threshold"]; got != float64(3) {
		t.Errorf("leech_threshold after patch: want 3, got %v", got)
	}

	// An older client that does not know the field keeps the stored value.
	rec = patchSettingsFromCurrent(t, r, func(st map[string]any) { delete(st, "leech_threshold") })
	if rec.Code != http.StatusOK {
		t.Fatalf("patch without field: %d %s", rec.Code, rec.Body)
	}
	if got := getSettingsMap(t, r)["leech_threshold"]; got != float64(3) {
		t.Errorf("leech_threshold after patch without field: want 3, got %v", got)
	}

	rec = patchSettingsFromCurrent(t, r, func(st map[string]any) { st["leech_threshold"] = -1 })
	if rec.Code != http.StatusBadRequest {
		t.Errorf("negative leech_threshold: want 400, got %d", rec.Code)
	}
}
