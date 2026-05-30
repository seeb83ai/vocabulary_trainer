package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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
		`SELECT id, email, password_hash, COALESCE(email_verified,0), COALESCE(role,'free') FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &verified, &u.Role)
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
		`SELECT id, email, password_hash, COALESCE(email_verified,0), COALESCE(role,'free') FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &verified, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	u.EmailVerified = verified == 1
	return &u, nil
}

// GetUserRole returns the role of the given user ("admin", "plus", or "free").
// Returns "free" if the user is not found.
func (s *Store) GetUserRole(ctx context.Context, userID int64) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(role,'free') FROM users WHERE id = ?`, userID).
		Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "free", nil
	}
	if err != nil {
		return "free", fmt.Errorf("get user role: %w", err)
	}
	return role, nil
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
		`SELECT id, email, password_hash, COALESCE(email_verified,0), COALESCE(role,'free')
		 FROM users
		 WHERE verification_token = ?
		   AND verification_expires_at > datetime('now')`, token).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &verified, &u.Role)
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

// ensureUserSettings creates a user_settings row with defaults if one does not
// already exist for the given user.
func (s *Store) ensureUserSettings(ctx context.Context, userID int64) error {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return err
	}
	salt := hex.EncodeToString(saltBytes)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO user_settings (user_id, api_key_salt) VALUES (?, ?)`,
		userID, salt)
	return err
}

// GetUserSettings returns the settings for a user (creating defaults on first access).
func (s *Store) GetUserSettings(ctx context.Context, userID int64) (*models.UserSettings, error) {
	settings, _, _, _, err := s.GetUserSettingsRaw(ctx, userID)
	return settings, err
}

// GetUserSettingsRaw returns settings plus the raw encrypted blobs needed by handlers.
func (s *Store) GetUserSettingsRaw(ctx context.Context, userID int64) (
	settings *models.UserSettings,
	salt, deeplEnc, llmEnc string,
	err error,
) {
	if err = s.ensureUserSettings(ctx, userID); err != nil {
		return nil, "", "", "", err
	}
	var st models.UserSettings
	err = s.db.QueryRowContext(ctx, `
		SELECT primary_lang, secondary_lang,
		       prog_new, prog_tier_struggling, prog_tier_learning,
		       prog_tier_practicing, prog_tier_mastered,
		       new_word_mode_0, new_word_mode_1, new_word_mode_2,
		       api_key_salt, deepl_key_enc, llm_provider, llm_key_enc, llm_local_url,
		       COALESCE(accept_correct_mode, 'typo')
		FROM user_settings WHERE user_id = ?`, userID).Scan(
		&st.PrimaryLang, &st.SecondaryLang,
		&st.ProgNew, &st.ProgTierStruggling, &st.ProgTierLearning,
		&st.ProgTierPracticing, &st.ProgTierMastered,
		&st.NewWordMode0, &st.NewWordMode1, &st.NewWordMode2,
		&salt, &deeplEnc, &st.LLMProvider, &llmEnc, &st.LLMLocalURL,
		&st.AcceptCorrectMode,
	)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("get user settings: %w", err)
	}
	return &st, salt, deeplEnc, llmEnc, nil
}

// UpdateUserSettings saves language and quiz-mode preferences.
func (s *Store) UpdateUserSettings(ctx context.Context, userID int64, st models.UserSettings) error {
	if err := s.ensureUserSettings(ctx, userID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_settings SET
			primary_lang         = ?,
			secondary_lang       = ?,
			prog_new             = ?,
			prog_tier_struggling = ?,
			prog_tier_learning   = ?,
			prog_tier_practicing = ?,
			prog_tier_mastered   = ?,
			new_word_mode_0      = ?,
			new_word_mode_1      = ?,
			new_word_mode_2      = ?,
			accept_correct_mode  = ?
		WHERE user_id = ?`,
		st.PrimaryLang, st.SecondaryLang,
		st.ProgNew, st.ProgTierStruggling, st.ProgTierLearning,
		st.ProgTierPracticing, st.ProgTierMastered,
		st.NewWordMode0, st.NewWordMode1, st.NewWordMode2,
		st.AcceptCorrectMode,
		userID,
	)
	return err
}

// UpdateUserAPIKeys stores encrypted API keys and the LLM provider / local URL.
func (s *Store) UpdateUserAPIKeys(ctx context.Context, userID int64, deeplEnc, llmProvider, llmEnc, llmLocalURL string) error {
	if err := s.ensureUserSettings(ctx, userID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_settings SET
			deepl_key_enc = ?,
			llm_provider  = ?,
			llm_key_enc   = ?,
			llm_local_url = ?
		WHERE user_id = ?`,
		deeplEnc, llmProvider, llmEnc, llmLocalURL, userID,
	)
	return err
}

// CreateUserWithSettings inserts a new user and immediately creates a default
// user_settings row so the settings key can be derived on first login.
func (s *Store) CreateUserWithSettings(ctx context.Context, email, passwordHash, verificationToken string, expiresAt time.Time) (int64, error) {
	userID, err := s.CreateUser(ctx, email, passwordHash, verificationToken, expiresAt)
	if err != nil {
		return 0, err
	}
	if err := s.ensureUserSettings(ctx, userID); err != nil {
		return userID, fmt.Errorf("init user settings: %w", err)
	}
	return userID, nil
}
