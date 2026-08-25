package db

import (
	"context"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

// words_seen and bucket counts must reflect only the calling user's words,
// not every user's aggregate.
func TestRecordDailyStat_UserIsolation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// User 2 (created by openTestDB) has one seen word.
	id2 := seedWord(t, s, "你好", "", []string{"hello"})
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET first_seen_at = date('now') WHERE word_id = ?`, id2); err != nil {
		t.Fatal(err)
	}

	// User 3 has a separate seen word.
	user3ID, err := s.CreateUser(ctx, "user3@example.com", "hash", "tok-u3", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	id3, err := s.CreateWord(ctx, user3ID, models.CreateWordRequest{
		ZhText: "再见", Translations: map[string][]string{"en": {"goodbye"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET first_seen_at = date('now') WHERE word_id = ?`, id3); err != nil {
		t.Fatal(err)
	}

	if _, err := s.RecordDailyStat(ctx, int64(2), true); err != nil {
		t.Fatal(err)
	}

	hist, err := s.GetDailyStatsHistory(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) == 0 {
		t.Fatal("expected a daily_stats row for user 2")
	}
	if hist[len(hist)-1].WordsSeen != 1 {
		t.Errorf("user 2 words_seen = %d, want 1 (must not count other users' words)", hist[len(hist)-1].WordsSeen)
	}
}

func TestEnsureDueTodaySnapshot_RecordsCount(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// No seen words yet — snapshot should be 0.
	count, err := s.EnsureDueTodaySnapshot(ctx, userID)
	if err != nil {
		t.Fatalf("EnsureDueTodaySnapshot: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0 due words, got %d", count)
	}
}

func TestEnsureDueTodaySnapshot_IdempotentOnSecondCall(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	first, err := s.EnsureDueTodaySnapshot(ctx, userID)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := s.EnsureDueTodaySnapshot(ctx, userID)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Errorf("snapshot should be stable: first=%d second=%d", first, second)
	}
}

func TestGetWordStats_AccBucketsFilterByTag(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	tagged := seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"vip"})
	other := seedWordWithTags(t, s, "再见", "", []string{"bye"}, []string{"other"})

	// Push both words into the "85-100" (mastered) accuracy bucket.
	for _, id := range []int64{tagged, other} {
		if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
			t.Fatal(err)
		}
		p, err := s.GetSM2Progress(ctx, id)
		if err != nil || p == nil {
			t.Fatalf("GetSM2Progress: %v / %v", err, p)
		}
		p.LearningNewWord = false
		p.TotalAttempts = 10
		p.TotalCorrect = 10
		p.StreakBonus = 0
		if err := s.UpdateSM2Progress(ctx, *p); err != nil {
			t.Fatal(err)
		}
	}

	// Regression: an empty/absent tag filter must be unchanged from current behavior.
	all, err := s.GetWordStats(ctx, int64(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if all.AccBuckets["85-100"] != 2 {
		t.Errorf("unfiltered 85-100 bucket: want 2, got %d", all.AccBuckets["85-100"])
	}
	if all.TotalSeen != 2 {
		t.Errorf("unfiltered total_seen: want 2, got %d", all.TotalSeen)
	}

	// Tag-filtered: only the "vip"-tagged word should be counted in AccBuckets.
	filtered, err := s.GetWordStats(ctx, int64(2), []string{"vip"})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.AccBuckets["85-100"] != 1 {
		t.Errorf("tag-filtered 85-100 bucket: want 1, got %d", filtered.AccBuckets["85-100"])
	}
	if filtered.AccBuckets["0-49"] != 0 {
		t.Errorf("tag-filtered 0-49 bucket: want 0, got %d", filtered.AccBuckets["0-49"])
	}

	// Other stats sections must stay unaffected by the bucket-only tag filter.
	if filtered.TotalSeen != all.TotalSeen {
		t.Errorf("total_seen should be unaffected by tag filter: want %d, got %d", all.TotalSeen, filtered.TotalSeen)
	}
	if len(filtered.MostPract) != len(all.MostPract) {
		t.Errorf("most_practiced should be unaffected by tag filter: want %d entries, got %d", len(all.MostPract), len(filtered.MostPract))
	}
}

func TestRecordDailyStat_IncrementsCounts(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedWord(t, s, "猫", "māo", []string{"cat"})

	// Mark the word as seen and meeting the "known" threshold (≥10 attempts, ≥85% accuracy)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET first_seen_at = date('now'), total_correct = 9, total_attempts = 10`); err != nil {
		t.Fatal(err)
	}

	if _, err := s.RecordDailyStat(ctx, int64(2), true); err != nil {
		t.Fatalf("RecordDailyStat(correct): %v", err)
	}
	if _, err := s.RecordDailyStat(ctx, int64(2), true); err != nil {
		t.Fatalf("RecordDailyStat(correct): %v", err)
	}
	if _, err := s.RecordDailyStat(ctx, int64(2), false); err != nil {
		t.Fatalf("RecordDailyStat(wrong): %v", err)
	}

	stats, err := s.GetDailyStatsHistory(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetDailyStatsHistory: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 day, got %d", len(stats))
	}
	d := stats[0]
	if d.Attempts != 3 {
		t.Errorf("attempts: got %d, want 3", d.Attempts)
	}
	if d.Mistakes != 1 {
		t.Errorf("mistakes: got %d, want 1", d.Mistakes)
	}
	if d.WordsSeen != 1 {
		t.Errorf("words_seen: got %d, want 1", d.WordsSeen)
	}
	if d.CorrectStreak != 2 {
		t.Errorf("correct_streak: got %d, want 2", d.CorrectStreak)
	}
}

func TestRecordDailyStat_StreakResets(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// wrong, correct, correct, wrong, correct
	for _, correct := range []bool{false, true, true, false, true} {
		if _, err := s.RecordDailyStat(ctx, int64(2), correct); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := s.GetDailyStatsHistory(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if stats[0].CorrectStreak != 2 {
		t.Errorf("correct_streak: got %d, want 2 (max streak of the day)", stats[0].CorrectStreak)
	}
}

func TestGetDailyStatsHistory_OrderedByDate(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Insert rows for multiple dates manually
	for _, d := range []string{"2026-02-10", "2026-02-12", "2026-02-11"} {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO daily_stats (user_id, date, attempts, mistakes, correct_streak, current_streak)
			 VALUES (2, ?, 10, 2, 3, 0)`, d); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := s.GetDailyStatsHistory(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(stats))
	}
	if stats[0].Date != "2026-02-10" || stats[1].Date != "2026-02-11" || stats[2].Date != "2026-02-12" {
		t.Errorf("wrong order: %s, %s, %s", stats[0].Date, stats[1].Date, stats[2].Date)
	}
}

func TestGetDailyStatsHistory_EmptyReturnsEmptySlice(t *testing.T) {
	s := openTestDB(t)
	stats, err := s.GetDailyStatsHistory(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if stats == nil {
		t.Error("expected non-nil empty slice")
	}
	if len(stats) != 0 {
		t.Errorf("expected 0 rows, got %d", len(stats))
	}
}

func TestGetTodaySessionInfo_NoRows(t *testing.T) {
	s := openTestDB(t)
	attempts, mistakes, available, err := s.GetTodaySessionInfo(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || mistakes != 0 {
		t.Errorf("expected 0/0, got %d/%d", attempts, mistakes)
	}
	if available != 0 {
		t.Errorf("expected 0 available, got %d", available)
	}
}

func TestGetTodaySessionInfo_WithData(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	// Mark the word as seen with a future due date.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET first_seen_at = date('now'), due_date = datetime('now', '+1 day') WHERE word_id = ?`, id); err != nil {
		t.Fatal(err)
	}

	// Record a daily stat (1 correct answer).
	if _, err := s.RecordDailyStat(ctx, int64(2), true); err != nil {
		t.Fatal(err)
	}

	attempts, mistakes, available, err := s.GetTodaySessionInfo(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
	if mistakes != 0 {
		t.Errorf("expected 0 mistakes, got %d", mistakes)
	}
	if available != 1 {
		t.Errorf("expected 1 available to advance, got %d", available)
	}
}

func TestRecordTrainingTime_Accumulates(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if err := s.RecordTrainingTime(ctx, int64(2), 45); err != nil {
		t.Fatalf("first RecordTrainingTime: %v", err)
	}
	if err := s.RecordTrainingTime(ctx, int64(2), 30); err != nil {
		t.Fatalf("second RecordTrainingTime: %v", err)
	}

	stats, err := s.GetDailyStatsHistory(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetDailyStatsHistory: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 day, got %d", len(stats))
	}
	if stats[0].TrainingSeconds != 75 {
		t.Errorf("training_seconds: want 75, got %d", stats[0].TrainingSeconds)
	}
}

func TestRecordTrainingTime_CreatesRowWhenNoneExists(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if err := s.RecordTrainingTime(ctx, int64(2), 10); err != nil {
		t.Fatalf("RecordTrainingTime: %v", err)
	}

	stats, err := s.GetDailyStatsHistory(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetDailyStatsHistory: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 day, got %d", len(stats))
	}
	if stats[0].TrainingSeconds != 10 {
		t.Errorf("training_seconds: want 10, got %d", stats[0].TrainingSeconds)
	}
}

func TestAdvanceDueDates_AdvancesNWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed 5 words and mark them as seen with staggered future due dates.
	ids := make([]int64, 5)
	for i := range ids {
		ids[i] = seedWord(t, s, []string{"一", "二", "三", "四", "五"}[i], "", []string{"en"})
		days := i + 1 // 1 day, 2 days, ..., 5 days from now
		if _, err := s.db.ExecContext(ctx,
			`UPDATE sm2_progress SET first_seen_at = date('now'), due_date = datetime('now', ? || ' days') WHERE word_id = ?`,
			days, ids[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Advance 3 words (the 3rd earliest due date is +3 days).
	nowDue, err := s.AdvanceDueDates(ctx, int64(2), 3)
	if err != nil {
		t.Fatal(err)
	}
	if nowDue != 3 {
		t.Errorf("expected 3 words due now, got %d", nowDue)
	}

	// Verify exactly 3 are due and 2 are still future.
	var due, future int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress WHERE due_date <= CURRENT_TIMESTAMP AND first_seen_at IS NOT NULL`).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress WHERE due_date > CURRENT_TIMESTAMP AND first_seen_at IS NOT NULL`).Scan(&future); err != nil {
		t.Fatal(err)
	}
	if due != 3 {
		t.Errorf("expected 3 due, got %d", due)
	}
	if future != 2 {
		t.Errorf("expected 2 future, got %d", future)
	}
}

func TestAdvanceDueDates_FewerThanN(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Only 2 seen words with future due dates.
	for i, zh := range []string{"一", "二"} {
		id := seedWord(t, s, zh, "", []string{"en"})
		if _, err := s.db.ExecContext(ctx,
			`UPDATE sm2_progress SET first_seen_at = date('now'), due_date = datetime('now', ? || ' days') WHERE word_id = ?`,
			i+1, id); err != nil {
			t.Fatal(err)
		}
	}

	// Request 10 but only 2 available — advance whatever is available.
	nowDue, err := s.AdvanceDueDates(ctx, int64(2), 10)
	if err != nil {
		t.Fatal(err)
	}
	if nowDue != 2 {
		t.Errorf("expected 2 (all available), got %d", nowDue)
	}
}

func TestAdvanceDueDates_ClusteredDueDates_OnlyAdvancesN(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed 15 words all with the same future due date (+1 day).
	// This simulates words that all completed their first SM-2 interval at roughly
	// the same time (e.g. user trained all words in one session).
	for i := 0; i < 15; i++ {
		id := seedWord(t, s, string(rune('a'+i)), "", []string{"en"})
		if _, err := s.db.ExecContext(ctx,
			`UPDATE sm2_progress SET first_seen_at = date('now'), due_date = datetime('now', '+1 day') WHERE word_id = ?`,
			id); err != nil {
			t.Fatal(err)
		}
	}

	// Advance 5 words — must not advance all 15.
	nowDue, err := s.AdvanceDueDates(ctx, int64(2), 5)
	if err != nil {
		t.Fatal(err)
	}
	if nowDue != 5 {
		t.Errorf("expected exactly 5 words due now, got %d", nowDue)
	}

	var future int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress WHERE due_date > CURRENT_TIMESTAMP AND first_seen_at IS NOT NULL`).Scan(&future); err != nil {
		t.Fatal(err)
	}
	if future != 10 {
		t.Errorf("expected 10 words still in the future, got %d", future)
	}
}
