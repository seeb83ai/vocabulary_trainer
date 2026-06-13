package db

import (
	"context"
	"fmt"
	"time"
)

// IncrementFailedLogins bumps the failed-login counter for the user and
// returns the new value. Use after a wrong-password attempt.
func (s *Store) IncrementFailedLogins(ctx context.Context, userID int64) (int, error) {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET failed_logins = failed_logins + 1 WHERE id = ?`, userID); err != nil {
		return 0, fmt.Errorf("increment failed_logins: %w", err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT failed_logins FROM users WHERE id = ?`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("read failed_logins: %w", err)
	}
	return n, nil
}

// GetFailedLogins returns the current counter and lockout timestamp. A
// zero-time lockoutUntil means no lockout is set.
func (s *Store) GetFailedLogins(ctx context.Context, userID int64) (int, time.Time, error) {
	var n int
	var lockoutUntil string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(failed_logins, 0), COALESCE(lockout_until, '') FROM users WHERE id = ?`, userID).
		Scan(&n, &lockoutUntil)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("get failed_logins: %w", err)
	}
	if lockoutUntil == "" {
		return n, time.Time{}, nil
	}
	return n, parseDateTime(lockoutUntil), nil
}

// LockAccountUntil sets the lockout_until timestamp.
func (s *Store) LockAccountUntil(ctx context.Context, userID int64, until time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET lockout_until = ? WHERE id = ?`,
		until.UTC().Format("2006-01-02 15:04:05"), userID)
	if err != nil {
		return fmt.Errorf("lock account: %w", err)
	}
	return nil
}

// IsAccountLocked returns (true, lockoutUntil) if the user has an active
// lockout window. An expired lockout is reported as not-locked but is
// not cleared from the row (ResetFailedLogins does that on a successful
// login).
func (s *Store) IsAccountLocked(ctx context.Context, userID int64) (bool, time.Time, error) {
	_, lockoutUntil, err := s.GetFailedLogins(ctx, userID)
	if err != nil {
		return false, time.Time{}, err
	}
	if lockoutUntil.IsZero() {
		return false, time.Time{}, nil
	}
	if lockoutUntil.Before(time.Now().UTC()) {
		return false, lockoutUntil, nil
	}
	return true, lockoutUntil, nil
}

// ResetFailedLogins zeroes the counter and clears any lockout window.
// Call on a successful login.
func (s *Store) ResetFailedLogins(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET failed_logins = 0, lockout_until = '' WHERE id = ?`, userID)
	if err != nil {
		return fmt.Errorf("reset failed_logins: %w", err)
	}
	return nil
}
