package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"vocabulary_trainer/models"
)

// SeedHanziTranslationForTest inserts a hanzi_decomposition_translation row.
// Intended for use in tests only.
func (s *Store) SeedHanziTranslationForTest(ctx context.Context, character, lang, definition string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, ?, ?)
		 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
		character, strings.ToUpper(lang), definition)
	return err
}

// SeedHanziDecompositionForTest inserts a hanzi_decomposition row with definition
// and also seeds the EN translation table entry. Intended for use in tests only.
func (s *Store) SeedHanziDecompositionForTest(ctx context.Context, character, definition string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, definition) VALUES (?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition`,
		character, definition); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, 'EN', ?)
		 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
		character, definition)
	return err
}

// SeedHanziDecompositionWithDecompForTest inserts a hanzi_decomposition row with
// definition and decomposition string, and also seeds the EN translation table entry.
// Intended for use in tests only.
func (s *Store) SeedHanziDecompositionWithDecompForTest(ctx context.Context, character, definition, decomposition string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, definition, decomposition) VALUES (?, ?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition, decomposition = excluded.decomposition`,
		character, definition, decomposition); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, 'EN', ?)
		 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
		character, definition)
	return err
}

// SeedHanziDecompositionWithPinyinForTest inserts a hanzi_decomposition row with
// definition and a JSON-encoded pinyin array, and seeds the EN translation entry.
// Intended for use in tests only.
func (s *Store) SeedHanziDecompositionWithPinyinForTest(ctx context.Context, character, definition, pinyinJSON string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, definition, pinyin) VALUES (?, ?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition, pinyin = excluded.pinyin`,
		character, definition, pinyinJSON); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, 'EN', ?)
		 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
		character, definition)
	return err
}

// SetComponentSeenForTest marks a component as seen. Intended for use in tests only.
func (s *Store) SetComponentSeenForTest(ctx context.Context, userID int64, character string) {
	s.db.ExecContext(ctx, //nolint:errcheck
		`UPDATE component_progress SET first_seen_date = date('now') WHERE user_id = ? AND character = ?`,
		userID, character)
}

// InsertComponentProgressForTest inserts a component_progress row directly.
// Intended for use in tests only.
func (s *Store) InsertComponentProgressForTest(ctx context.Context, userID int64, character string, dueDate time.Time) {
	s.db.ExecContext(ctx, //nolint:errcheck
		`INSERT OR IGNORE INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
		userID, character, dueDate.UTC().Format("2006-01-02 15:04:05"))
}

// SetComponentAttemptsForTest sets total_attempts for a component_progress row.
// Intended for use in tests only.
func (s *Store) SetComponentAttemptsForTest(ctx context.Context, userID int64, character string, attempts int) {
	s.db.ExecContext(ctx, //nolint:errcheck
		`UPDATE component_progress SET total_attempts = ? WHERE user_id = ? AND character = ?`,
		attempts, userID, character)
}

// SetComponentProgressForTest sets total_correct/total_attempts directly.
// Intended for use in tests only.
func (s *Store) SetComponentProgressForTest(ctx context.Context, userID int64, character string, totalCorrect, totalAttempts int) {
	s.db.ExecContext(ctx, //nolint:errcheck
		`UPDATE component_progress SET total_correct = ?, total_attempts = ? WHERE user_id = ? AND character = ?`,
		totalCorrect, totalAttempts, userID, character)
}

// GetComponentProgressForTest reads a component_progress row directly.
// Intended for use in tests only.
func (s *Store) GetComponentProgressForTest(ctx context.Context, userID int64, character string) (models.ComponentProgress, time.Time, error) {
	var p models.ComponentProgress
	var dueDateStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT repetitions, easiness, interval_days, due_date, total_correct, total_attempts
		 FROM component_progress WHERE user_id = ? AND character = ?`,
		userID, character,
	).Scan(&p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDateStr,
		&p.TotalCorrect, &p.TotalAttempts)
	if err != nil {
		return p, time.Time{}, err
	}
	return p, parseDateTime(dueDateStr), nil
}

// SaveComponentPrevState serialises p to JSON and stores it in the prev_state column
// of component_progress. Called before applying a wrong answer so AcceptCorrect can
// restore the pre-answer state without trusting client data.
func (s *Store) SaveComponentPrevState(ctx context.Context, userID int64, character string, p models.ComponentProgress) error {
	blob, err := json.Marshal(componentPrevState{
		Repetitions:   p.Repetitions,
		Easiness:      p.Easiness,
		IntervalDays:  p.IntervalDays,
		TotalCorrect:  p.TotalCorrect,
		TotalAttempts: p.TotalAttempts,
	})
	if err != nil {
		return fmt.Errorf("marshal component prev state: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE component_progress SET prev_state = ? WHERE user_id = ? AND character = ?`,
		string(blob), userID, character)
	return err
}

// GetComponentPrevState reads the stored pre-answer state for a component.
// Returns nil, nil when no previous state is stored.
func (s *Store) GetComponentPrevState(ctx context.Context, userID int64, character string) (*models.ComponentProgress, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT prev_state FROM component_progress WHERE user_id = ? AND character = ?`,
		userID, character).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get component prev state: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var prev componentPrevState
	if err := json.Unmarshal([]byte(raw.String), &prev); err != nil {
		return nil, fmt.Errorf("unmarshal component prev state: %w", err)
	}
	return &models.ComponentProgress{
		UserID:        userID,
		Character:     character,
		Repetitions:   prev.Repetitions,
		Easiness:      prev.Easiness,
		IntervalDays:  prev.IntervalDays,
		TotalCorrect:  prev.TotalCorrect,
		TotalAttempts: prev.TotalAttempts,
	}, nil
}

// ClearComponentPrevState sets prev_state = NULL for the given component.
// Called after a correct answer or after AcceptCorrect.
func (s *Store) ClearComponentPrevState(ctx context.Context, userID int64, character string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE component_progress SET prev_state = NULL WHERE user_id = ? AND character = ?`,
		userID, character)
	return err
}

// SeedDailyStatBucketsForTest inserts (or overwrites) a daily_stats row's
// bucket snapshot for the given user/date, so tests can simulate a prior
// day's proficiency-bucket state without waiting for real elapsed time.
// dateExpr is a SQLite date()-compatible expression, e.g. "date('now', '-1 day')".
// Intended for use in tests only.
func (s *Store) SeedDailyStatBucketsForTest(ctx context.Context, userID int64, dateExpr string, bNew, bStruggling, bLearning, bPracticing, bMastered int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO daily_stats (user_id, date, bucket_new, bucket_struggling, bucket_learning, bucket_practicing, bucket_mastered)
		VALUES (?, `+dateExpr+`, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET
			bucket_new        = excluded.bucket_new,
			bucket_struggling = excluded.bucket_struggling,
			bucket_learning   = excluded.bucket_learning,
			bucket_practicing = excluded.bucket_practicing,
			bucket_mastered   = excluded.bucket_mastered`,
		userID, bNew, bStruggling, bLearning, bPracticing, bMastered)
	return err
}
