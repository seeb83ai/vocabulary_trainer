package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"vocabulary_trainer/models"
)

// hmmNameFilter returns the SQL WHERE fragment that filters out entries with
// empty names for the joined library tables.
const hmmNameFilter = `(
       (p.entity_type = 'actor'     AND a.actor_name    != '')
    OR (p.entity_type = 'location'  AND l.location_name != '')
    OR (p.entity_type = 'tone_room' AND tr.room_name    != '')
    OR (p.entity_type = 'prop'      AND pr.prop_name    != '')
  )`

// hmmBaseJoins is the shared LEFT JOIN fragment for all HMM quiz queries.
const hmmBaseJoins = `
LEFT JOIN hmm_actors     a  ON p.entity_type = 'actor'     AND a.initial              = p.entity_key
LEFT JOIN hmm_locations  l  ON p.entity_type = 'location'  AND l.final_key             = p.entity_key
LEFT JOIN hmm_tone_rooms tr ON p.entity_type = 'tone_room' AND CAST(tr.tone AS TEXT)   = p.entity_key
LEFT JOIN hmm_props      pr ON p.entity_type = 'prop'      AND pr.radical              = p.entity_key`

// hmmPrompt builds the human-readable prompt for a card.
func hmmPrompt(entityType, entityKey string) string {
	switch entityType {
	case models.HMMEntityActor:
		if entityKey == "null" {
			return "(no initial)"
		}
		return entityKey
	case models.HMMEntityLocation:
		if entityKey == "null" {
			return "(no final)"
		}
		return entityKey
	case models.HMMEntityToneRoom:
		return "Tone " + entityKey
	case models.HMMEntityProp:
		return entityKey
	}
	return entityKey
}

// EnsureHMMProgress inserts missing progress rows for all library entries that
// have a non-empty name. Safe to call repeatedly (INSERT OR IGNORE).
func (s *Store) EnsureHMMProgress(ctx context.Context) error {
	type seedQuery struct {
		typ   string
		query string
	}
	seeds := []seedQuery{
		{"actor", `SELECT initial FROM hmm_actors WHERE actor_name != ''`},
		{"location", `SELECT final_key FROM hmm_locations WHERE location_name != ''`},
		{"tone_room", `SELECT CAST(tone AS TEXT) FROM hmm_tone_rooms WHERE room_name != ''`},
		{"prop", `SELECT radical FROM hmm_props WHERE prop_name != ''`},
	}
	for _, s2 := range seeds {
		rows, err := s.db.QueryContext(ctx, s2.query)
		if err != nil {
			return fmt.Errorf("ensure hmm_progress %s: %w", s2.typ, err)
		}
		var keys []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return fmt.Errorf("scan hmm %s key: %w", s2.typ, err)
			}
			keys = append(keys, k)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate hmm %s keys: %w", s2.typ, err)
		}
		for _, key := range keys {
			if _, err := s.db.ExecContext(ctx,
				`INSERT OR IGNORE INTO hmm_progress (entity_type, entity_key, first_seen_date) VALUES (?, ?, date('now'))`,
				s2.typ, key); err != nil {
				return fmt.Errorf("insert hmm_progress %s/%s: %w", s2.typ, key, err)
			}
		}
	}
	return nil
}

// GetNextDueHMMCard returns the next mnemonic library entry to review.
// Used by the word training page to interleave HMM reviews.
func (s *Store) GetNextDueHMMCard(ctx context.Context, types []string) (*models.HMMQuizCard, *models.HMMProgress, error) {
	typeFilter := ""
	var typeArgs []any
	if len(types) > 0 {
		placeholders := make([]string, len(types))
		for i, t := range types {
			placeholders[i] = "?"
			typeArgs = append(typeArgs, t)
		}
		typeFilter = " AND p.entity_type IN (" + strings.Join(placeholders, ",") + ")"
	}

	query := `
SELECT p.entity_type, p.entity_key,
       COALESCE(a.actor_name, l.location_name, tr.room_name, pr.prop_name, '') AS current_name,
       COALESCE(a.category, ''), COALESCE(a.hint, ''),
       p.repetitions, p.easiness, p.interval_days, p.due_date,
       p.total_correct, p.total_attempts, p.learning, p.streak_bonus,
       COALESCE(p.first_seen_date, '')
FROM hmm_progress p` + hmmBaseJoins + `
WHERE ` + hmmNameFilter + typeFilter + ` %s
ORDER BY p.due_date ASC
LIMIT 1`

	tryQuery := func(extra string) (*models.HMMQuizCard, *models.HMMProgress, error) {
		args := append([]any{}, typeArgs...)
		row := s.db.QueryRowContext(ctx, fmt.Sprintf(query, extra), args...)
		var card models.HMMQuizCard
		var prog models.HMMProgress
		var dueDate string
		var learningInt int
		err := row.Scan(
			&card.EntityType, &card.EntityKey,
			new(string),
			&card.Category, &card.Hint,
			&prog.Repetitions, &prog.Easiness, &prog.IntervalDays, &dueDate,
			&prog.TotalCorrect, &prog.TotalAttempts, &learningInt, &prog.StreakBonus,
			&prog.FirstSeenDate,
		)
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get next due hmm card: %w", err)
		}
		card.Prompt = hmmPrompt(card.EntityType, card.EntityKey)
		card.DueDate = parseDateTime(dueDate)
		card.IntervalDays = prog.IntervalDays
		card.Learning = learningInt == 1
		prog.EntityType = card.EntityType
		prog.EntityKey = card.EntityKey
		prog.DueDate = card.DueDate
		prog.Learning = card.Learning
		return &card, &prog, nil
	}

	todayBound := "AND p.due_date < date('now', '+1 day')"

	card, prog, err := tryQuery("AND p.due_date <= CURRENT_TIMESTAMP")
	if err != nil || card != nil {
		return card, prog, err
	}
	card, prog, err = tryQuery(fmt.Sprintf("AND p.due_date > datetime('now', '+%d seconds') %s", 180, todayBound))
	if err != nil || card != nil {
		return card, prog, err
	}
	return tryQuery(todayBound)
}

// GetHMMProgress loads the progress record for a specific entity.
func (s *Store) GetHMMProgress(ctx context.Context, entityType, entityKey string) (*models.HMMProgress, error) {
	var p models.HMMProgress
	var dueDate string
	var learningInt int
	err := s.db.QueryRowContext(ctx,
		`SELECT entity_type, entity_key, repetitions, easiness, interval_days, due_date,
		        total_correct, total_attempts, learning, streak_bonus,
		        COALESCE(first_seen_date, '')
		 FROM hmm_progress WHERE entity_type = ? AND entity_key = ?`,
		entityType, entityKey).
		Scan(&p.EntityType, &p.EntityKey, &p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDate,
			&p.TotalCorrect, &p.TotalAttempts, &learningInt, &p.StreakBonus, &p.FirstSeenDate)
	p.DueDate = parseDateTime(dueDate)
	p.Learning = learningInt == 1
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hmm progress: %w", err)
	}
	return &p, nil
}

// UpdateHMMProgress saves updated progress for an HMM entity.
func (s *Store) UpdateHMMProgress(ctx context.Context, p models.HMMProgress) error {
	learningInt := 0
	if p.Learning {
		learningInt = 1
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE hmm_progress
		 SET repetitions = ?, easiness = ?, interval_days = ?, due_date = ?,
		     total_correct = ?, total_attempts = ?, learning = ?, streak_bonus = ?
		 WHERE entity_type = ? AND entity_key = ?`,
		p.Repetitions, p.Easiness, p.IntervalDays,
		p.DueDate.UTC().Format("2006-01-02 15:04:05"),
		p.TotalCorrect, p.TotalAttempts, learningInt, p.StreakBonus,
		p.EntityType, p.EntityKey)
	if err != nil {
		return fmt.Errorf("update hmm progress: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetHMMStats returns the number of entries due today and the total number of
// named entries with progress rows. Optionally filtered by entity types.
func (s *Store) GetHMMStats(ctx context.Context, types []string) (models.HMMQuizStats, error) {
	typeFilter := ""
	var typeArgs []any
	if len(types) > 0 {
		placeholders := make([]string, len(types))
		for i, t := range types {
			placeholders[i] = "?"
			typeArgs = append(typeArgs, t)
		}
		typeFilter = " AND p.entity_type IN (" + strings.Join(placeholders, ",") + ")"
	}

	baseQuery := `FROM hmm_progress p` + hmmBaseJoins + `
WHERE ` + hmmNameFilter + typeFilter

	var stats models.HMMQuizStats

	totalArgs := append([]any{}, typeArgs...)
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) `+baseQuery, totalArgs...).Scan(&stats.Total); err != nil {
		return stats, fmt.Errorf("count hmm total: %w", err)
	}

	dueArgs := append([]any{}, typeArgs...)
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) `+baseQuery+` AND p.due_date < date('now', '+1 day')`,
		dueArgs...).Scan(&stats.DueToday); err != nil {
		return stats, fmt.Errorf("count hmm due: %w", err)
	}

	return stats, nil
}
