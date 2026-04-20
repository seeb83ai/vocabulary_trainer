package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"unicode"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// InitComponentsForWord adds a component_progress row for every Han rune in
// zhText that exists in hanzi_decomposition (with a non-empty definition).
// Rows are INSERT OR IGNORE so calling this multiple times is safe.
// dueDate is copied from the origin zh word's sm2_progress.due_date.
func (s *Store) InitComponentsForWord(ctx context.Context, userID int64, zhText string, dueDate time.Time) error {
	dueDateStr := dueDate.UTC().Format("2006-01-02 15:04:05")
	for _, r := range []rune(zhText) {
		if !unicode.Is(unicode.Han, r) {
			continue
		}
		var def string
		err := s.db.QueryRowContext(ctx,
			`SELECT COALESCE(definition, '') FROM hanzi_decomposition WHERE character = ?`,
			string(r),
		).Scan(&def)
		if err == sql.ErrNoRows || def == "" {
			continue
		}
		if err != nil {
			return fmt.Errorf("component lookup %q: %w", string(r), err)
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
			userID, string(r), dueDateStr,
		); err != nil {
			return fmt.Errorf("init component %q: %w", string(r), err)
		}
	}
	return nil
}

// componentCard is the internal representation (includes definition for answer checking).
type componentCard struct {
	Character string
	Definition string
	Progress  models.ComponentProgress
}

// GetNextComponentCard returns the most-overdue component due today for the user,
// or nil if nothing is due. The Definition field is server-side only.
func (s *Store) GetNextComponentCard(ctx context.Context, userID int64) (*componentCard, error) {
	var c componentCard
	var dueDateStr string
	var firstSeenDate sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT cp.character, hd.definition, cp.due_date,
		       cp.repetitions, cp.easiness, cp.interval_days,
		       cp.total_correct, cp.total_attempts, cp.first_seen_date
		FROM component_progress cp
		JOIN hanzi_decomposition hd ON hd.character = cp.character
		WHERE cp.user_id = ?
		  AND hd.definition IS NOT NULL AND hd.definition != ''
		  AND cp.due_date < datetime('now', '+1 day')
		ORDER BY cp.due_date ASC
		LIMIT 1`,
		userID,
	).Scan(
		&c.Character, &c.Definition, &dueDateStr,
		&c.Progress.Repetitions, &c.Progress.Easiness, &c.Progress.IntervalDays,
		&c.Progress.TotalCorrect, &c.Progress.TotalAttempts, &firstSeenDate,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get next component card: %w", err)
	}
	c.Progress.UserID = userID
	c.Progress.Character = c.Character
	c.Progress.DueDate = dueDateStr
	if firstSeenDate.Valid {
		c.Progress.FirstSeenDate = &firstSeenDate.String
	}
	return &c, nil
}

// MarkComponentSeen sets first_seen_date = date('now') if it is currently NULL.
func (s *Store) MarkComponentSeen(ctx context.Context, userID int64, character string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE component_progress SET first_seen_date = COALESCE(first_seen_date, date('now'))
		 WHERE user_id = ? AND character = ?`,
		userID, character)
	return err
}

// GetComponentDefinition returns the definition string for a character from
// hanzi_decomposition, or "" if not found.
func (s *Store) GetComponentDefinition(ctx context.Context, character string) (string, error) {
	var def string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(definition, '') FROM hanzi_decomposition WHERE character = ?`, character,
	).Scan(&def)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get component definition: %w", err)
	}
	return def, nil
}

// RecordComponentAnswer updates SM-2 state for a component after an answer.
// Returns the updated progress and the next due time.Time (for JSON responses).
func (s *Store) RecordComponentAnswer(ctx context.Context, userID int64, character string, correct bool) (models.ComponentProgress, time.Time, error) {
	var p models.ComponentProgress
	var dueDateStr string
	var firstSeenDate sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT repetitions, easiness, interval_days, due_date, total_correct, total_attempts, first_seen_date
		 FROM component_progress WHERE user_id = ? AND character = ?`,
		userID, character,
	).Scan(&p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDateStr,
		&p.TotalCorrect, &p.TotalAttempts, &firstSeenDate)
	if err == sql.ErrNoRows {
		return p, time.Time{}, fmt.Errorf("component progress not found for %q", character)
	}
	if err != nil {
		return p, time.Time{}, fmt.Errorf("get component progress: %w", err)
	}
	if firstSeenDate.Valid {
		p.FirstSeenDate = &firstSeenDate.String
	}

	sm2p := models.SM2Progress{
		Repetitions:   p.Repetitions,
		Easiness:      p.Easiness,
		IntervalDays:  p.IntervalDays,
		DueDate:       parseDateTime(dueDateStr),
		TotalCorrect:  p.TotalCorrect,
		TotalAttempts: p.TotalAttempts,
	}
	quality := sm2.QualityWrong
	if correct {
		quality = sm2.QualityCorrect
	}
	updated := sm2.Update(sm2p, quality)
	updated.TotalAttempts++
	if correct {
		updated.TotalCorrect++
	}

	newDue := updated.DueDate.UTC().Format("2006-01-02 15:04:05")
	_, err = s.db.ExecContext(ctx,
		`UPDATE component_progress
		 SET repetitions = ?, easiness = ?, interval_days = ?, due_date = ?,
		     total_correct = ?, total_attempts = ?,
		     first_seen_date = COALESCE(first_seen_date, date('now'))
		 WHERE user_id = ? AND character = ?`,
		updated.Repetitions, updated.Easiness, updated.IntervalDays, newDue,
		updated.TotalCorrect, updated.TotalAttempts,
		userID, character,
	)
	if err != nil {
		return p, time.Time{}, fmt.Errorf("update component progress: %w", err)
	}

	p.UserID = userID
	p.Character = character
	p.Repetitions = updated.Repetitions
	p.Easiness = updated.Easiness
	p.IntervalDays = updated.IntervalDays
	p.DueDate = newDue
	p.TotalCorrect = updated.TotalCorrect
	p.TotalAttempts = updated.TotalAttempts
	return p, updated.DueDate, nil
}

// RecordComponentStat increments today's correct or wrong count in component_stats.
func (s *Store) RecordComponentStat(ctx context.Context, userID int64, correct bool) error {
	col := "wrong"
	if correct {
		col = "correct"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO component_stats (user_id, date, correct, wrong) VALUES (?, date('now'), 0, 0)
		 ON CONFLICT(user_id, date) DO NOTHING`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("upsert component_stats row: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE component_stats SET `+col+` = `+col+` + 1 WHERE user_id = ? AND date = date('now')`,
		userID,
	)
	return err
}

// GetComponentStatsHistory returns daily component training stats for a user.
func (s *Store) GetComponentStatsHistory(ctx context.Context, userID int64) ([]models.ComponentDailyStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT date, correct, wrong FROM component_stats WHERE user_id = ? ORDER BY date ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get component stats history: %w", err)
	}
	var stats []models.ComponentDailyStat
	for rows.Next() {
		var s models.ComponentDailyStat
		if err := rows.Scan(&s.Date, &s.Correct, &s.Wrong); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan component stat: %w", err)
		}
		stats = append(stats, s)
	}
	rows.Close()
	return stats, rows.Err()
}

// SeedHanziDecompositionForTest inserts a hanzi_decomposition row with definition.
// Intended for use in tests only.
func (s *Store) SeedHanziDecompositionForTest(ctx context.Context, character, definition string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, definition) VALUES (?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition`,
		character, definition)
	return err
}

// SetComponentSeenForTest marks a component as seen. Intended for use in tests only.
func (s *Store) SetComponentSeenForTest(ctx context.Context, userID int64, character string) {
	s.db.ExecContext(ctx, //nolint:errcheck
		`UPDATE component_progress SET first_seen_date = date('now') WHERE user_id = ? AND character = ?`,
		userID, character)
}

// GetComponentCounts returns the number of components due today and the total
// number of components in training for the given user.
func (s *Store) GetComponentCounts(ctx context.Context, userID int64) (dueToday, total int, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM component_progress cp
		 JOIN hanzi_decomposition hd ON hd.character = cp.character
		 WHERE cp.user_id = ?
		   AND hd.definition IS NOT NULL AND hd.definition != ''
		   AND cp.first_seen_date IS NOT NULL
		   AND cp.due_date < date('now', '+1 day')`,
		userID,
	).Scan(&dueToday)
	if err != nil {
		return 0, 0, fmt.Errorf("get component due count: %w", err)
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM component_progress WHERE user_id = ?`,
		userID,
	).Scan(&total)
	if err != nil {
		return 0, 0, fmt.Errorf("get component total count: %w", err)
	}
	return dueToday, total, nil
}
