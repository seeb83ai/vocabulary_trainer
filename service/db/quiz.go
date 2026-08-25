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

// DetectConfusion checks if the user's wrong answer matches a different known word.
// For zh_to_en / zh_pinyin_to_en: looks for a translation word (restricted to langs)
// whose text matches the answer, then returns the zh word it belongs to (if different
// from zhWordID).
// For en_to_zh: looks for a ZH word whose text matches the answer (if different from zhWordID).
// Returns (confusedWithID, true, nil) if a confusion is found, (0, false, nil) if not.
func (s *Store) DetectConfusion(ctx context.Context, userID, zhWordID int64, answer, mode string, langs []string) (int64, bool, error) {
	normalized := sm2.NormalizeAnswer(answer)
	if normalized == "" {
		return 0, false, nil
	}

	switch mode {
	case models.ModeZhToTransl, models.ModeZhPinyinToTransl, models.ModeZhToTranslNoSound, models.ModeVoiceToTransl:
		// Fetch all translation words for the user across ALL languages (excluding
		// translations of the word being quizzed), then match in Go using ExpandVariants.
		// Language-agnostic: when transl_to_zh has no translations in the selected lang
		// it falls back to zh_to_transl, so the typed answer may be in any language.
		// Go-level matching also handles SQLite LOWER() not folding non-ASCII (umlauts)
		// and slash-separated alternatives (e.g. "dog / hound").
		rows, qErr := s.db.QueryContext(ctx, `
			SELECT t.zh_word_id, w.text FROM words w
			JOIN translations t ON t.translation_word_id = w.id
			JOIN words wz ON wz.id = t.zh_word_id
			WHERE t.zh_word_id != ?
			  AND wz.user_id = ?`, zhWordID, userID)
		if qErr != nil {
			return 0, false, fmt.Errorf("lookup confusion: %w", qErr)
		}
		defer rows.Close()
		for rows.Next() {
			var zhID int64
			var text string
			if sErr := rows.Scan(&zhID, &text); sErr != nil {
				return 0, false, fmt.Errorf("scan confusion: %w", sErr)
			}
			for _, v := range sm2.ExpandVariants(text) {
				if v == normalized {
					_ = rows.Close()
					return zhID, true, nil
				}
			}
		}
		return 0, false, rows.Err()

	case models.ModeTranslToZh:
		// First: find a ZH word whose text matches the answer (user typed Chinese).
		var confusedWithID int64
		err := s.db.QueryRowContext(ctx, `
			SELECT id FROM words
			WHERE language = 'zh' AND LOWER(TRIM(text)) = ?
			  AND id != ? AND user_id = ?
			LIMIT 1`, normalized, zhWordID, userID).Scan(&confusedWithID)
		if err != nil && err != sql.ErrNoRows {
			return 0, false, fmt.Errorf("lookup confusion: %w", err)
		}
		if err == nil {
			return confusedWithID, true, nil
		}

		// Second: user may have typed a translation of a different zh word.
		// Fetch all translation words for the user (excluding current word's
		// translations) and match in Go using ExpandVariants so that umlauts
		// and slash-separated alternatives are handled correctly.
		if len(langs) == 0 {
			langs = []string{"en"}
		}
		placeholders := make([]string, len(langs))
		args := make([]any, 0, len(langs)+2)
		for i, l := range langs {
			placeholders[i] = "?"
			args = append(args, l)
		}
		args = append(args, zhWordID, userID)
		rows, qErr := s.db.QueryContext(ctx, `
			SELECT t.zh_word_id, w.text FROM words w
			JOIN translations t ON t.translation_word_id = w.id
			JOIN words wz ON wz.id = t.zh_word_id
			WHERE w.language IN (`+strings.Join(placeholders, ",")+`)
			  AND t.zh_word_id != ?
			  AND wz.user_id = ?`, args...)
		if qErr != nil {
			return 0, false, fmt.Errorf("lookup confusion translations: %w", qErr)
		}
		defer rows.Close()
		for rows.Next() {
			var zhID int64
			var text string
			if sErr := rows.Scan(&zhID, &text); sErr != nil {
				return 0, false, fmt.Errorf("scan confusion: %w", sErr)
			}
			for _, v := range sm2.ExpandVariants(text) {
				if v == normalized {
					_ = rows.Close()
					return zhID, true, nil
				}
			}
		}
		return 0, false, rows.Err()

	default:
		return 0, false, nil
	}
}

// upsertConfusion records or increments a confusion pair. Exactly one of
// zhWordID/zhComponent and one of confusedWithID/confusedWithComponent should
// be set (the other left at its zero value) on each side, per the caller's
// detection result. userID is required directly (rather than inferred by
// joining through words) because component characters, unlike word ids, are
// not inherently scoped to one user.
func (s *Store) upsertConfusion(ctx context.Context, userID, zhWordID int64, zhComponent string, confusedWithID int64, confusedWithComponent, mode string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO confusion_pairs (user_id, zh_word_id, zh_component, confused_with_id, confused_with_component, mode, count, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, zh_word_id, zh_component, confused_with_id, confused_with_component, mode)
		DO UPDATE SET count = count + 1, last_seen = CURRENT_TIMESTAMP`,
		userID, zhWordID, zhComponent, confusedWithID, confusedWithComponent, mode)
	if err != nil {
		return fmt.Errorf("upsert confusion: %w", err)
	}
	return nil
}

// UpsertConfusion records or increments a word-vs-word confusion pair.
func (s *Store) UpsertConfusion(ctx context.Context, userID, zhWordID, confusedWithID int64, mode string) error {
	return s.upsertConfusion(ctx, userID, zhWordID, "", confusedWithID, "", mode)
}

// GetConfusionDetail returns a single word-vs-word ConfusionDetail for use in
// the vocabulary-word answer response.
func (s *Store) GetConfusionDetail(ctx context.Context, userID, zhWordID, confusedWithID int64, mode string, langs []string) (*models.ConfusionDetail, error) {
	var d models.ConfusionDetail
	var lastSeen string
	err := s.db.QueryRowContext(ctx, `
		SELECT cp.zh_word_id, wz.text, wz.pinyin,
		       cp.confused_with_id, wc.text, wc.pinyin,
		       cp.mode, cp.count, cp.last_seen
		FROM confusion_pairs cp
		JOIN words wz ON wz.id = cp.zh_word_id
		JOIN words wc ON wc.id = cp.confused_with_id
		WHERE cp.user_id = ? AND cp.zh_word_id = ? AND cp.confused_with_id = ? AND cp.mode = ?`,
		userID, zhWordID, confusedWithID, mode).Scan(
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
	d.ZhKind = models.ConfusionKindWord
	d.ConfusedWithKind = models.ConfusionKindWord
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

// rawConfusionRow is the shape of a confusion_pairs row before its word/component
// sides have been resolved to display data.
type rawConfusionRow struct {
	zhWordID, confusedWithID           int64
	zhComponent, confusedWithComponent string
	mode                               string
	count                              int
	lastSeen                           string
}

// resolveConfusionEntity resolves one side of a confusion pair (word if wordID
// != 0, otherwise component) into display data. ok=false means the referenced
// word no longer exists (e.g. a pre-cleanup dangling row) and the row should
// be skipped, matching the old behaviour of silently dropping such rows via
// an INNER JOIN.
func (s *Store) resolveConfusionEntity(ctx context.Context, userID, wordID int64, component string, langs []string) (kind, text string, pinyin *string, translations map[string][]string, ok bool, err error) {
	translations = map[string][]string{}
	if wordID != 0 {
		var t string
		var p sql.NullString
		err = s.db.QueryRowContext(ctx, `SELECT text, pinyin FROM words WHERE id = ? AND user_id = ?`, wordID, userID).Scan(&t, &p)
		if err == sql.ErrNoRows {
			return "", "", nil, nil, false, nil
		}
		if err != nil {
			return "", "", nil, nil, false, fmt.Errorf("resolve confusion word %d: %w", wordID, err)
		}
		if p.Valid {
			pinyin = &p.String
		}
		for _, lang := range langs {
			texts, ferr := s.getTranslationTextsForZhWord(ctx, wordID, lang)
			if ferr != nil {
				return "", "", nil, nil, false, ferr
			}
			if len(texts) > 0 {
				translations[lang] = texts
			}
		}
		return models.ConfusionKindWord, t, pinyin, translations, true, nil
	}

	// Component side.
	if py := s.GetComponentPinyin(ctx, component); py != "" {
		pinyin = &py
	}
	defs, dErr := s.GetComponentDefinitions(ctx, userID, component, langs)
	if dErr != nil {
		return "", "", nil, nil, false, dErr
	}
	for lang, def := range defs {
		translations[lang] = []string{def}
	}
	return models.ConfusionKindComponent, component, pinyin, translations, true, nil
}

func (s *Store) hydrateConfusionRows(ctx context.Context, userID int64, raws []rawConfusionRow) ([]models.ConfusionDetail, error) {
	allLangs, err := s.GetTranslationLanguages(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]models.ConfusionDetail, 0, len(raws))
	for _, r := range raws {
		d := models.ConfusionDetail{Mode: r.mode, Count: r.count, LastSeen: parseDateTime(r.lastSeen)}
		var zhOK, cwOK bool
		d.ZhKind, d.ZhText, d.ZhPinyin, d.ZhTranslations, zhOK, err = s.resolveConfusionEntity(ctx, userID, r.zhWordID, r.zhComponent, allLangs)
		if err != nil {
			return nil, err
		}
		d.ConfusedWithKind, d.ConfusedWithText, d.ConfusedWithPinyin, d.ConfusedWithTranslations, cwOK, err =
			s.resolveConfusionEntity(ctx, userID, r.confusedWithID, r.confusedWithComponent, allLangs)
		if err != nil {
			return nil, err
		}
		if !zhOK || !cwOK {
			continue
		}
		d.ZhWordID, d.ZhComponent = r.zhWordID, r.zhComponent
		d.ConfusedWithID, d.ConfusedWithComponent = r.confusedWithID, r.confusedWithComponent
		items = append(items, d)
	}
	return items, nil
}

// GetConfusions returns all confusion pairs for the given user, ordered by last_seen DESC.
func (s *Store) GetConfusions(ctx context.Context, userID int64) ([]models.ConfusionDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT zh_word_id, zh_component, confused_with_id, confused_with_component, mode, count, last_seen
		FROM confusion_pairs
		WHERE user_id = ?
		ORDER BY last_seen DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get confusions: %w", err)
	}
	var raws []rawConfusionRow
	for rows.Next() {
		var r rawConfusionRow
		if err := rows.Scan(&r.zhWordID, &r.zhComponent, &r.confusedWithID, &r.confusedWithComponent, &r.mode, &r.count, &r.lastSeen); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan confusion: %w", err)
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	items, err := s.hydrateConfusionRows(ctx, userID, raws)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []models.ConfusionDetail{}
	}
	return items, nil
}

// GetRecentMismatches returns confusion pairs with last_seen >= since that have
// not yet been shown in a game (or have been re-confused since last shown), up to limit rows.
func (s *Store) GetRecentMismatches(ctx context.Context, userID int64, since time.Time, limit int) ([]models.ConfusionDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT zh_word_id, zh_component, confused_with_id, confused_with_component, mode, count, last_seen
		FROM confusion_pairs
		WHERE user_id = ?
		  AND last_seen >= ?
		  AND (last_shown_in_game IS NULL OR last_seen > last_shown_in_game)
		ORDER BY last_seen DESC
		LIMIT ?`,
		userID, since.UTC().Format("2006-01-02 15:04:05"), limit)
	if err != nil {
		return nil, fmt.Errorf("get recent mismatches: %w", err)
	}
	var raws []rawConfusionRow
	for rows.Next() {
		var r rawConfusionRow
		if err := rows.Scan(&r.zhWordID, &r.zhComponent, &r.confusedWithID, &r.confusedWithComponent, &r.mode, &r.count, &r.lastSeen); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan recent mismatch: %w", err)
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	items, err := s.hydrateConfusionRows(ctx, userID, raws)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []models.ConfusionDetail{}
	}
	return items, nil
}

// ConfusionPairKey identifies one side-pair of a confusion_pairs row (each
// side being either a word id or a component character). UserID is required
// directly (rather than inferred by joining through words) because component
// characters, unlike word ids, are not inherently scoped to one user — the
// same character can be trained, and independently confused, by many users.
type ConfusionPairKey struct {
	UserID                int64
	ZhWordID              int64
	ZhComponent           string
	ConfusedWithID        int64
	ConfusedWithComponent string
	Mode                  string
}

// MarkConfusionsShownInGame stamps the given pairs with the current time so
// they are excluded from future match-game sessions unless the user confuses
// them again after this timestamp.
func (s *Store) MarkConfusionsShownInGame(ctx context.Context, pairs []ConfusionPairKey) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for _, p := range pairs {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE confusion_pairs SET last_shown_in_game = ?
			 WHERE user_id = ? AND zh_word_id = ? AND zh_component = ? AND confused_with_id = ? AND confused_with_component = ? AND mode = ?`,
			now, p.UserID, p.ZhWordID, p.ZhComponent, p.ConfusedWithID, p.ConfusedWithComponent, p.Mode,
		); err != nil {
			return fmt.Errorf("mark confusion shown: %w", err)
		}
	}
	return nil
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
