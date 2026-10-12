package db

import (
	"context"
	"testing"
	"time"
)

type bucketCount struct{ attempts, errors int }

// answerStats reads the counters per bucket for one word from the daily
// table (period = day) or the monthly table (period = month).
func answerStats(t *testing.T, s *Store, table, periodCol, period string, wordID int64) map[string]bucketCount {
	t.Helper()
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT bucket, attempts, errors FROM `+table+` WHERE word_id = ? AND `+periodCol+` = ?`, wordID, period)
	if err != nil {
		t.Fatalf("query %s: %v", table, err)
	}
	defer rows.Close()
	got := map[string]bucketCount{}
	for rows.Next() {
		var b string
		var c bucketCount
		if err := rows.Scan(&b, &c.attempts, &c.errors); err != nil {
			t.Fatalf("scan %s: %v", table, err)
		}
		got[b] = c
	}
	return got
}

func today() string     { return time.Now().UTC().Format("2006-01-02") }
func thisMonth() string { return time.Now().UTC().Format("2006-01") }

func TestRecordAnswerBuckets_WrongAnswerInTwoBuckets(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	if err := s.RecordAnswerBuckets(ctx, 2, id, []string{"pinyin", "voice"}, false); err != nil {
		t.Fatalf("RecordAnswerBuckets: %v", err)
	}

	want := map[string]bucketCount{"pinyin": {1, 1}, "voice": {1, 1}}
	if got := answerStats(t, s, "answer_stats_daily", "day", today(), id); len(got) != 2 || got["pinyin"] != want["pinyin"] || got["voice"] != want["voice"] {
		t.Errorf("daily: got %v, want %v", got, want)
	}
	if got := answerStats(t, s, "answer_stats_monthly", "month", thisMonth(), id); len(got) != 2 || got["pinyin"] != want["pinyin"] || got["voice"] != want["voice"] {
		t.Errorf("monthly: got %v, want %v", got, want)
	}
}

func TestRecordAnswerBuckets_CorrectAnswerAddsAttemptOnly(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	for _, correct := range []bool{false, true, true} {
		if err := s.RecordAnswerBuckets(ctx, 2, id, []string{"zh_only"}, correct); err != nil {
			t.Fatalf("RecordAnswerBuckets: %v", err)
		}
	}

	want := bucketCount{3, 1}
	if got := answerStats(t, s, "answer_stats_daily", "day", today(), id)["zh_only"]; got != want {
		t.Errorf("daily zh_only: got %v, want %v", got, want)
	}
	if got := answerStats(t, s, "answer_stats_monthly", "month", thisMonth(), id)["zh_only"]; got != want {
		t.Errorf("monthly zh_only: got %v, want %v", got, want)
	}
}

func TestRecordAnswerBuckets_PrunesDailyRowsOlderThan30Days(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	day := func(daysAgo int) string { return time.Now().UTC().AddDate(0, 0, -daysAgo).Format("2006-01-02") }
	for _, d := range []string{day(31), day(30)} {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO answer_stats_daily (user_id, day, word_id, bucket, attempts, errors) VALUES (2, ?, ?, 'zh_only', 1, 1)`,
			d, id); err != nil {
			t.Fatalf("seed daily row: %v", err)
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO answer_stats_monthly (user_id, month, word_id, bucket, attempts, errors) VALUES (2, '2020-01', ?, 'zh_only', 1, 1)`,
		id); err != nil {
		t.Fatalf("seed monthly row: %v", err)
	}

	if err := s.RecordAnswerBuckets(ctx, 2, id, []string{"zh_only"}, true); err != nil {
		t.Fatalf("RecordAnswerBuckets: %v", err)
	}

	if got := answerStats(t, s, "answer_stats_daily", "day", day(31), id); len(got) != 0 {
		t.Errorf("row from 31 days ago should be deleted, got %v", got)
	}
	if got := answerStats(t, s, "answer_stats_daily", "day", day(30), id); len(got) != 1 {
		t.Errorf("row from 30 days ago should be kept, got %v", got)
	}
	if got := answerStats(t, s, "answer_stats_monthly", "month", "2020-01", id); len(got) != 1 {
		t.Errorf("monthly rows should be kept, got %v", got)
	}
}

func TestAcceptAnswerBuckets_RemovesTheError(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	if err := s.RecordAnswerBuckets(ctx, 2, id, []string{"pinyin", "voice"}, false); err != nil {
		t.Fatalf("RecordAnswerBuckets: %v", err)
	}
	// The second accept has no error left to remove; counts must not go negative.
	for i := 0; i < 2; i++ {
		if err := s.AcceptAnswerBuckets(ctx, 2, id, []string{"pinyin", "voice"}); err != nil {
			t.Fatalf("AcceptAnswerBuckets: %v", err)
		}
	}

	want := bucketCount{1, 0}
	for _, b := range []string{"pinyin", "voice"} {
		if got := answerStats(t, s, "answer_stats_daily", "day", today(), id)[b]; got != want {
			t.Errorf("daily %s: got %v, want %v", b, got, want)
		}
		if got := answerStats(t, s, "answer_stats_monthly", "month", thisMonth(), id)[b]; got != want {
			t.Errorf("monthly %s: got %v, want %v", b, got, want)
		}
	}
}

func TestAnswerBuckets_DeletedWithTheWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	if err := s.RecordAnswerBuckets(ctx, 2, id, []string{"zh_only"}, false); err != nil {
		t.Fatalf("RecordAnswerBuckets: %v", err)
	}
	if err := s.DeleteWord(ctx, 2, id); err != nil {
		t.Fatalf("DeleteWord: %v", err)
	}

	if got := answerStats(t, s, "answer_stats_daily", "day", today(), id); len(got) != 0 {
		t.Errorf("daily rows should be deleted with the word, got %v", got)
	}
	if got := answerStats(t, s, "answer_stats_monthly", "month", thisMonth(), id); len(got) != 0 {
		t.Errorf("monthly rows should be deleted with the word, got %v", got)
	}
}
