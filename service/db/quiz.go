package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// GetSM2Progress returns the SM-2 progress for a word.
func (s *Store) GetSM2Progress(ctx context.Context, wordID int64) (*models.SM2Progress, error) {
	var p models.SM2Progress
	var dueDate string
	var learning int
	err := s.db.QueryRowContext(ctx,
		`SELECT word_id, repetitions, easiness, interval_days, due_date, total_correct, total_attempts, streak_bonus, learning_new_word, known_correct_count
		 FROM sm2_progress WHERE word_id = ?`, wordID).
		Scan(&p.WordID, &p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDate,
			&p.TotalCorrect, &p.TotalAttempts, &p.StreakBonus, &learning, &p.KnownCorrectCount)
	p.DueDate = parseDateTime(dueDate)
	p.LearningNewWord = learning == 1
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get sm2 progress: %w", err)
	}
	return &p, nil
}

// UpdateSM2Progress saves updated SM-2 state back to the DB.
func (s *Store) UpdateSM2Progress(ctx context.Context, p models.SM2Progress) error {
	learningInt := 0
	if p.LearningNewWord {
		learningInt = 1
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress
		 SET repetitions = ?, easiness = ?, interval_days = ?, due_date = ?,
		     total_correct = ?, total_attempts = ?, streak_bonus = ?, learning_new_word = ?, known_correct_count = ?
		 WHERE word_id = ?`,
		p.Repetitions, p.Easiness, p.IntervalDays,
		p.DueDate.UTC().Format("2006-01-02 15:04:05"),
		p.TotalCorrect, p.TotalAttempts, p.StreakBonus, learningInt, p.KnownCorrectCount, p.WordID)
	if err != nil {
		return fmt.Errorf("update sm2: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RecordAnswerTimestamps stamps sm2_progress.last_attempt_at (always) and
// last_wrong_at (only when correct=false) for the given word. These drive the
// "hardest words" and "last mistakes" match-game modes' repeat-avoidance
// rules (issue #288) and are independent bookkeeping, not part of the SM-2
// algorithm itself.
func (s *Store) RecordAnswerTimestamps(ctx context.Context, wordID int64, correct bool) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if correct {
		_, err := s.db.ExecContext(ctx,
			`UPDATE sm2_progress SET last_attempt_at = ? WHERE word_id = ?`, now, wordID)
		if err != nil {
			return fmt.Errorf("record answer timestamps: %w", err)
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET last_attempt_at = ?, last_wrong_at = ? WHERE word_id = ?`, now, now, wordID)
	if err != nil {
		return fmt.Errorf("record answer timestamps: %w", err)
	}
	return nil
}

// IsLearningNewWord returns true if the given word is currently in the new-word
// introduction phase (learning_new_word=1) for the given user.
func (s *Store) IsLearningNewWord(ctx context.Context, userID, wordID int64) (bool, error) {
	var v int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(p.learning_new_word, 0) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE p.word_id = ? AND w.user_id = ?`,
		wordID, userID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("is learning new word: %w", err)
	}
	return v == 1, nil
}

// SkipWord moves a word's due date forward by the given number of days without
// touching first_seen_at or attempt counters.
func (s *Store) SkipWord(ctx context.Context, userID, wordID int64, days int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = datetime('now', ?)
		 WHERE word_id = ? AND word_id IN (SELECT id FROM words WHERE user_id = ?)`,
		fmt.Sprintf("+%d days", days), wordID, userID)
	if err != nil {
		return fmt.Errorf("skip word: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AcknowledgeWord marks a new word as "introduced" by setting total_attempts=1,
// first_seen_at=now, and due_date=now so it becomes immediately available for quizzing.
func (s *Store) AcknowledgeWord(ctx context.Context, userID, wordID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress
		 SET total_attempts = CASE WHEN total_attempts = 0 THEN 1 ELSE total_attempts END,
		     first_seen_at   = COALESCE(first_seen_at, CURRENT_TIMESTAMP),
		     due_date = CURRENT_TIMESTAMP
		 WHERE word_id = ? AND word_id IN (SELECT id FROM words WHERE user_id = ?)`,
		wordID, userID)
	if err != nil {
		return fmt.Errorf("acknowledge word: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ResetWordProgress restores a zh word's SM-2 progress to the unseen state
// (matching a freshly created word) so it is removed from every bucket and
// reintroduced as new.
func (s *Store) ResetWordProgress(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress
		 SET repetitions = 0, easiness = 2.5, interval_days = 1, due_date = CURRENT_TIMESTAMP,
		     total_correct = 0, total_attempts = 0, streak_bonus = 0, learning_new_word = 1,
		     first_seen_at = NULL
		 WHERE word_id = ? AND word_id IN (
		     SELECT id FROM words WHERE id = ? AND language = 'zh' AND user_id = ?)`,
		id, id, userID)
	if err != nil {
		return fmt.Errorf("reset word progress: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AcknowledgeRandomWords marks up to n random unseen zh words as due now so they
// appear immediately in the quiz without going through the new-word introduction flow.
// Also initialises component_progress rows for each acknowledged word.
// Returns the number of words actually acknowledged.
func (s *Store) AcknowledgeRandomWords(ctx context.Context, userID int64, n int) (int, error) {
	type wordInfo struct {
		id     int64
		zhText string
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, w.text FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		WHERE w.language = 'zh' AND w.user_id = ?
		  AND p.first_seen_at IS NULL
		ORDER BY RANDOM()
		LIMIT ?`, userID, n)
	if err != nil {
		return 0, fmt.Errorf("select random words to acknowledge: %w", err)
	}
	var words []wordInfo
	for rows.Next() {
		var w wordInfo
		if err := rows.Scan(&w.id, &w.zhText); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan random word: %w", err)
		}
		words = append(words, w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("scan random words: %w", err)
	}
	if len(words) == 0 {
		return 0, nil
	}

	now := time.Now()
	nowStr := now.UTC().Format("2006-01-02 15:04:05")

	// Wrap the acknowledgement writes in a single transaction so a mid-loop
	// failure leaves no partial state (some words acknowledged, others not).
	// Component initialisation runs after commit: it uses the same single
	// SQLite connection, so it cannot run while the transaction holds it, and it
	// is non-fatal (already best-effort/logged).
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin acknowledge tx: %w", err)
	}
	defer tx.Rollback()
	for _, w := range words {
		if _, err := tx.ExecContext(ctx,
			`UPDATE sm2_progress
			 SET total_attempts = 1, first_seen_at = CURRENT_TIMESTAMP, due_date = ?
			 WHERE word_id = ?`, nowStr, w.id); err != nil {
			return 0, fmt.Errorf("acknowledge word %d: %w", w.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit acknowledge tx: %w", err)
	}

	for _, w := range words {
		if err := s.InitComponentsForWord(ctx, userID, w.zhText, now); err != nil {
			log.Printf("AcknowledgeRandomWords: initComponents %q: %v", w.zhText, err)
		}
		if err := s.CreateSubwordsForWord(ctx, userID, w.id, w.zhText); err != nil {
			log.Printf("AcknowledgeRandomWords: CreateSubwordsForWord %q: %v", w.zhText, err)
		}
	}
	return len(words), nil
}

// GetStats returns due-today count, total word count (zh words only), and the number of
// new words introduced today (globally, not filtered by tag).
func (s *Store) GetStats(ctx context.Context, userID int64, tags []string, bucket string) (dueToday, total, newToday int, err error) {
	tagFilter := ""
	var tagArgs []any
	if len(tags) > 0 {
		placeholders := make([]string, len(tags))
		for i, t := range tags {
			placeholders[i] = "?"
			tagArgs = append(tagArgs, t)
		}
		tagFilter = ` AND EXISTS (
			SELECT 1 FROM word_tags wt
			JOIN tags tg ON tg.id = wt.tag_id
			WHERE wt.word_id = w.id AND tg.name IN (` + strings.Join(placeholders, ",") + `))`
	}
	bucketSQL := tierFilter(bucket)

	// When a bucket filter is active the total count must join sm2_progress.
	totalArgs := append([]any{userID}, tagArgs...)
	if bucket != "" {
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM words w JOIN sm2_progress p ON p.word_id = w.id`+
				` WHERE w.language = 'zh' AND w.user_id = ?`+tagFilter+bucketSQL, totalArgs...).Scan(&total)
	} else {
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM words w WHERE w.language = 'zh' AND w.user_id = ?`+tagFilter, totalArgs...).Scan(&total)
	}
	if err != nil {
		return
	}
	dueArgs := append([]any{userID}, tagArgs...)
	// Count all words due by end of today (midnight) so the user sees the
	// full day's workload and the "done" screen only appears once every
	// card due today has been reviewed.
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND w.user_id = ? AND p.first_seen_at IS NOT NULL
		   AND p.due_date < date('now', '+1 day')`+tagFilter+bucketSQL, dueArgs...).Scan(&dueToday)
	if err != nil {
		return
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND w.user_id = ? AND date(p.first_seen_at) = date('now')`, userID).Scan(&newToday)
	return
}

// CountUnseenZhWords returns the number of zh words that have never been presented
// (first_seen_at IS NULL), optionally filtered by tags.
func (s *Store) CountUnseenZhWords(ctx context.Context, userID int64, tags []string) (int, error) {
	tagFilter := ""
	args := []any{userID}
	if len(tags) > 0 {
		placeholders := make([]string, len(tags))
		for i, t := range tags {
			placeholders[i] = "?"
			args = append(args, t)
		}
		tagFilter = ` AND EXISTS (
			SELECT 1 FROM word_tags wt
			JOIN tags tg ON tg.id = wt.tag_id
			WHERE wt.word_id = w.id AND tg.name IN (` + strings.Join(placeholders, ",") + `))`
	}
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND w.user_id = ? AND p.first_seen_at IS NULL`+tagFilter,
		args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unseen zh words: %w", err)
	}
	return count, nil
}

// CountLearningNewWords returns the number of zh words still in the "new" learning
// phase (learning_new_word = 1), optionally filtered by tags.
func (s *Store) CountLearningNewWords(ctx context.Context, userID int64, tags []string) (int, error) {
	tagFilter := ""
	args := []any{userID}
	if len(tags) > 0 {
		placeholders := make([]string, len(tags))
		for i, t := range tags {
			placeholders[i] = "?"
			args = append(args, t)
		}
		tagFilter = ` AND EXISTS (
			SELECT 1 FROM word_tags wt
			JOIN tags tg ON tg.id = wt.tag_id
			WHERE wt.word_id = w.id AND tg.name IN (` + strings.Join(placeholders, ",") + `))`
	}
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND w.user_id = ? AND p.learning_new_word = 1 AND p.first_seen_at IS NOT NULL`+tagFilter,
		args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count learning new words: %w", err)
	}
	return count, nil
}

// GetWordCountByDueDate returns the number of zh words grouped by due date,
// covering overdue words (grouped as today), today, and the next 30 days.
// Unseen words (first_seen_at IS NULL) are excluded.
func (s *Store) GetWordCountByDueDate(ctx context.Context, userID int64, tags []string) ([]models.DueDateCount, error) {
	tagFilter := ""
	args := []any{userID}
	if len(tags) > 0 {
		placeholders := make([]string, len(tags))
		for i, t := range tags {
			placeholders[i] = "?"
			args = append(args, t)
		}
		tagFilter = ` AND EXISTS (
			SELECT 1 FROM word_tags wt
			JOIN tags tg ON tg.id = wt.tag_id
			WHERE wt.word_id = w.id AND tg.name IN (` + strings.Join(placeholders, ",") + `))`
	}
	query := `SELECT
		CASE
			WHEN date(p.due_date) <= date('now') THEN date('now')
			ELSE date(p.due_date)
		END AS bucket_date,
		COUNT(*) AS cnt
	FROM sm2_progress p
	JOIN words w ON w.id = p.word_id
	WHERE w.language = 'zh'
	  AND w.user_id = ?
	  AND p.first_seen_at IS NOT NULL
	  AND date(p.due_date) <= date('now', '+30 days')` + tagFilter + `
	GROUP BY bucket_date
	ORDER BY bucket_date`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get word count by due date: %w", err)
	}
	defer rows.Close()
	var result []models.DueDateCount
	for rows.Next() {
		var d models.DueDateCount
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			return nil, fmt.Errorf("scan due date count: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// sm2PrevState is the internal JSON encoding for SaveSM2PrevState.
type sm2PrevState struct {
	Easiness          float64 `json:"ef"`
	Repetitions       int     `json:"reps"`
	IntervalDays      int     `json:"iv"`
	TotalCorrect      int     `json:"tc"`
	TotalAttempts     int     `json:"ta"`
	StreakBonus       int     `json:"sb"`
	LearningNewWord   bool    `json:"lnw"`
	KnownCorrectCount int     `json:"kcc"`
}

// SaveSM2PrevState serialises p to JSON and stores it in the prev_state column
// of sm2_progress for the given word. Called before applying a wrong answer so
// AcceptCorrect can restore the pre-answer state without trusting client data.
func (s *Store) SaveSM2PrevState(ctx context.Context, wordID int64, p models.SM2Progress) error {
	blob, err := json.Marshal(sm2PrevState{
		Easiness:          p.Easiness,
		Repetitions:       p.Repetitions,
		IntervalDays:      p.IntervalDays,
		TotalCorrect:      p.TotalCorrect,
		TotalAttempts:     p.TotalAttempts,
		StreakBonus:       p.StreakBonus,
		LearningNewWord:   p.LearningNewWord,
		KnownCorrectCount: p.KnownCorrectCount,
	})
	if err != nil {
		return fmt.Errorf("marshal prev state: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET prev_state = ? WHERE word_id = ?`, string(blob), wordID)
	return err
}

// GetSM2PrevState reads the stored pre-answer SM-2 state for a word.
// Returns nil, nil when no previous state is stored (column is NULL).
func (s *Store) GetSM2PrevState(ctx context.Context, wordID int64) (*models.SM2Progress, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT prev_state FROM sm2_progress WHERE word_id = ?`, wordID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get prev state: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var prev sm2PrevState
	if err := json.Unmarshal([]byte(raw.String), &prev); err != nil {
		return nil, fmt.Errorf("unmarshal prev state: %w", err)
	}
	return &models.SM2Progress{
		WordID:            wordID,
		Easiness:          prev.Easiness,
		Repetitions:       prev.Repetitions,
		IntervalDays:      prev.IntervalDays,
		TotalCorrect:      prev.TotalCorrect,
		TotalAttempts:     prev.TotalAttempts,
		StreakBonus:       prev.StreakBonus,
		LearningNewWord:   prev.LearningNewWord,
		KnownCorrectCount: prev.KnownCorrectCount,
	}, nil
}

// ClearSM2PrevState sets prev_state = NULL for the given word.
// Called after a correct answer or after AcceptCorrect.
func (s *Store) ClearSM2PrevState(ctx context.Context, wordID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET prev_state = NULL WHERE word_id = ?`, wordID)
	return err
}

// SharesTranslation returns true when zhWordID1 and zhWordID2 share at least
// one translation in the given languages. If langs is empty it falls back to
// ["en"]. Matching is done in Go using sm2.ExpandVariants (the same
// slash-alternative / optional-parens expansion CheckAnswer applies) rather
// than raw string equality, so a multi-gloss entry like "Nudeln / Pasta"
// correctly overlaps with a plain "Nudeln" translation on another word.
func (s *Store) SharesTranslation(ctx context.Context, wordID1, wordID2 int64, langs []string) (bool, error) {
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	placeholders := make([]string, len(langs))
	args := make([]any, len(langs))
	for i, l := range langs {
		placeholders[i] = "?"
		args[i] = l
	}
	langList := strings.Join(placeholders, ",")

	fetchVariants := func(wordID int64) (map[string]struct{}, error) {
		rows, err := s.db.QueryContext(ctx, `
			SELECT w.text FROM words w
			JOIN translations t ON t.translation_word_id = w.id
			WHERE t.zh_word_id = ? AND w.language IN (`+langList+`)`,
			append([]any{wordID}, args...)...)
		if err != nil {
			return nil, fmt.Errorf("shares translation: %w", err)
		}
		defer rows.Close()
		variants := map[string]struct{}{}
		for rows.Next() {
			var text string
			if err := rows.Scan(&text); err != nil {
				return nil, fmt.Errorf("shares translation: %w", err)
			}
			for _, v := range sm2.ExpandVariants(text) {
				variants[v] = struct{}{}
			}
		}
		return variants, rows.Err()
	}

	variants1, err := fetchVariants(wordID1)
	if err != nil {
		return false, err
	}
	variants2, err := fetchVariants(wordID2)
	if err != nil {
		return false, err
	}
	for v := range variants1 {
		if _, ok := variants2[v]; ok {
			return true, nil
		}
	}
	return false, nil
}
