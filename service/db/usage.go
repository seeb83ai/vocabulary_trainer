package db

import (
	"context"
	"fmt"
	"time"
)

// RecordUsageEvent upserts a (userID, name) usage_events row, incrementing
// count and refreshing last_seen. userID 0 represents an anonymous request.
func (s *Store) RecordUsageEvent(ctx context.Context, userID int64, name string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO usage_events (user_id, name, count, last_seen)
		VALUES (?, ?, 1, ?)
		ON CONFLICT(user_id, name) DO UPDATE SET
			count     = count + 1,
			last_seen = excluded.last_seen`,
		userID, name, time.Now().UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("record usage event: %w", err)
	}
	return nil
}

// GetUsageEventForTest reads a usage_events row directly. Intended for use in
// tests only.
func (s *Store) GetUsageEventForTest(ctx context.Context, userID int64, name string) (count int, lastSeen string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT count, last_seen FROM usage_events WHERE user_id = ? AND name = ?`,
		userID, name).Scan(&count, &lastSeen)
	return count, lastSeen, err
}

// CountUsageEventsForTest returns the total number of usage_events rows.
// Intended for use in tests only.
func (s *Store) CountUsageEventsForTest(ctx context.Context) (int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_events`).Scan(&total)
	return total, err
}
