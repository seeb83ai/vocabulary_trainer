package db

import (
	"context"
	"fmt"
)

// RecordAnswerBuckets counts one answer in each of the given buckets, in
// today's daily row and this month's monthly row. It also deletes the user's
// daily rows that are older than 30 days.
func (s *Store) RecordAnswerBuckets(ctx context.Context, userID, wordID int64, buckets []string, correct bool) error {
	errInc := 1
	if correct {
		errInc = 0
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("record answer buckets: %w", err)
	}
	defer tx.Rollback()
	for _, b := range buckets {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO answer_stats_daily (user_id, day, word_id, bucket, attempts, errors)
			 VALUES (?, date('now'), ?, ?, 1, ?)
			 ON CONFLICT(user_id, day, word_id, bucket) DO UPDATE SET
			   attempts = attempts + 1, errors = errors + excluded.errors`,
			userID, wordID, b, errInc); err != nil {
			return fmt.Errorf("record daily answer bucket: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO answer_stats_monthly (user_id, month, word_id, bucket, attempts, errors)
			 VALUES (?, strftime('%Y-%m', 'now'), ?, ?, 1, ?)
			 ON CONFLICT(user_id, month, word_id, bucket) DO UPDATE SET
			   attempts = attempts + 1, errors = errors + excluded.errors`,
			userID, wordID, b, errInc); err != nil {
			return fmt.Errorf("record monthly answer bucket: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM answer_stats_daily WHERE user_id = ? AND day < date('now', '-30 days')`, userID); err != nil {
		return fmt.Errorf("prune daily answer buckets: %w", err)
	}
	return tx.Commit()
}

// AcceptAnswerBuckets turns one wrong answer from today into a correct one
// ("accept as correct"): it removes one error from today's daily row and this
// month's monthly row in each bucket. Attempts stay the same.
func (s *Store) AcceptAnswerBuckets(ctx context.Context, userID, wordID int64, buckets []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("accept answer buckets: %w", err)
	}
	defer tx.Rollback()
	for _, b := range buckets {
		if _, err := tx.ExecContext(ctx,
			`UPDATE answer_stats_daily SET errors = MAX(errors - 1, 0)
			 WHERE user_id = ? AND day = date('now') AND word_id = ? AND bucket = ?`,
			userID, wordID, b); err != nil {
			return fmt.Errorf("accept daily answer bucket: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE answer_stats_monthly SET errors = MAX(errors - 1, 0)
			 WHERE user_id = ? AND month = strftime('%Y-%m', 'now') AND word_id = ? AND bucket = ?`,
			userID, wordID, b); err != nil {
			return fmt.Errorf("accept monthly answer bucket: %w", err)
		}
	}
	return tx.Commit()
}

// AnswerBucketCount holds the counters of one answer bucket.
type AnswerBucketCount struct{ Attempts, Errors int }

// TodayAnswerBucketsForTest returns today's daily counters per bucket for one
// word. Intended for use in tests only.
func (s *Store) TodayAnswerBucketsForTest(ctx context.Context, wordID int64) (map[string]AnswerBucketCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT bucket, attempts, errors FROM answer_stats_daily WHERE word_id = ? AND day = date('now')`, wordID)
	if err != nil {
		return nil, fmt.Errorf("today answer buckets: %w", err)
	}
	defer rows.Close()
	got := map[string]AnswerBucketCount{}
	for rows.Next() {
		var b string
		var c AnswerBucketCount
		if err := rows.Scan(&b, &c.Attempts, &c.Errors); err != nil {
			return nil, fmt.Errorf("scan answer bucket: %w", err)
		}
		got[b] = c
	}
	return got, rows.Err()
}
