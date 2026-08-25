package db

import (
	"context"
	"fmt"
	"vocabulary_trainer/models"
)

// GetFunnelReport computes the signup → activation → retention funnel from
// the users and daily_stats tables. minAttempts is the total-attempts
// threshold for the Engaged stage. The shared library user (id=1) is
// excluded from every count.
func (s *Store) GetFunnelReport(ctx context.Context, minAttempts int) (models.FunnelReport, error) {
	var r models.FunnelReport

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(CASE WHEN COALESCE(email_verified, 0) = 1 THEN 1 ELSE 0 END), 0)
		 FROM users WHERE id <> 1`).Scan(&r.Registered, &r.Verified); err != nil {
		return r, fmt.Errorf("count registered/verified users: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT user_id) FROM daily_stats WHERE user_id <> 1`).
		Scan(&r.Activated); err != nil {
		return r, fmt.Errorf("count activated users: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM (
		   SELECT user_id FROM daily_stats WHERE user_id <> 1
		   GROUP BY user_id HAVING SUM(attempts) >= ?
		 )`, minAttempts).Scan(&r.Engaged); err != nil {
		return r, fmt.Errorf("count engaged users: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM (
		   SELECT user_id FROM daily_stats WHERE user_id <> 1
		   GROUP BY user_id HAVING COUNT(DISTINCT date) >= 2
		 )`).Scan(&r.Returned); err != nil {
		return r, fmt.Errorf("count returned users: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM (
		   SELECT user_id, MIN(date) AS first_day
		   FROM daily_stats WHERE user_id <> 1
		   GROUP BY user_id
		 ) f
		 WHERE EXISTS (
		   SELECT 1 FROM daily_stats d
		   WHERE d.user_id = f.user_id AND d.date = date(f.first_day, '+1 day')
		 )`).Scan(&r.ReturnedDay1); err != nil {
		return r, fmt.Errorf("count day-1 returns: %w", err)
	}

	return r, nil
}
