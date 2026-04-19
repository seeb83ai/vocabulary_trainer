package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"vocabulary_trainer/models"
)

// GetUserByEmail looks up a user by email address. Returns nil, nil if not found.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	var verified int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(email_verified,0) FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.EmailVerified = verified == 1
	return &u, nil
}

// GetUserByID returns a user by primary key. Returns nil, nil if not found.
func (s *Store) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	var u models.User
	var verified int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(email_verified,0) FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	u.EmailVerified = verified == 1
	return &u, nil
}

// CreateUser inserts a new unverified user and returns its ID.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash, verificationToken string, expiresAt time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, email_verified, verification_token, verification_expires_at)
		 VALUES (?, ?, 0, ?, ?)`,
		email, passwordHash, verificationToken, expiresAt.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}
	return res.LastInsertId()
}

// SetUserEmailVerified finds a user by verification token (not expired), marks
// email_verified = 1, clears the token, and returns the user. Returns nil, nil
// if the token is unknown or expired.
func (s *Store) SetUserEmailVerified(ctx context.Context, token string) (*models.User, error) {
	var u models.User
	var verified int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(email_verified,0)
		 FROM users
		 WHERE verification_token = ?
		   AND verification_expires_at > datetime('now')`, token).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user by token: %w", err)
	}
	u.EmailVerified = verified == 1
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET email_verified = 1, verification_token = NULL, verification_expires_at = NULL
		 WHERE id = ?`, u.ID); err != nil {
		return nil, fmt.Errorf("verify user: %w", err)
	}
	u.EmailVerified = true
	return &u, nil
}

// UpdateUserPassword replaces the password hash for the given user.
func (s *Store) UpdateUserPassword(ctx context.Context, userID int64, passwordHash string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
