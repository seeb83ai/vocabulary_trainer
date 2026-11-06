package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const sessionTimeLayout = "2006-01-02 15:04:05.000000000"

// hashSessionToken returns the value stored for a session token. Only the
// hash is stored, so a copy of the database holds no usable session cookies.
func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateSession stores a new session for userID that is valid until
// expiresAt, and removes expired sessions of all users.
func (s *Store) CreateSession(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	now := time.Now().UTC().Format(sessionTimeLayout)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now); err != nil {
		return fmt.Errorf("prune sessions: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		hashSessionToken(token), userID, now, expiresAt.UTC().Format(sessionTimeLayout)); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSessionUserID returns the user and expiry of the session for token.
// It returns user ID 0 when the session does not exist or expired before now.
func (s *Store) GetSessionUserID(ctx context.Context, token string, now time.Time) (int64, time.Time, error) {
	var userID int64
	var expires string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM sessions WHERE token_hash = ? AND expires_at > ?`,
		hashSessionToken(token), now.UTC().Format(sessionTimeLayout)).Scan(&userID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("get session: %w", err)
	}
	return userID, parseDateTime(expires), nil
}

// DeleteSession revokes the session for token (logout).
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashSessionToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
