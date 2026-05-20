package db

import (
	"context"
	"database/sql"
	"encoding/json"
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
		`SELECT word_id, repetitions, easiness, interval_days, due_date, total_correct, total_attempts, streak_bonus, learning_new_word
		 FROM sm2_progress WHERE word_id = ?`, wordID).
		Scan(&p.WordID, &p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDate,
			&p.TotalCorrect, &p.TotalAttempts, &p.StreakBonus, &learning)
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
		     total_correct = ?, total_attempts = ?, streak_bonus = ?, learning_new_word = ?
		 WHERE word_id = ?`,
		p.Repetitions, p.Easiness, p.IntervalDays,
		p.DueDate.UTC().Format("2006-01-02 15:04:05"),
		p.TotalCorrect, p.TotalAttempts, p.StreakBonus, learningInt, p.WordID)
	if err != nil {
		return fmt.Errorf("update sm2: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SkipWord moves a word's due date forward by the given number of days without
// touching first_seen_date or attempt counters.
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
// first_seen_date=today, and due_date=now so it becomes immediately available for quizzing.
func (s *Store) AcknowledgeWord(ctx context.Context, userID, wordID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress
		 SET total_attempts = CASE WHEN total_attempts = 0 THEN 1 ELSE total_attempts END,
		     first_seen_date = COALESCE(first_seen_date, date('now')),
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
		  AND p.first_seen_date IS NULL
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
	for _, w := range words {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE sm2_progress
			 SET total_attempts = 1, first_seen_date = date('now'), due_date = ?
			 WHERE word_id = ?`, nowStr, w.id); err != nil {
			return 0, fmt.Errorf("acknowledge word %d: %w", w.id, err)
		}
		if err := s.InitComponentsForWord(ctx, userID, w.zhText, now); err != nil {
			log.Printf("AcknowledgeRandomWords: initComponents %q: %v", w.zhText, err)
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
		 WHERE w.language = 'zh' AND w.user_id = ? AND p.first_seen_date IS NOT NULL
		   AND p.due_date < date('now', '+1 day')`+tagFilter+bucketSQL, dueArgs...).Scan(&dueToday)
	if err != nil {
		return
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND w.user_id = ? AND p.first_seen_date = date('now')`, userID).Scan(&newToday)
	return
}

// CountUnseenZhWords returns the number of zh words that have never been presented
// (first_seen_date IS NULL), optionally filtered by tags.
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
		 WHERE w.language = 'zh' AND w.user_id = ? AND p.first_seen_date IS NULL`+tagFilter,
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
		 WHERE w.language = 'zh' AND w.user_id = ? AND p.learning_new_word = 1 AND p.first_seen_date IS NOT NULL`+tagFilter,
		args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count learning new words: %w", err)
	}
	return count, nil
}

// GetWordCountByDueDate returns the number of zh words grouped by due date,
// covering overdue words (grouped as today), today, and the next 30 days.
// Unseen words (first_seen_date IS NULL) are excluded.
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
	  AND p.first_seen_date IS NOT NULL
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

// LookupConfusion checks if the user's wrong answer matches a different known word.
// For zh_to_en / zh_pinyin_to_en: looks for a translation word (restricted to langs)
// whose text matches the answer, then returns the zh word it belongs to (if different
// from zhWordID).
// For en_to_zh: looks for a ZH word whose text matches the answer (if different from zhWordID).
// Returns (confusedWithID, true, nil) if a confusion is found, (0, false, nil) if not.
func (s *Store) LookupConfusion(ctx context.Context, userID, zhWordID int64, answer, mode string, langs []string) (int64, bool, error) {
	normalized := sm2.NormalizeAnswer(answer)
	if normalized == "" {
		return 0, false, nil
	}

	var confusedWithID int64
	var err error

	switch mode {
	case "zh_to_transl", "zh_pinyin_to_transl":
		// Find the zh word linked to a translation word whose text matches the answer,
		// restricted to the languages the user has selected and owned by the same user.
		if len(langs) == 0 {
			langs = []string{"en"}
		}
		placeholders := make([]string, len(langs))
		args := make([]any, 0, len(langs)+3)
		args = append(args, normalized)
		for i, l := range langs {
			placeholders[i] = "?"
			args = append(args, l)
		}
		args = append(args, zhWordID, userID)
		err = s.db.QueryRowContext(ctx, `
			SELECT t.zh_word_id FROM words w
			JOIN translations t ON t.translation_word_id = w.id
			JOIN words wz ON wz.id = t.zh_word_id
			WHERE LOWER(TRIM(w.text)) = ?
			  AND w.language IN (`+strings.Join(placeholders, ",")+`)
			  AND t.zh_word_id != ?
			  AND wz.user_id = ?
			LIMIT 1`, args...).Scan(&confusedWithID)
	case "transl_to_zh":
		// Find a ZH word whose text matches the answer, owned by the same user.
		err = s.db.QueryRowContext(ctx, `
			SELECT id FROM words
			WHERE language = 'zh' AND LOWER(TRIM(text)) = ?
			  AND id != ? AND user_id = ?
			LIMIT 1`, normalized, zhWordID, userID).Scan(&confusedWithID)
	default:
		return 0, false, nil
	}

	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("lookup confusion: %w", err)
	}
	return confusedWithID, true, nil
}

// UpsertConfusion records or increments a confusion pair.
func (s *Store) UpsertConfusion(ctx context.Context, zhWordID, confusedWithID int64, mode string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO confusion_pairs (zh_word_id, confused_with_id, mode, count, last_seen)
		VALUES (?, ?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(zh_word_id, confused_with_id, mode)
		DO UPDATE SET count = count + 1, last_seen = CURRENT_TIMESTAMP`,
		zhWordID, confusedWithID, mode)
	if err != nil {
		return fmt.Errorf("upsert confusion: %w", err)
	}
	return nil
}

// GetConfusionDetail returns a single ConfusionDetail for use in the answer response.
func (s *Store) GetConfusionDetail(ctx context.Context, zhWordID, confusedWithID int64, mode string, langs []string) (*models.ConfusionDetail, error) {
	var d models.ConfusionDetail
	var lastSeen string
	err := s.db.QueryRowContext(ctx, `
		SELECT cp.zh_word_id, wz.text, wz.pinyin,
		       cp.confused_with_id, wc.text, wc.pinyin,
		       cp.mode, cp.count, cp.last_seen
		FROM confusion_pairs cp
		JOIN words wz ON wz.id = cp.zh_word_id
		JOIN words wc ON wc.id = cp.confused_with_id
		WHERE cp.zh_word_id = ? AND cp.confused_with_id = ? AND cp.mode = ?`,
		zhWordID, confusedWithID, mode).Scan(
		&d.ZhWordID, &d.ZhText, &d.ZhPinyin,
		&d.ConfusedWithID, &d.ConfusedWithText, &d.ConfusedWithPinyin,
		&d.Mode, &d.Count, &lastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get confusion detail: %w", err)
	}
	d.LastSeen = parseDateTime(lastSeen)
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	d.ZhTranslations = map[string][]string{}
	d.ConfusedWithTranslations = map[string][]string{}
	for _, lang := range langs {
		texts, ferr := s.getTranslationTextsForZhWord(ctx, zhWordID, lang)
		if ferr != nil {
			return nil, ferr
		}
		if len(texts) > 0 {
			d.ZhTranslations[lang] = texts
		}
		texts, ferr = s.getTranslationTextsForZhWord(ctx, confusedWithID, lang)
		if ferr != nil {
			return nil, ferr
		}
		if len(texts) > 0 {
			d.ConfusedWithTranslations[lang] = texts
		}
	}
	return &d, nil
}

// GetConfusions returns all confusion pairs for the given user, ordered by last_seen DESC.
func (s *Store) GetConfusions(ctx context.Context, userID int64) ([]models.ConfusionDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT cp.zh_word_id, wz.text, wz.pinyin,
		       cp.confused_with_id, wc.text, wc.pinyin,
		       cp.mode, cp.count, cp.last_seen
		FROM confusion_pairs cp
		JOIN words wz ON wz.id = cp.zh_word_id
		JOIN words wc ON wc.id = cp.confused_with_id
		WHERE wz.user_id = ?
		ORDER BY cp.last_seen DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get confusions: %w", err)
	}
	defer rows.Close()

	var items []models.ConfusionDetail
	for rows.Next() {
		var d models.ConfusionDetail
		var lastSeen string
		if err := rows.Scan(
			&d.ZhWordID, &d.ZhText, &d.ZhPinyin,
			&d.ConfusedWithID, &d.ConfusedWithText, &d.ConfusedWithPinyin,
			&d.Mode, &d.Count, &lastSeen,
		); err != nil {
			return nil, fmt.Errorf("scan confusion: %w", err)
		}
		d.LastSeen = parseDateTime(lastSeen)
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close() // release before per-row queries

	allLangs, err := s.GetTranslationLanguages(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].ZhTranslations = map[string][]string{}
		items[i].ConfusedWithTranslations = map[string][]string{}
		for _, lang := range allLangs {
			texts, ferr := s.getTranslationTextsForZhWord(ctx, items[i].ZhWordID, lang)
			if ferr != nil {
				return nil, ferr
			}
			if len(texts) > 0 {
				items[i].ZhTranslations[lang] = texts
			}
			texts, ferr = s.getTranslationTextsForZhWord(ctx, items[i].ConfusedWithID, lang)
			if ferr != nil {
				return nil, ferr
			}
			if len(texts) > 0 {
				items[i].ConfusedWithTranslations[lang] = texts
			}
		}
	}
	if items == nil {
		items = []models.ConfusionDetail{}
	}
	return items, nil
}

// sm2PrevState is the internal JSON encoding for SaveSM2PrevState.
type sm2PrevState struct {
	Easiness        float64 `json:"ef"`
	Repetitions     int     `json:"reps"`
	IntervalDays    int     `json:"iv"`
	TotalCorrect    int     `json:"tc"`
	TotalAttempts   int     `json:"ta"`
	StreakBonus     int     `json:"sb"`
	LearningNewWord bool    `json:"lnw"`
}

// SaveSM2PrevState serialises p to JSON and stores it in the prev_state column
// of sm2_progress for the given word. Called before applying a wrong answer so
// AcceptCorrect can restore the pre-answer state without trusting client data.
func (s *Store) SaveSM2PrevState(ctx context.Context, wordID int64, p models.SM2Progress) error {
	blob, err := json.Marshal(sm2PrevState{
		Easiness:        p.Easiness,
		Repetitions:     p.Repetitions,
		IntervalDays:    p.IntervalDays,
		TotalCorrect:    p.TotalCorrect,
		TotalAttempts:   p.TotalAttempts,
		StreakBonus:     p.StreakBonus,
		LearningNewWord: p.LearningNewWord,
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
		WordID:          wordID,
		Easiness:        prev.Easiness,
		Repetitions:     prev.Repetitions,
		IntervalDays:    prev.IntervalDays,
		TotalCorrect:    prev.TotalCorrect,
		TotalAttempts:   prev.TotalAttempts,
		StreakBonus:     prev.StreakBonus,
		LearningNewWord: prev.LearningNewWord,
	}, nil
}

// ClearSM2PrevState sets prev_state = NULL for the given word.
// Called after a correct answer or after AcceptCorrect.
func (s *Store) ClearSM2PrevState(ctx context.Context, wordID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET prev_state = NULL WHERE word_id = ?`, wordID)
	return err
}
