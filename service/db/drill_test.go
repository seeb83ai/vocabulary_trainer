package db

import (
	"context"
	"testing"
	"time"
)

func TestFlagDifficultWords_LowestAccuracyAndEasiness(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a := seedWord(t, s, "一", "", []string{"one"})   // lowest accuracy (10%)
	b := seedWord(t, s, "二", "", []string{"two"})   // lowest easiness (1.3)
	c := seedWord(t, s, "三", "", []string{"three"}) // mid
	d := seedWord(t, s, "四", "", []string{"four"})  // easiest/highest accuracy
	makeDifficult(t, s, a, 1, 10, 2.5, time.Hour)
	makeDifficult(t, s, b, 9, 10, 1.3, time.Hour)
	makeDifficult(t, s, c, 5, 10, 2.0, time.Hour)
	makeDifficult(t, s, d, 10, 10, 2.8, time.Hour)

	n, err := s.FlagDifficultWords(ctx, int64(2), 2)
	if err != nil {
		t.Fatalf("FlagDifficultWords: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 flagged, got %d", n)
	}
	if !isFlagged(t, s, a) {
		t.Errorf("expected lowest-accuracy word %d flagged", a)
	}
	if !isFlagged(t, s, b) {
		t.Errorf("expected lowest-easiness word %d flagged", b)
	}
	if isFlagged(t, s, c) || isFlagged(t, s, d) {
		t.Errorf("did not expect words c/d flagged")
	}
}

func TestFlagDifficultWords_RespectsAttemptsGuard(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	few := seedWord(t, s, "一", "", []string{"one"}) // ta=2, below guard
	ok := seedWord(t, s, "二", "", []string{"two"})  // ta=3, eligible
	makeDifficult(t, s, few, 0, 2, 1.3, time.Hour)
	makeDifficult(t, s, ok, 1, 3, 1.3, time.Hour)

	n, err := s.FlagDifficultWords(ctx, int64(2), 5)
	if err != nil {
		t.Fatalf("FlagDifficultWords: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 flagged (attempts guard), got %d", n)
	}
	if isFlagged(t, s, few) {
		t.Errorf("word with <3 attempts must not be flagged")
	}
	if !isFlagged(t, s, ok) {
		t.Errorf("eligible word must be flagged")
	}
}

func TestFlagDifficultWords_ClearsPreviousFlags(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a := seedWord(t, s, "一", "", []string{"one"})
	b := seedWord(t, s, "二", "", []string{"two"})
	makeDifficult(t, s, a, 1, 10, 1.3, time.Hour)
	makeDifficult(t, s, b, 9, 10, 2.5, time.Hour)

	if _, err := s.FlagDifficultWords(ctx, int64(2), 1); err != nil {
		t.Fatal(err)
	}
	if !isFlagged(t, s, a) {
		t.Fatalf("expected word a flagged on first drill")
	}
	// Make b the hardest, re-flag with count=1 — a must be cleared.
	makeDifficult(t, s, b, 0, 10, 1.3, time.Hour)
	makeDifficult(t, s, a, 10, 10, 2.8, time.Hour)
	if _, err := s.FlagDifficultWords(ctx, int64(2), 1); err != nil {
		t.Fatal(err)
	}
	if isFlagged(t, s, a) {
		t.Errorf("previous flag on a must be cleared by a new drill")
	}
	if !isFlagged(t, s, b) {
		t.Errorf("expected word b flagged on second drill")
	}
}

func TestGetNextDrillCard_OrdersByDueDateAndIgnoresHorizon(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	soon := seedWord(t, s, "一", "", []string{"one"})
	later := seedWord(t, s, "二", "", []string{"two"})
	// Both flagged; "later" is far in the future (beyond the normal horizon).
	makeDifficult(t, s, soon, 1, 10, 1.3, 1*time.Hour)
	makeDifficult(t, s, later, 1, 10, 1.3, 72*time.Hour)
	if _, err := s.FlagDifficultWords(ctx, int64(2), 5); err != nil {
		t.Fatal(err)
	}

	w, p, err := s.GetNextDrillCard(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetNextDrillCard: %v", err)
	}
	if w == nil {
		t.Fatal("expected a flagged card")
	}
	if w.ID != soon {
		t.Errorf("expected soonest-due flagged word %d, got %d", soon, w.ID)
	}
	if p == nil || p.WordID != soon {
		t.Errorf("expected progress for word %d", soon)
	}

	// Clear the soon flag — the far-future flagged word must still be returned.
	if err := s.ClearDrillFlag(ctx, soon); err != nil {
		t.Fatal(err)
	}
	w2, _, err := s.GetNextDrillCard(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if w2 == nil || w2.ID != later {
		t.Errorf("expected far-future flagged word %d to still be served, got %v", later, w2)
	}
}

func TestGetNextDrillCard_NoneFlagged_ReturnsNil(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "一", "", []string{"one"})
	makeDifficult(t, s, id, 1, 10, 1.3, time.Hour) // eligible but not flagged
	w, _, err := s.GetNextDrillCard(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if w != nil {
		t.Errorf("expected nil when no words are flagged, got id=%d", w.ID)
	}
}

func TestClearAllDrillFlags_And_Count(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a := seedWord(t, s, "一", "", []string{"one"})
	b := seedWord(t, s, "二", "", []string{"two"})
	makeDifficult(t, s, a, 1, 10, 1.3, time.Hour)
	makeDifficult(t, s, b, 2, 10, 1.4, time.Hour)
	if _, err := s.FlagDifficultWords(ctx, int64(2), 5); err != nil {
		t.Fatal(err)
	}
	n, err := s.CountDrillFlags(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 flagged, got %d", n)
	}
	if err := s.ClearAllDrillFlags(ctx, int64(2)); err != nil {
		t.Fatal(err)
	}
	n, err = s.CountDrillFlags(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 flagged after clear-all, got %d", n)
	}
}

func isFlagged(t *testing.T, s *Store, id int64) bool {
	t.Helper()
	var f int
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT drill_flag FROM sm2_progress WHERE word_id = ?`, id).Scan(&f); err != nil {
		t.Fatalf("isFlagged(%d): %v", id, err)
	}
	return f == 1
}
