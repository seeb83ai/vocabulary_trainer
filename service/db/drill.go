package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"vocabulary_trainer/models"
)

// difficultCandidateAccuracy is the SQL expression for a word's accuracy, used to
// rank the user's hardest words. Mirrors the accuracy formula in tierFilter.
const difficultCandidateAccuracy = `CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts`

// FlagDifficultWords marks the user's hardest seen words for a difficult-words
// drill. It first clears any existing flags, then flags up to count words: about
// half by lowest accuracy and half by lowest SM-2 easiness (the two halves are
// kept distinct). Only seen, graduated words with at least 3 attempts are
// eligible. Returns the number of words actually flagged (may be < count if the
// pool is small).
func (s *Store) FlagDifficultWords(ctx context.Context, userID int64, count int) (int, error) {
	if count <= 0 {
		return 0, nil
	}
	if err := s.ClearAllDrillFlags(ctx, userID); err != nil {
		return 0, err
	}

	accCount := (count + 1) / 2 // ceil(count/2)
	efCount := count - accCount // floor(count/2)

	const candidate = `
		FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh' AND w.user_id = ?
		  AND p.first_seen_at IS NOT NULL
		  AND p.learning_new_word = 0
		  AND p.total_attempts >= 3`

	// Lowest-accuracy half.
	picked := make(map[int64]bool)
	var order []int64
	collect := func(query string, limit int) error {
		rows, err := s.db.QueryContext(ctx, query, userID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			if !picked[id] {
				picked[id] = true
				order = append(order, id)
			}
		}
		return nil
	}

	if accCount > 0 {
		if err := collect(`SELECT p.word_id `+candidate+
			` ORDER BY `+difficultCandidateAccuracy+` ASC, p.word_id ASC LIMIT ?`, accCount); err != nil {
			return 0, fmt.Errorf("flag difficult (accuracy): %w", err)
		}
	}
	// Lowest-easiness half. Fetch up to `count` so distinct picks remain after
	// removing any overlap with the accuracy half, then keep at most efCount new ones.
	if efCount > 0 {
		before := len(order)
		if err := collect(`SELECT p.word_id `+candidate+
			` ORDER BY p.easiness ASC, p.word_id ASC LIMIT ?`, count); err != nil {
			return 0, fmt.Errorf("flag difficult (easiness): %w", err)
		}
		// Trim any easiness picks beyond efCount.
		if max := before + efCount; len(order) > max {
			for _, id := range order[max:] {
				delete(picked, id)
			}
			order = order[:max]
		}
	}

	if len(order) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(order))
	args := make([]any, len(order))
	for i, id := range order {
		placeholders[i] = "?"
		args[i] = id
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET drill_flag = 1 WHERE word_id IN (`+strings.Join(placeholders, ",")+`)`,
		args...); err != nil {
		return 0, fmt.Errorf("flag difficult (update): %w", err)
	}
	return len(order), nil
}

// ClearDrillFlag clears the difficult-words drill flag for a single word
// (called after the word is answered correctly).
func (s *Store) ClearDrillFlag(ctx context.Context, wordID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET drill_flag = 0 WHERE word_id = ?`, wordID)
	return err
}

// ClearAllDrillFlags clears the difficult-words drill flag for all of the user's words.
func (s *Store) ClearAllDrillFlags(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET drill_flag = 0
		 WHERE word_id IN (SELECT id FROM words WHERE user_id = ?)`, userID)
	return err
}

// CountDrillFlags returns how many of the user's words are currently flagged for
// the difficult-words drill.
func (s *Store) CountDrillFlags(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.user_id = ? AND p.drill_flag = 1`, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count drill flags: %w", err)
	}
	return n, nil
}

// GetNextDrillCard returns the next flagged difficult-words card, ordered by
// due_date so a just-failed word (pushed a few minutes out by SM-2) drops behind
// the others instead of repeating immediately. Unlike GetNextCard it ignores the
// due-date horizon: every flagged word is eligible even if due far in the future.
// Returns (nil, nil, nil) when no flagged words remain.
func (s *Store) GetNextDrillCard(ctx context.Context, userID int64) (*models.Word, *models.SM2Progress, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT w.id, w.text, w.language, w.pinyin, w.created_at,
		       p.repetitions, p.easiness, p.interval_days, p.due_date,
		       p.total_correct, p.total_attempts, p.streak_bonus, p.learning_new_word
		FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		WHERE w.language = 'zh' AND w.user_id = ? AND p.drill_flag = 1
		ORDER BY p.due_date ASC
		LIMIT 1`, userID)
	var w models.Word
	var p models.SM2Progress
	var createdAt, dueDate string
	var learning int
	err := row.Scan(
		&w.ID, &w.Text, &w.Language, &w.Pinyin, &createdAt,
		&p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDate,
		&p.TotalCorrect, &p.TotalAttempts, &p.StreakBonus, &learning,
	)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get next drill card: %w", err)
	}
	w.CreatedAt = parseDateTime(createdAt)
	p.DueDate = parseDateTime(dueDate)
	p.LearningNewWord = learning == 1
	p.WordID = w.ID
	return &w, &p, nil
}
