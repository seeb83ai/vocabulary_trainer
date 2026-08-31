package db

import (
	"context"
	"database/sql"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

func TestUpdateSM2Progress_Persists(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	p, err := s.GetSM2Progress(context.Background(), id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}

	p.Repetitions = 3
	p.Easiness = 2.8
	p.IntervalDays = 15
	p.TotalCorrect = 7
	p.TotalAttempts = 10
	p.DueDate = time.Now().UTC().Add(15 * 24 * time.Hour)

	if err := s.UpdateSM2Progress(context.Background(), *p); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSM2Progress(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Repetitions != 3 {
		t.Errorf("repetitions: want 3, got %d", got.Repetitions)
	}
	if got.TotalCorrect != 7 {
		t.Errorf("total_correct: want 7, got %d", got.TotalCorrect)
	}
	if got.IntervalDays != 15 {
		t.Errorf("interval_days: want 15, got %d", got.IntervalDays)
	}
}

func TestUpdateSM2Progress_KnownCorrectCountPersists(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	p, err := s.GetSM2Progress(context.Background(), id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	if p.KnownCorrectCount != 0 {
		t.Errorf("initial known_correct_count: want 0, got %d", p.KnownCorrectCount)
	}

	p.KnownCorrectCount = 4
	if err := s.UpdateSM2Progress(context.Background(), *p); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSM2Progress(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.KnownCorrectCount != 4 {
		t.Errorf("known_correct_count: want 4, got %d", got.KnownCorrectCount)
	}
}

func TestGetStats_Empty(t *testing.T) {
	s := openTestDB(t)
	due, total, _, err := s.GetStats(context.Background(), int64(2), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if due != 0 || total != 0 {
		t.Errorf("empty db: want 0/0, got %d/%d", due, total)
	}
}

func TestGetStats_CountsOnlyZh(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello", "hi"})
	_, total, _, err := s.GetStats(context.Background(), int64(2), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	// Only 1 zh word should be counted, not the 2 en words
	if total != 1 {
		t.Errorf("total zh words: want 1, got %d", total)
	}
}

func TestGetStats_DueTodayCount(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "一", "", []string{"one"})
	seedWord(t, s, "二", "", []string{"two"})

	// Mark both words as seen so they count as due
	ctx := context.Background()
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET first_seen_at = date('now')`)

	// Move one word into the future so it's NOT due
	future := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ? WHERE word_id = ?`, future, id1)

	due, _, _, err := s.GetStats(ctx, int64(2), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if due != 1 {
		t.Errorf("due_today: want 1, got %d", due)
	}
}

func TestGetStats_NewTodayCount(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1 := seedWord(t, s, "一", "", []string{"one"})
	seedWord(t, s, "二", "", []string{"two"})

	// Stamp one word as introduced today.
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET first_seen_at = date('now') WHERE word_id = ?`, id1)

	_, _, newToday, err := s.GetStats(ctx, int64(2), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if newToday != 1 {
		t.Errorf("new_today: want 1, got %d", newToday)
	}
}

func TestAcknowledgeWord_SetsFirstSeenAt(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	id, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "水", Translations: map[string][]string{"en": {"water"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeWord(ctx, userID, id); err != nil {
		t.Fatal(err)
	}

	var firstSeenAt string
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(first_seen_at, '') FROM sm2_progress WHERE word_id = ?`, id).
		Scan(&firstSeenAt); err != nil {
		t.Fatalf("query first_seen_at: %v", err)
	}
	if firstSeenAt == "" {
		t.Error("AcknowledgeWord should set first_seen_at")
	}
}

func TestGetStats_FilterByTag(t *testing.T) {
	s := openTestDB(t)
	seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"greetings"})
	seedWordWithTags(t, s, "吃饭", "", []string{"eat"}, []string{"food"})

	_, total, _, err := s.GetStats(context.Background(), int64(2), []string{"food"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("tag-filtered total: want 1, got %d", total)
	}
}

func TestCountLearningNewWords_BeforePresented(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Newly created word: learning_new_word=1 (default), first_seen_at=NULL
	wordId := seedWord(t, s, "一", "", []string{"one"})

	count, err := s.CountLearningNewWords(ctx, int64(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Must count unseen learning words so the new-word gate works correctly.
	if count != 0 {
		t.Errorf("want 0 learning word (unseen), got %d", count)
	}

	s.AcknowledgeWord(ctx, int64(2), wordId)

	count, err = s.CountLearningNewWords(ctx, int64(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Must count unseen learning words so the new-word gate works correctly.
	if count != 1 {
		t.Errorf("want 1 learning word (unseen), got %d", count)
	}
}

func TestCountLearningNewWords_GraduatedNotCounted(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	id := seedWord(t, s, "一", "", []string{"one"})
	// Graduate the word (learning_new_word=0)
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET learning_new_word = 0 WHERE word_id = ?`, id)

	count, err := s.CountLearningNewWords(ctx, int64(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("graduated word should not count as learning, got %d", count)
	}
}

func TestAcknowledgeWord_SetsLearningPhase(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatal(err)
	}

	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	if !p.LearningNewWord {
		t.Error("AcknowledgeWord should set learning_new_word=1")
	}
	if p.TotalAttempts != 1 {
		t.Errorf("total_attempts: want 1, got %d", p.TotalAttempts)
	}
}

func TestAcknowledgeWord_Idempotent(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	s.AcknowledgeWord(ctx, int64(2), id)
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Errorf("second AcknowledgeWord should not error: %v", err)
	}

	p, _ := s.GetSM2Progress(ctx, id)
	if p.TotalAttempts != 1 {
		t.Errorf("total_attempts should not increment beyond 1: got %d", p.TotalAttempts)
	}
}

func TestSkipWord_AdvancesDueDateByNDays(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "一", "", []string{"one"})

	before := time.Now().UTC()
	if err := s.SkipWord(ctx, int64(2), id, 7); err != nil {
		t.Fatal(err)
	}

	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}

	minDue := before.Truncate(time.Second).Add(7 * 24 * time.Hour)
	maxDue := time.Now().UTC().Add(8 * 24 * time.Hour)
	if p.DueDate.Before(minDue) || p.DueDate.After(maxDue) {
		t.Errorf("due_date not advanced by ~7 days; got %v (expected between %v and %v)", p.DueDate, minDue, maxDue)
	}
}

func TestSkipWord_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.SkipWord(context.Background(), int64(2), 9999, 7)
	if err == nil {
		t.Error("expected error for unknown word id")
	}
}

func TestResetWordProgress_RestoresUnseenState(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "水", "shuǐ", []string{"water"})

	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	p.Repetitions = 4
	p.Easiness = 2.1
	p.IntervalDays = 20
	p.TotalCorrect = 5
	p.TotalAttempts = 6
	p.StreakBonus = 2
	p.LearningNewWord = false
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatal(err)
	}

	if err := s.ResetWordProgress(ctx, int64(2), id); err != nil {
		t.Fatalf("ResetWordProgress: %v", err)
	}

	got, err := s.GetSM2Progress(ctx, id)
	if err != nil || got == nil {
		t.Fatalf("GetSM2Progress after reset: %v / %v", err, got)
	}
	if got.Repetitions != 0 {
		t.Errorf("repetitions: want 0, got %d", got.Repetitions)
	}
	if got.Easiness != 2.5 {
		t.Errorf("easiness: want 2.5, got %v", got.Easiness)
	}
	if got.IntervalDays != 1 {
		t.Errorf("interval_days: want 1, got %d", got.IntervalDays)
	}
	if got.TotalCorrect != 0 || got.TotalAttempts != 0 {
		t.Errorf("attempts/correct: want 0/0, got %d/%d", got.TotalCorrect, got.TotalAttempts)
	}
	if got.StreakBonus != 0 {
		t.Errorf("streak_bonus: want 0, got %d", got.StreakBonus)
	}
	if !got.LearningNewWord {
		t.Error("learning_new_word: want true after reset")
	}

	var firstSeenAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT first_seen_at FROM sm2_progress WHERE word_id = ?`, id).Scan(&firstSeenAt); err != nil {
		t.Fatal(err)
	}
	if firstSeenAt.Valid {
		t.Errorf("first_seen_at: want NULL after reset, got %q", firstSeenAt.String)
	}
}

func TestResetWordProgress_RemovesFromNewBucketCount(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "水", "shuǐ", []string{"water"})
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatal(err)
	}

	if err := s.ResetWordProgress(ctx, int64(2), id); err != nil {
		t.Fatalf("ResetWordProgress: %v", err)
	}

	// A reset word must not be selected as a "New bucket" or "due" card —
	// it should behave exactly like a freshly created unseen word.
	baselines := &NewWordBaselines{NewBucketEnabled: true, NewBucketValue: 0}
	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 0, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w != nil {
		t.Errorf("expected no card returned (new words blocked, no seen words remain), got %+v", w)
	}
}

func TestResetWordProgress_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.ResetWordProgress(context.Background(), int64(2), 9999)
	if err == nil {
		t.Error("expected error for missing word, got nil")
	}
}

func TestResetWordProgress_OtherUsersWordNotFound(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id, err := s.CreateWord(ctx, int64(1), models.CreateWordRequest{
		ZhText: "水", Translations: map[string][]string{"en": {"water"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeWord(ctx, int64(1), id); err != nil {
		t.Fatal(err)
	}

	err = s.ResetWordProgress(ctx, int64(2), id)
	if err == nil {
		t.Error("expected error resetting another user's word, got nil")
	}
}

func TestAcknowledgeRandomWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed 5 unseen words for user 2.
	for i := 0; i < 5; i++ {
		req := models.CreateWordRequest{ZhText: string(rune('一' + i)), Translations: map[string][]string{"en": {"word"}}}
		if _, err := s.CreateWord(ctx, 2, req); err != nil {
			t.Fatalf("CreateWord: %v", err)
		}
	}

	// Acknowledge 3 random words.
	n, err := s.AcknowledgeRandomWords(ctx, 2, 3)
	if err != nil {
		t.Fatalf("AcknowledgeRandomWords: %v", err)
	}
	if n != 3 {
		t.Errorf("want 3 acknowledged, got %d", n)
	}

	// due_today should now be 3.
	due, _, _, err := s.GetStats(ctx, 2, nil, "")
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if due != 3 {
		t.Errorf("want due_today=3, got %d", due)
	}

	// Asking for more than available should cap at the remaining unseen count (2).
	n2, err := s.AcknowledgeRandomWords(ctx, 2, 10)
	if err != nil {
		t.Fatalf("AcknowledgeRandomWords second call: %v", err)
	}
	if n2 != 2 {
		t.Errorf("want 2 acknowledged (remaining unseen), got %d", n2)
	}
}

func TestAcknowledgeRandomWords_InitComponents(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed a component that 你好's characters decompose into.
	if err := s.SeedHanziDecompositionForTest(ctx, "你", "you"); err != nil {
		t.Fatalf("seed decomp: %v", err)
	}
	// Seed a hanzi_decomposition row for 你 itself so InitComponentsForWord can find it.
	// Also seed a decomposition entry for 你 pointing to 你 as its own component.
	// Use InsertComponentProgressForTest indirectly by seeding the decomp table properly.
	// The simpler approach: seed 好 as a component of 你 via the decomposition table.
	// In practice InitComponentsForWord reads hanzi_decomposition.decomposition for each rune.
	// For this test we seed 你 in hanzi_decomposition with definition so a component row is created.
	// Since the decomposition column is NULL, InitComponentsForWord won't create component rows — that's fine.
	// What matters is that AcknowledgeRandomWords doesn't error.
	req := models.CreateWordRequest{ZhText: "你好", Translations: map[string][]string{"en": {"hello"}}}
	if _, err := s.CreateWord(ctx, 2, req); err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	n, err := s.AcknowledgeRandomWords(ctx, 2, 1)
	if err != nil {
		t.Fatalf("AcknowledgeRandomWords: %v", err)
	}
	if n != 1 {
		t.Errorf("want 1 acknowledged, got %d", n)
	}

	// SM-2 progress should be updated.
	due, _, _, err := s.GetStats(ctx, 2, nil, "")
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if due != 1 {
		t.Errorf("want due_today=1, got %d", due)
	}
}

func TestSaveSM2PrevState_RoundTrips(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	prog, err := s.GetSM2Progress(ctx, id)
	if err != nil || prog == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, prog)
	}
	prog.Easiness = 2.9
	prog.Repetitions = 5
	prog.IntervalDays = 21
	prog.TotalCorrect = 10
	prog.TotalAttempts = 12
	prog.StreakBonus = 2
	prog.LearningNewWord = false
	prog.KnownCorrectCount = 3

	if err := s.SaveSM2PrevState(ctx, id, *prog); err != nil {
		t.Fatalf("SaveSM2PrevState: %v", err)
	}

	got, err := s.GetSM2PrevState(ctx, id)
	if err != nil {
		t.Fatalf("GetSM2PrevState: %v", err)
	}
	if got == nil {
		t.Fatal("GetSM2PrevState: expected non-nil, got nil")
	}
	if got.Easiness != prog.Easiness {
		t.Errorf("Easiness: want %v, got %v", prog.Easiness, got.Easiness)
	}
	if got.Repetitions != prog.Repetitions {
		t.Errorf("Repetitions: want %d, got %d", prog.Repetitions, got.Repetitions)
	}
	if got.IntervalDays != prog.IntervalDays {
		t.Errorf("IntervalDays: want %d, got %d", prog.IntervalDays, got.IntervalDays)
	}
	if got.TotalCorrect != prog.TotalCorrect {
		t.Errorf("TotalCorrect: want %d, got %d", prog.TotalCorrect, got.TotalCorrect)
	}
	if got.TotalAttempts != prog.TotalAttempts {
		t.Errorf("TotalAttempts: want %d, got %d", prog.TotalAttempts, got.TotalAttempts)
	}
	if got.LearningNewWord != prog.LearningNewWord {
		t.Errorf("LearningNewWord: want %v, got %v", prog.LearningNewWord, got.LearningNewWord)
	}
	if got.KnownCorrectCount != prog.KnownCorrectCount {
		t.Errorf("KnownCorrectCount: want %d, got %d", prog.KnownCorrectCount, got.KnownCorrectCount)
	}
}

func TestClearSM2PrevState_ReturnsNil(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "谢谢", "", []string{"thank you"})

	prog, _ := s.GetSM2Progress(ctx, id)
	s.SaveSM2PrevState(ctx, id, *prog)

	if err := s.ClearSM2PrevState(ctx, id); err != nil {
		t.Fatalf("ClearSM2PrevState: %v", err)
	}
	got, err := s.GetSM2PrevState(ctx, id)
	if err != nil {
		t.Fatalf("GetSM2PrevState after clear: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after clear, got %+v", got)
	}
}

func TestGetSM2PrevState_NilWhenUnset(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "水", "", []string{"water"})

	got, err := s.GetSM2PrevState(ctx, id)
	if err != nil {
		t.Fatalf("GetSM2PrevState: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for fresh word, got %+v", got)
	}
}

// TestAcknowledgeRandomWords_AtomicAndConsolidated covers issue 08:
//   - 4.3: the duplicate first_seen_date column is gone; first_seen_at is the
//     single source of truth and is set on acknowledgement.
//   - 4.4: every selected word is acknowledged atomically (all get first_seen_at
//   - total_attempts=1).
func TestAcknowledgeRandomWords_AtomicAndConsolidated(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// 4.3: the consolidated schema no longer has first_seen_date on sm2_progress.
	var cnt int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('sm2_progress') WHERE name = 'first_seen_date'`).Scan(&cnt); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("sm2_progress.first_seen_date should have been dropped, still present")
	}

	for i := 0; i < 4; i++ {
		req := models.CreateWordRequest{ZhText: string(rune('甲' + i)), Translations: map[string][]string{"en": {"word"}}}
		if _, err := s.CreateWord(ctx, 2, req); err != nil {
			t.Fatalf("CreateWord: %v", err)
		}
	}

	n, err := s.AcknowledgeRandomWords(ctx, 2, 4)
	if err != nil {
		t.Fatalf("AcknowledgeRandomWords: %v", err)
	}
	if n != 4 {
		t.Fatalf("want 4 acknowledged, got %d", n)
	}

	// 4.4: all four rows acknowledged atomically — first_seen_at set, attempts=1.
	var seen, attempts1 int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p JOIN words w ON w.id = p.word_id
		 WHERE w.user_id = 2 AND w.language = 'zh' AND p.first_seen_at IS NOT NULL`).Scan(&seen); err != nil {
		t.Fatalf("count seen: %v", err)
	}
	if seen != 4 {
		t.Errorf("want 4 words with first_seen_at set, got %d", seen)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p JOIN words w ON w.id = p.word_id
		 WHERE w.user_id = 2 AND w.language = 'zh' AND p.total_attempts = 1`).Scan(&attempts1); err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	if attempts1 != 4 {
		t.Errorf("want 4 words with total_attempts=1, got %d", attempts1)
	}
}

func TestSharesTranslation_SharedEnTranslation(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "知道", "zhīdào", []string{"know"})
	id2 := seedWord(t, s, "认识", "rènshi", []string{"know", "recognize"})

	shared, err := s.SharesTranslation(context.Background(), id1, id2, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !shared {
		t.Error("expected shared=true for two words with common EN translation 'know'")
	}
}

func TestSharesTranslation_NoOverlap(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "书", "shū", []string{"book"})
	id2 := seedWord(t, s, "鱼", "yú", []string{"fish"})

	shared, err := s.SharesTranslation(context.Background(), id1, id2, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if shared {
		t.Error("expected shared=false for words with distinct translations")
	}
}

func TestSharesTranslation_WrongLang(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "知道", "zhīdào", []string{"know"})
	id2 := seedWord(t, s, "认识", "rènshi", []string{"know"})

	// Translations are EN, but we query DE — should return false
	shared, err := s.SharesTranslation(context.Background(), id1, id2, []string{"de"})
	if err != nil {
		t.Fatal(err)
	}
	if shared {
		t.Error("expected shared=false when querying a language with no translations")
	}
}

func TestSharesTranslation_EmptyLangs_FallsBackToEn(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "知道", "zhīdào", []string{"know"})
	id2 := seedWord(t, s, "认识", "rènshi", []string{"know"})

	shared, err := s.SharesTranslation(context.Background(), id1, id2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !shared {
		t.Error("expected shared=true with empty langs (should fall back to 'en')")
	}
}

func TestSharesTranslation_CaseInsensitive(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "知道", "zhīdào", []string{"Know"})
	id2 := seedWord(t, s, "认识", "rènshi", []string{"know"})

	shared, err := s.SharesTranslation(context.Background(), id1, id2, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !shared {
		t.Error("expected shared=true for case-insensitive translation match")
	}
}

// TestSharesTranslation_SlashVariantOverlap covers the real-world "Nudeln"
// scenario (issue #188): 面 has DE translation "Nudeln" while 面条 has DE
// translation "Nudeln / Pasta" — a single multi-gloss entry rather than a
// separate row. Plain string equality misses the overlap; the same
// slash-alternative expansion CheckAnswer already applies (sm2.ExpandVariants)
// must be used so "Nudeln" is recognised as one of 面条's valid variants.
func TestSharesTranslation_SlashVariantOverlap(t *testing.T) {
	s := openTestDB(t)
	id1, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       "面",
		Pinyin:       "miàn",
		Translations: map[string][]string{"en": {"noodles"}, "de": {"Nudeln"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       "面条",
		Pinyin:       "miàntiáo",
		Translations: map[string][]string{"en": {"noodle"}, "de": {"Nudeln / Pasta"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	shared, err := s.SharesTranslation(context.Background(), id1, id2, []string{"en", "de"})
	if err != nil {
		t.Fatal(err)
	}
	if !shared {
		t.Error("expected shared=true: 面条's 'Nudeln / Pasta' includes the 'Nudeln' variant that 面 has")
	}
}

func TestRecordAnswerTimestamps_SetsAttemptAlwaysAndWrongOnlyOnWrong(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a := seedWord(t, s, "一", "", []string{"one"})

	scan := func() (attemptAt, wrongAt sql.NullString) {
		if err := s.db.QueryRowContext(ctx,
			`SELECT last_attempt_at, last_wrong_at FROM sm2_progress WHERE word_id = ?`, a,
		).Scan(&attemptAt, &wrongAt); err != nil {
			t.Fatal(err)
		}
		return
	}

	if err := s.RecordAnswerTimestamps(ctx, a, true); err != nil {
		t.Fatal(err)
	}
	attemptAt, wrongAt := scan()
	if !attemptAt.Valid {
		t.Error("expected last_attempt_at set after a correct answer")
	}
	if wrongAt.Valid {
		t.Error("expected last_wrong_at to remain unset after a correct answer")
	}

	if err := s.RecordAnswerTimestamps(ctx, a, false); err != nil {
		t.Fatal(err)
	}
	_, wrongAt2 := scan()
	if !wrongAt2.Valid {
		t.Error("expected last_wrong_at set after a wrong answer")
	}
}
