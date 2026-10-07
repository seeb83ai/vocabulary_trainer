package handlers_test

import (
	"net/http"
	"testing"
	"vocabulary_trainer/db"
)

// seedLapsedWord seeds a word whose last review failed, due tomorrow.
func seedLapsedWord(t *testing.T, s *db.Store, zhText string) int64 {
	t.Helper()
	id := seedWord(t, s, zhText, "", []string{zhText + "-en"})
	if _, err := s.ExecForTest(`UPDATE sm2_progress SET learning_new_word = 0, repetitions = 2,
		first_seen_at = datetime('now', '-5 days'), last_attempt_at = datetime('now', '-1 day'),
		due_date = datetime('now', '+1 day'), total_attempts = 5, total_correct = 3,
		lapses = 1, consecutive_lapses = 1 WHERE word_id = ?`, id); err != nil {
		t.Fatalf("seedLapsedWord: %v", err)
	}
	return id
}

func TestSettings_LapsedBaselineDefaultsOn(t *testing.T) {
	r := newRouter(openTestDB(t))
	st := getSettingsMap(t, r)
	if st["baseline_lapsed_enabled"] != true || st["baseline_lapsed_value"] != float64(10) {
		t.Fatalf("defaults: want true/10, got %v/%v", st["baseline_lapsed_enabled"], st["baseline_lapsed_value"])
	}

	rec := patchSettingsFromCurrent(t, r, func(st map[string]any) {
		st["baseline_lapsed_enabled"] = false
		st["baseline_lapsed_value"] = 4
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}
	st = getSettingsMap(t, r)
	if st["baseline_lapsed_enabled"] != false || st["baseline_lapsed_value"] != float64(4) {
		t.Errorf("after patch: want false/4, got %v/%v", st["baseline_lapsed_enabled"], st["baseline_lapsed_value"])
	}

	// An older client that does not know the fields keeps the stored values.
	rec = patchSettingsFromCurrent(t, r, func(st map[string]any) {
		delete(st, "baseline_lapsed_enabled")
		delete(st, "baseline_lapsed_value")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch without fields: %d %s", rec.Code, rec.Body)
	}
	st = getSettingsMap(t, r)
	if st["baseline_lapsed_enabled"] != false || st["baseline_lapsed_value"] != float64(4) {
		t.Errorf("after patch without fields: want false/4, got %v/%v", st["baseline_lapsed_enabled"], st["baseline_lapsed_value"])
	}

	rec = patchSettingsFromCurrent(t, r, func(st map[string]any) { st["baseline_lapsed_value"] = -1 })
	if rec.Code != http.StatusBadRequest {
		t.Errorf("negative baseline_lapsed_value: want 400, got %d", rec.Code)
	}
}

func TestLapsedBaseline_PausesNewWords(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	seedLapsedWord(t, s, "水")
	seedLapsedWord(t, s, "山")
	seedWord(t, s, "火", "", []string{"fire"})
	if rec := patchSettingsFromCurrent(t, r, func(st map[string]any) { st["baseline_lapsed_value"] = 2 }); rec.Code != 200 {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}

	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	var stats map[string]any
	decodeJSON(t, rec, &stats)
	if stats["lapsed_pause_count"] != float64(2) || stats["lapsed_pause_max"] != float64(2) {
		t.Errorf("stats pause fields: want 2/2, got %v/%v", stats["lapsed_pause_count"], stats["lapsed_pause_max"])
	}
	if stats["new_available"] != float64(0) {
		t.Errorf("new_available: want 0, got %v", stats["new_available"])
	}

	rec = do(t, r, "GET", "/api/quiz/next?langs=en", nil)
	if rec.Code == http.StatusOK {
		var card map[string]any
		decodeJSON(t, rec, &card)
		if card["prompt"] == "火" {
			t.Errorf("the unseen word 火 must not be served while new words are paused")
		}
	}
}

func TestLapsedBaseline_BelowLimitNoPause(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	seedLapsedWord(t, s, "水")
	seedWord(t, s, "火", "", []string{"fire"})

	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	var stats map[string]any
	decodeJSON(t, rec, &stats)
	if _, ok := stats["lapsed_pause_count"]; ok {
		t.Errorf("1 lapsed word, default limit 10: want no lapsed_pause_count, got %v", stats["lapsed_pause_count"])
	}
}
