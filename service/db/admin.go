package db

import (
	"context"
	"fmt"
	"strings"
	"vocabulary_trainer/models"
)

// GetAdminOverview aggregates cross-user usage data for the admin dashboard:
// account counts by role/verification, registration and activity trends,
// aggregate quiz volume, guest (unauthenticated) activity from audit_log,
// and per-route usage_events broken into page views vs. API features
// (with DeepL/LLM call volume singled out).
func (s *Store) GetAdminOverview(ctx context.Context) (*models.AdminOverview, error) {
	ov := &models.AdminOverview{
		Signups:       []models.DueDateCount{},
		QuizVolume:    []models.AdminQuizDay{},
		GuestActivity: []models.DueDateCount{},
		PageViews:     []models.AdminFeatureUsage{},
		FeatureUsage:  []models.AdminFeatureUsage{},
	}

	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN role = 'admin' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN role = 'plus' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN role = 'free' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN email_verified = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN email_verified != 1 THEN 1 ELSE 0 END), 0)
		FROM users`).Scan(
		&ov.Users.Total, &ov.Users.Admins, &ov.Users.Plus, &ov.Users.Free,
		&ov.Users.Verified, &ov.Users.Unverified,
	); err != nil {
		return nil, fmt.Errorf("get admin user stats: %w", err)
	}

	signupRows, err := s.db.QueryContext(ctx, `
		SELECT date(created_at), COUNT(*)
		FROM users
		WHERE created_at >= datetime('now', '-30 days')
		GROUP BY date(created_at)
		ORDER BY date(created_at)`)
	if err != nil {
		return nil, fmt.Errorf("get admin signups: %w", err)
	}
	for signupRows.Next() {
		var d models.DueDateCount
		if err := signupRows.Scan(&d.Date, &d.Count); err != nil {
			signupRows.Close()
			return nil, fmt.Errorf("scan admin signup: %w", err)
		}
		ov.Signups = append(ov.Signups, d)
	}
	if err := signupRows.Err(); err != nil {
		signupRows.Close()
		return nil, err
	}
	signupRows.Close()

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT user_id) FROM daily_stats WHERE date >= date('now', '-7 days')`,
	).Scan(&ov.Activity.ActiveLast7Days); err != nil {
		return nil, fmt.Errorf("count active users (7d): %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT user_id) FROM daily_stats WHERE date >= date('now', '-30 days')`,
	).Scan(&ov.Activity.ActiveLast30Days); err != nil {
		return nil, fmt.Errorf("count active users (30d): %w", err)
	}
	ov.Activity.Dormant = ov.Users.Total - ov.Activity.ActiveLast30Days

	quizRows, err := s.db.QueryContext(ctx, `
		SELECT date, COALESCE(SUM(attempts), 0), COALESCE(SUM(mistakes), 0)
		FROM daily_stats
		WHERE date >= date('now', '-30 days')
		GROUP BY date
		ORDER BY date`)
	if err != nil {
		return nil, fmt.Errorf("get admin quiz volume: %w", err)
	}
	for quizRows.Next() {
		var d models.AdminQuizDay
		if err := quizRows.Scan(&d.Date, &d.Attempts, &d.Mistakes); err != nil {
			quizRows.Close()
			return nil, fmt.Errorf("scan admin quiz day: %w", err)
		}
		ov.QuizVolume = append(ov.QuizVolume, d)
	}
	if err := quizRows.Err(); err != nil {
		quizRows.Close()
		return nil, err
	}
	quizRows.Close()

	guestRows, err := s.db.QueryContext(ctx, `
		SELECT date(created_at), COUNT(*)
		FROM audit_log
		WHERE user_id = 0 AND action = ? AND created_at >= datetime('now', '-30 days')
		GROUP BY date(created_at)
		ORDER BY date(created_at)`, AuditLoginFailure)
	if err != nil {
		return nil, fmt.Errorf("get admin guest activity: %w", err)
	}
	for guestRows.Next() {
		var d models.DueDateCount
		if err := guestRows.Scan(&d.Date, &d.Count); err != nil {
			guestRows.Close()
			return nil, fmt.Errorf("scan admin guest activity: %w", err)
		}
		ov.GuestActivity = append(ov.GuestActivity, d)
	}
	if err := guestRows.Err(); err != nil {
		guestRows.Close()
		return nil, err
	}
	guestRows.Close()

	usageRows, err := s.db.QueryContext(ctx, `
		SELECT name, SUM(count), COUNT(DISTINCT user_id), MAX(last_seen)
		FROM usage_events
		GROUP BY name
		ORDER BY SUM(count) DESC`)
	if err != nil {
		return nil, fmt.Errorf("get admin feature usage: %w", err)
	}
	for usageRows.Next() {
		var fu models.AdminFeatureUsage
		if err := usageRows.Scan(&fu.Name, &fu.TotalCount, &fu.UniqueUsers, &fu.LastSeen); err != nil {
			usageRows.Close()
			return nil, fmt.Errorf("scan admin feature usage: %w", err)
		}
		switch {
		case fu.Name == "POST /api/translate":
			ov.DeepLUsage.TotalCalls += fu.TotalCount
			ov.DeepLUsage.UniqueUsers += fu.UniqueUsers
			ov.FeatureUsage = append(ov.FeatureUsage, fu)
		case strings.HasSuffix(fu.Name, "/hmm/generate-scene"):
			ov.LLMUsage.TotalCalls += fu.TotalCount
			ov.LLMUsage.UniqueUsers += fu.UniqueUsers
			ov.FeatureUsage = append(ov.FeatureUsage, fu)
		case strings.Contains(fu.Name, "/api/"):
			ov.FeatureUsage = append(ov.FeatureUsage, fu)
		default:
			ov.PageViews = append(ov.PageViews, fu)
		}
	}
	if err := usageRows.Err(); err != nil {
		usageRows.Close()
		return nil, err
	}
	usageRows.Close()

	return ov, nil
}
