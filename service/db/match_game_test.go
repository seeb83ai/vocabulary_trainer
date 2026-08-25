package db

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestGetHardestWordsForGame_RanksByLowestAccuracy(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a := seedWord(t, s, "一", "", []string{"one"}) // 10% accuracy -> hardest
	b := seedWord(t, s, "二", "", []string{"two"}) // 90% accuracy
	makeDifficult(t, s, a, 1, 10, 2.5, time.Hour)
	makeDifficult(t, s, b, 9, 10, 2.5, time.Hour)

	words, err := s.GetHardestWordsForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %+v", len(words), words)
	}
	if words[0].ZhWordID != a {
		t.Errorf("expected hardest word %d ranked first, got %d", a, words[0].ZhWordID)
	}
}

func TestGetHardestWordsForGame_RespectsAttemptsGuard(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	few := seedWord(t, s, "一", "", []string{"one"})
	makeDifficult(t, s, few, 0, 2, 1.3, time.Hour) // ta=2, below the >=3 guard

	words, err := s.GetHardestWordsForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 0 {
		t.Errorf("expected 0 candidates below attempts guard, got %d", len(words))
	}
}

// TestGetHardestWordsForGame_RepeatAvoidance covers issue #288 decision #4:
// a word shown in this mode is suppressed until it has any newer attempt
// (right or wrong) than when it was last shown.
func TestGetHardestWordsForGame_RepeatAvoidance(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a := seedWord(t, s, "一", "", []string{"one"})
	b := seedWord(t, s, "二", "", []string{"two"})
	makeDifficult(t, s, a, 1, 10, 2.5, time.Hour)
	makeDifficult(t, s, b, 2, 10, 2.5, time.Hour)
	setLastAttemptOffset(t, s, a, "-2 hours")
	setLastAttemptOffset(t, s, b, "-2 hours")

	words, err := s.GetHardestWordsForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 2 {
		t.Fatalf("expected 2 candidates before shown, got %d", len(words))
	}

	setWordGameShownOffset(t, s, 2, a, "hardest", "-1 hour")
	setWordGameShownOffset(t, s, 2, b, "hardest", "-1 hour")

	words2, err := s.GetHardestWordsForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words2) != 0 {
		t.Errorf("expected 0 candidates immediately after shown, got %d: %+v", len(words2), words2)
	}

	// b gets a fresh attempt (correct, doesn't matter) after being shown.
	setLastAttemptOffset(t, s, b, "-30 minutes")

	words3, err := s.GetHardestWordsForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words3) != 1 || words3[0].ZhWordID != b {
		t.Errorf("expected only word %d re-eligible after new attempt, got %+v", b, words3)
	}
}

func TestGetLastMistakesForGame_ExcludesWordsNeverWrong(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedWord(t, s, "一", "", []string{"one"}) // never wrong -> last_wrong_at NULL

	words, err := s.GetLastMistakesForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 0 {
		t.Errorf("expected 0 candidates (no recorded mistakes), got %d", len(words))
	}
}

// TestGetLastMistakesForGame_OnlyNewWrongAnswerReEligible covers issue #288
// decision #4: unlike hardest-words, a merely-correct re-attempt must NOT
// re-include a word shown in this mode — only a fresh wrong answer does.
func TestGetLastMistakesForGame_OnlyNewWrongAnswerReEligible(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a := seedWord(t, s, "一", "", []string{"one"})
	setLastWrongOffset(t, s, a, "-2 hours")

	words, err := s.GetLastMistakesForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(words))
	}

	setWordGameShownOffset(t, s, 2, a, "last_mistakes", "-1 hour")

	words2, err := s.GetLastMistakesForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words2) != 0 {
		t.Fatalf("expected 0 candidates immediately after shown, got %d", len(words2))
	}

	// A correct re-attempt only touches last_attempt_at, not last_wrong_at —
	// must NOT re-include the word.
	setLastAttemptOffset(t, s, a, "-30 minutes")

	words3, err := s.GetLastMistakesForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words3) != 0 {
		t.Errorf("a correct re-attempt must not re-include the word, got %+v", words3)
	}

	// A fresh wrong answer re-includes it.
	setLastWrongOffset(t, s, a, "-10 minutes")

	words4, err := s.GetLastMistakesForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words4) != 1 || words4[0].ZhWordID != a {
		t.Errorf("expected word %d re-eligible after new wrong answer, got %+v", a, words4)
	}
}

func TestMarkWordsShownInGame_UpsertAndModeScoping(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a := seedWord(t, s, "一", "", []string{"one"})

	if err := s.MarkWordsShownInGame(ctx, int64(2), []int64{a}, "hardest"); err != nil {
		t.Fatal(err)
	}
	// Upsert again must not error.
	if err := s.MarkWordsShownInGame(ctx, int64(2), []int64{a}, "hardest"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM word_game_shown WHERE user_id = 2 AND word_id = ? AND game_mode = 'hardest'`, a,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 row after upsert, got %d", count)
	}
	// A different mode must get its own independent row.
	var otherCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM word_game_shown WHERE user_id = 2 AND word_id = ? AND game_mode = 'last_mistakes'`, a,
	).Scan(&otherCount); err != nil {
		t.Fatal(err)
	}
	if otherCount != 0 {
		t.Errorf("expected no last_mistakes row from a hardest-mode mark, got %d", otherCount)
	}
}

// newestWordsGamePoolSize mirrors the production constant so the pool-window
// test can seed exactly one word beyond it without hardcoding two "30"s.
const newestWordsGamePoolSizeForTest = 30

func TestGetNewestWordsForGame_OnlyWithinPoolWindow(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	var ids []int64
	for i := 0; i < newestWordsGamePoolSizeForTest+5; i++ {
		ids = append(ids, seedWord(t, s, fmt.Sprintf("word%02d", i), "", []string{fmt.Sprintf("w%02d", i)}))
	}
	outsidePool := ids[:5] // oldest 5, created before the 30-newest window

	counts := map[int64]int{}
	for i := 0; i < 300; i++ {
		words, err := s.GetNewestWordsForGame(ctx, int64(2), 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(words) != 1 {
			t.Fatalf("expected 1 word, got %d", len(words))
		}
		counts[words[0].ZhWordID]++
	}
	for _, id := range outsidePool {
		if counts[id] != 0 {
			t.Errorf("word %d is outside the 30-newest pool but was picked %d times", id, counts[id])
		}
	}
}

// TestGetNewestWordsForGame_WeightDecaysWithAge covers issue #288's weighted
// decay requirement: within the pool, older words must be picked less often
// than newer ones. weight(rank) = 1/(rank+1) makes this an extreme (30x) skew
// between the newest and oldest pool member, so it's safe to assert on a
// statistical sample without flaking.
func TestGetNewestWordsForGame_WeightDecaysWithAge(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	var ids []int64
	for i := 0; i < newestWordsGamePoolSizeForTest; i++ {
		ids = append(ids, seedWord(t, s, fmt.Sprintf("word%02d", i), "", []string{fmt.Sprintf("w%02d", i)}))
	}
	oldest := ids[0]
	newest := ids[len(ids)-1]

	counts := map[int64]int{}
	for i := 0; i < 2000; i++ {
		words, err := s.GetNewestWordsForGame(ctx, int64(2), 1)
		if err != nil {
			t.Fatal(err)
		}
		counts[words[0].ZhWordID]++
	}
	if counts[newest] <= counts[oldest] {
		t.Errorf("expected newest word picked more often than oldest: newest=%d oldest=%d", counts[newest], counts[oldest])
	}
}

func TestGetNewestWordsForGame_DistinctWithinRound(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		seedWord(t, s, fmt.Sprintf("word%02d", i), "", []string{fmt.Sprintf("w%02d", i)})
	}

	words, err := s.GetNewestWordsForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 4 {
		t.Fatalf("expected 4 words, got %d", len(words))
	}
	seen := map[int64]bool{}
	for _, w := range words {
		if seen[w.ZhWordID] {
			t.Errorf("duplicate word %d within a single round", w.ZhWordID)
		}
		seen[w.ZhWordID] = true
	}
}

func TestGetNewestWordsForGame_FewerWordsThanRequested(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedWord(t, s, "一", "", []string{"one"})

	words, err := s.GetNewestWordsForGame(ctx, int64(2), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 1 {
		t.Errorf("expected 1 word (fewer than requested), got %d", len(words))
	}
}

func setLastAttemptOffset(t *testing.T, s *Store, wordID int64, offset string) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		`UPDATE sm2_progress SET last_attempt_at = datetime('now', ?) WHERE word_id = ?`, offset, wordID); err != nil {
		t.Fatalf("setLastAttemptOffset(%d): %v", wordID, err)
	}
}

func setLastWrongOffset(t *testing.T, s *Store, wordID int64, offset string) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		`UPDATE sm2_progress SET last_wrong_at = datetime('now', ?) WHERE word_id = ?`, offset, wordID); err != nil {
		t.Fatalf("setLastWrongOffset(%d): %v", wordID, err)
	}
}

func setWordGameShownOffset(t *testing.T, s *Store, userID, wordID int64, mode, offset string) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(), `
		INSERT INTO word_game_shown (user_id, word_id, game_mode, last_shown_in_game)
		VALUES (?, ?, ?, datetime('now', ?))
		ON CONFLICT(user_id, word_id, game_mode) DO UPDATE SET last_shown_in_game = excluded.last_shown_in_game`,
		userID, wordID, mode, offset); err != nil {
		t.Fatalf("setWordGameShownOffset(%d): %v", wordID, err)
	}
}
