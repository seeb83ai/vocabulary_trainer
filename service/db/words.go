package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// validSortExprs maps allowed sort keys to their SQL ORDER BY expressions.
// Values may contain multiple comma-separated terms; all use the same direction.
var validSortExprs = map[string]string{
	"zh":          "w.text",
	"pinyin":      "w.pinyin",
	"en":          "(SELECT MIN(ew.text) FROM words ew JOIN translations t ON t.en_word_id = ew.id AND t.zh_word_id = w.id WHERE ew.language = 'en')",
	"de":          "(SELECT MIN(ew.text) FROM words ew JOIN translations t ON t.en_word_id = ew.id AND t.zh_word_id = w.id WHERE ew.language = 'de')",
	"repetitions": "COALESCE(p.repetitions, 0)|CAST(COALESCE(p.total_correct + p.streak_bonus, 0) AS REAL) / NULLIF(COALESCE(p.total_attempts, 0), 0)",
	"due_date":    "COALESCE(p.due_date, CURRENT_TIMESTAMP)",
	"accuracy":    "CAST(COALESCE(p.total_correct + p.streak_bonus, 0) AS REAL) / NULLIF(COALESCE(p.total_attempts, 0), 0)|COALESCE(p.total_attempts, 0)",
}

// GetWords returns a paginated list of vocabulary entries (zh words with their en translations).
// If reviewOnly is true, only words with needs_review = 1 are returned.
// If hideUnseen is true, only words with at least one quiz attempt are returned.
// bucket filters by accuracy tier (same rules as tierFilter / wordTier in app.js).
func (s *Store) GetWords(ctx context.Context, userID int64, q string, page, perPage int, sortBy, sortDir string, tags []string, reviewOnly bool, hideUnseen bool, bucket string, dueFilter string, missingLang string) ([]models.WordDetail, int, error) {
	exportAll := perPage <= 0
	if exportAll {
		page = 1
	} else {
		if page < 1 {
			page = 1
		}
		if perPage > 100 {
			perPage = 100
		}
	}
	offset := (page - 1) * perPage

	q = strings.TrimSpace(q)

	// Build optional tag filter clause
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

	reviewFilter := ""
	if reviewOnly {
		reviewFilter = " AND w.needs_review = 1"
	}

	hideUnseenFilter := ""
	if hideUnseen {
		hideUnseenFilter = " AND COALESCE(p.total_attempts, 0) > 0"
	}

	dueFilterSQL := ""
	switch dueFilter {
	case "today":
		dueFilterSQL = " AND p.due_date < date('now', '+1 day')"
	case "tomorrow":
		dueFilterSQL = " AND p.due_date >= date('now', '+1 day') AND p.due_date < date('now', '+2 day')"
	}

	bucketFilter := tierFilter(bucket)

	missingLangFilter := ""
	var missingLangArgs []any
	if missingLang == "en" || missingLang == "de" {
		missingLangFilter = ` AND NOT EXISTS (
			SELECT 1 FROM translations t
			JOIN words tw ON t.en_word_id = tw.id
			WHERE t.zh_word_id = w.id AND tw.language = ?
		)`
		missingLangArgs = []any{missingLang}
	}

	orderExpr, ok := validSortExprs[sortBy]
	if !ok {
		orderExpr = "w.created_at"
	}
	if sortDir != "asc" {
		sortDir = "desc"
	}
	// Build "term1 dir, term2 dir, ..." from a pipe-separated expression.
	orderTerms := strings.Split(orderExpr, "|")
	for i, t := range orderTerms {
		orderTerms[i] = t + " " + sortDir
	}
	orderClause := strings.Join(orderTerms, ", ")

	limitClause := "\n\t\tLIMIT ? OFFSET ?"
	if exportAll {
		limitClause = ""
	}

	// Single query: COUNT(*) OVER() returns the total alongside each row,
	// eliminating the separate count query.
	listQuery := `
		SELECT w.id, w.text, w.pinyin, w.created_at,
		       COALESCE(p.repetitions, 0), COALESCE(p.easiness, 2.5),
		       COALESCE(p.interval_days, 1),
		       COALESCE(p.total_correct, 0), COALESCE(p.total_attempts, 0),
		       COALESCE(p.streak_bonus, 0),
		       COALESCE(p.due_date, CURRENT_TIMESTAMP),
		       COALESCE(w.needs_review, 0),
		       COALESCE(p.learning_new_word, 1),
		       COUNT(*) OVER() AS total
		FROM words w
		LEFT JOIN sm2_progress p ON p.word_id = w.id
		WHERE w.language = 'zh'
		  AND w.user_id = ?
		  AND (? = '' OR w.text LIKE '%' || ? || '%'
		       OR w.pinyin LIKE '%' || ? || '%'
		       OR EXISTS (
		           SELECT 1 FROM words ew
		           JOIN translations t ON t.en_word_id = ew.id AND t.zh_word_id = w.id
		           WHERE ew.text LIKE '%' || ? || '%'
		       ))` + tagFilter + reviewFilter + hideUnseenFilter + bucketFilter + dueFilterSQL + missingLangFilter + `
		ORDER BY ` + orderClause + limitClause
	listArgs := []any{userID, q, q, q, q}
	listArgs = append(listArgs, tagArgs...)
	listArgs = append(listArgs, missingLangArgs...)
	if !exportAll {
		listArgs = append(listArgs, perPage, offset)
	}
	rows, err := s.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list words: %w", err)
	}
	defer rows.Close()

	var total int
	var words []models.WordDetail
	for rows.Next() {
		var wd models.WordDetail
		var createdAt, dueDate string
		var needsReview, learning int
		if err := rows.Scan(
			&wd.ID, &wd.ZhText, &wd.Pinyin, &createdAt,
			&wd.Repetitions, &wd.Easiness, &wd.IntervalDays,
			&wd.TotalCorrect, &wd.TotalAttempts, &wd.StreakBonus,
			&dueDate, &needsReview, &learning,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan word: %w", err)
		}
		wd.NeedsReview = needsReview == 1
		wd.LearningNewWord = learning == 1
		wd.CreatedAt = parseDateTime(createdAt)
		wd.DueDate = parseDateTime(dueDate)
		words = append(words, wd)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()

	// Batch-load translations and tags for all page results.
	if len(words) > 0 {
		ids := make([]int64, len(words))
		idIndex := make(map[int64]int, len(words))
		for i, w := range words {
			ids[i] = w.ID
			idIndex[w.ID] = i
		}
		enMap, err := s.batchLoadTranslationTexts(ctx, ids, "en")
		if err != nil {
			return nil, 0, err
		}
		deMap, err := s.batchLoadTranslationTexts(ctx, ids, "de")
		if err != nil {
			return nil, 0, err
		}
		for i, w := range words {
			if t := enMap[w.ID]; t != nil {
				words[i].EnTexts = t
			} else {
				words[i].EnTexts = []string{}
			}
			if t := deMap[w.ID]; t != nil {
				words[i].DeTexts = t
			} else {
				words[i].DeTexts = []string{}
			}
		}
		if err := s.batchLoadTags(ctx, words, ids, idIndex); err != nil {
			return nil, 0, err
		}
	}
	if words == nil {
		words = []models.WordDetail{}
	}
	return words, total, nil
}

// batchLoadTranslationTexts loads all translation texts for the given zh word IDs
// filtered by the given language ('en' or 'de'), returning a map of zhID → texts.
func (s *Store) batchLoadTranslationTexts(ctx context.Context, ids []int64, lang string) (map[int64][]string, error) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids)+1)
	args[0] = lang
	for i, id := range ids {
		placeholders[i] = "?"
		args[i+1] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.zh_word_id, w.text FROM words w
		 JOIN translations t ON t.en_word_id = w.id
		 WHERE w.language = ?
		   AND t.zh_word_id IN (`+strings.Join(placeholders, ",")+`)
		 ORDER BY w.text`, args...)
	if err != nil {
		return nil, fmt.Errorf("batch %s texts: %w", lang, err)
	}
	defer rows.Close()
	result := make(map[int64][]string)
	for rows.Next() {
		var zhID int64
		var text string
		if err := rows.Scan(&zhID, &text); err != nil {
			return nil, err
		}
		result[zhID] = append(result[zhID], text)
	}
	return result, rows.Err()
}

func (s *Store) batchLoadTags(ctx context.Context, words []models.WordDetail, ids []int64, idIndex map[int64]int) error {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT wt.word_id, tg.name FROM tags tg
		 JOIN word_tags wt ON wt.tag_id = tg.id
		 WHERE wt.word_id IN (`+strings.Join(placeholders, ",")+`)
		 ORDER BY tg.name`, args...)
	if err != nil {
		return fmt.Errorf("batch tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var wordID int64
		var tag string
		if err := rows.Scan(&wordID, &tag); err != nil {
			return err
		}
		idx := idIndex[wordID]
		words[idx].Tags = append(words[idx].Tags, tag)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range words {
		if words[i].Tags == nil {
			words[i].Tags = []string{}
		}
	}
	return nil
}

func (s *Store) getTranslationTextsForZhWord(ctx context.Context, zhID int64, lang string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.text FROM words w
		 JOIN translations t ON t.en_word_id = w.id
		 WHERE t.zh_word_id = ? AND w.language = ?
		 ORDER BY w.text`, zhID, lang)
	if err != nil {
		return nil, fmt.Errorf("get %s texts: %w", lang, err)
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var txt string
		if err := rows.Scan(&txt); err != nil {
			return nil, err
		}
		texts = append(texts, txt)
	}
	if texts == nil {
		texts = []string{}
	}
	return texts, rows.Err()
}

// GetWordByID returns a single zh word with all its English translations.
func (s *Store) GetWordByID(ctx context.Context, userID, id int64) (*models.WordDetail, error) {
	var wd models.WordDetail
	var createdAt, dueDate string
	var needsReview int
	err := s.db.QueryRowContext(ctx,
		`SELECT w.id, w.text, w.pinyin, w.created_at,
		        COALESCE(p.repetitions, 0), COALESCE(p.easiness, 2.5),
		        COALESCE(p.interval_days, 1),
		        COALESCE(p.total_correct, 0), COALESCE(p.total_attempts, 0),
		        COALESCE(p.streak_bonus, 0),
		        COALESCE(p.due_date, CURRENT_TIMESTAMP),
		        COALESCE(w.needs_review, 0)
		 FROM words w
		 LEFT JOIN sm2_progress p ON p.word_id = w.id
		 WHERE w.id = ? AND w.language = 'zh' AND w.user_id = ?`, id, userID).
		Scan(&wd.ID, &wd.ZhText, &wd.Pinyin, &createdAt,
			&wd.Repetitions, &wd.Easiness, &wd.IntervalDays,
			&wd.TotalCorrect, &wd.TotalAttempts, &wd.StreakBonus,
			&dueDate, &needsReview)
	wd.CreatedAt = parseDateTime(createdAt)
	wd.DueDate = parseDateTime(dueDate)
	wd.NeedsReview = needsReview == 1
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get word by id: %w", err)
	}
	enTexts, err := s.getTranslationTextsForZhWord(ctx, id, "en")
	if err != nil {
		return nil, err
	}
	wd.EnTexts = enTexts
	deTexts, err := s.getTranslationTextsForZhWord(ctx, id, "de")
	if err != nil {
		return nil, err
	}
	wd.DeTexts = deTexts
	wd.Tags, err = s.getTagsForWord(ctx, id)
	if err != nil {
		return nil, err
	}
	wd.SceneText, _ = s.GetHMMSceneText(ctx, id)
	return &wd, nil
}

// CreateWord creates (or reuses) the zh word + en words and links them.
// Returns the zh word ID.
func (s *Store) CreateWord(ctx context.Context, userID int64, req models.CreateWordRequest) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	zhID, err := upsertWord(ctx, tx, req.ZhText, "zh", &req.Pinyin, userID)
	if err != nil {
		return 0, err
	}
	if err := initSM2(ctx, tx, zhID); err != nil {
		return 0, err
	}

	for _, enText := range req.EnTexts {
		enText = strings.TrimSpace(enText)
		if enText == "" {
			continue
		}
		enID, err := upsertWord(ctx, tx, enText, "en", nil, userID)
		if err != nil {
			return 0, err
		}
		if err := initSM2(ctx, tx, enID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`,
			enID, zhID); err != nil {
			return 0, fmt.Errorf("link en translation: %w", err)
		}
	}

	for _, deText := range req.DeTexts {
		deText = strings.TrimSpace(deText)
		if deText == "" {
			continue
		}
		deID, err := upsertWord(ctx, tx, deText, "de", nil, userID)
		if err != nil {
			return 0, err
		}
		if err := initSM2(ctx, tx, deID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`,
			deID, zhID); err != nil {
			return 0, fmt.Errorf("link de translation: %w", err)
		}
	}

	if err := setWordTags(ctx, tx, zhID, req.Tags); err != nil {
		return 0, err
	}

	return zhID, tx.Commit()
}

// UpdateWord updates zh word text/pinyin and replaces all translation links.
func (s *Store) UpdateWord(ctx context.Context, userID int64, id int64, req models.UpdateWordRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var pinyin *string
	if p := strings.TrimSpace(req.Pinyin); p != "" {
		pinyin = &p
	}

	newText := strings.TrimSpace(req.ZhText)

	// Read current text so we can skip updating it when unchanged — avoids
	// a spurious UNIQUE constraint violation caused by duplicate zh rows in
	// the database (e.g. after manual migrations) or by the form's trim().
	var currentText string
	err = tx.QueryRowContext(ctx,
		`SELECT text FROM words WHERE id = ? AND language = 'zh' AND user_id = ?`, id, userID).Scan(&currentText)
	if err == sql.ErrNoRows {
		return sql.ErrNoRows
	} else if err != nil {
		return fmt.Errorf("get current word: %w", err)
	}

	var res sql.Result
	if currentText == newText {
		res, err = tx.ExecContext(ctx,
			`UPDATE words SET pinyin = ?, needs_review = 0 WHERE id = ? AND language = 'zh' AND user_id = ?`,
			pinyin, id, userID)
	} else {
		res, err = tx.ExecContext(ctx,
			`UPDATE words SET text = ?, pinyin = ?, needs_review = 0 WHERE id = ? AND language = 'zh' AND user_id = ?`,
			newText, pinyin, id, userID)
	}
	if err != nil {
		return fmt.Errorf("update word: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}

	// Remove old translation links (not the en word rows themselves)
	if _, err := tx.ExecContext(ctx, `DELETE FROM translations WHERE zh_word_id = ?`, id); err != nil {
		return fmt.Errorf("delete translations: %w", err)
	}

	for _, enText := range req.EnTexts {
		enText = strings.TrimSpace(enText)
		if enText == "" {
			continue
		}
		enID, err := upsertWord(ctx, tx, enText, "en", nil, userID)
		if err != nil {
			return err
		}
		if err := initSM2(ctx, tx, enID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`,
			enID, id); err != nil {
			return fmt.Errorf("link en translation: %w", err)
		}
	}

	for _, deText := range req.DeTexts {
		deText = strings.TrimSpace(deText)
		if deText == "" {
			continue
		}
		deID, err := upsertWord(ctx, tx, deText, "de", nil, userID)
		if err != nil {
			return err
		}
		if err := initSM2(ctx, tx, deID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`,
			deID, id); err != nil {
			return fmt.Errorf("link de translation: %w", err)
		}
	}

	if err := setWordTags(ctx, tx, id, req.Tags); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return s.cleanOrphanTags(ctx)
}

// AddTranslation appends a single EN text as a new translation for the given zh word ID.
// If the EN word already exists it is reused; if the link already exists it is a no-op.
func (s *Store) AddTranslation(ctx context.Context, userID int64, zhID int64, enText string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Verify the zh word exists
	var exists int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM words WHERE id = ? AND language = 'zh'`, zhID).Scan(&exists); err != nil {
		return fmt.Errorf("check word: %w", err)
	}
	if exists == 0 {
		return sql.ErrNoRows
	}

	enID, err := upsertWord(ctx, tx, enText, "en", nil, userID)
	if err != nil {
		return err
	}
	if err := initSM2(ctx, tx, enID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`,
		enID, zhID); err != nil {
		return fmt.Errorf("link translation: %w", err)
	}
	return tx.Commit()
}

// DeleteWord deletes a word by ID. Cascades to translations and sm2_progress.
func (s *Store) DeleteWord(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM words WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete word: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return s.cleanOrphanTags(ctx)
}

// MarkWordForReview sets needs_review = 1 for the given zh word ID.
func (s *Store) MarkWordForReview(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE words SET needs_review = 1 WHERE id = ? AND language = 'zh' AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("mark for review: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetTranslationLanguages returns the distinct non-zh languages that have at
// least one translation row in the database.
func (s *Store) GetTranslationLanguages(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT w.language FROM words w
		 JOIN translations t ON t.en_word_id = w.id
		 WHERE w.language != 'zh'
		 ORDER BY w.language`)
	if err != nil {
		return nil, fmt.Errorf("get translation languages: %w", err)
	}
	defer rows.Close()
	var langs []string
	for rows.Next() {
		var lang string
		if err := rows.Scan(&lang); err != nil {
			return nil, err
		}
		langs = append(langs, lang)
	}
	return langs, rows.Err()
}

// GetTranslationsForWord returns all words in targetLang linked to wordID.
func (s *Store) GetTranslationsForWord(ctx context.Context, wordID int64, targetLang string) ([]models.Word, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.text, w.language, w.pinyin, w.created_at
		 FROM words w
		 JOIN translations t ON t.en_word_id = w.id
		 WHERE t.zh_word_id = ? AND w.language = ?`, wordID, targetLang)
	if err != nil {
		return nil, fmt.Errorf("get translations: %w", err)
	}
	defer rows.Close()
	var words []models.Word
	for rows.Next() {
		var w models.Word
		var createdAt string
		if err := rows.Scan(&w.ID, &w.Text, &w.Language, &w.Pinyin, &createdAt); err != nil {
			return nil, err
		}
		w.CreatedAt = parseDateTime(createdAt)
		words = append(words, w)
	}
	return words, rows.Err()
}

// tierFilter returns the SQL WHERE fragment (prefixed with AND) that restricts
// rows to the given accuracy/attempt bucket. The alias "p" must refer to
// sm2_progress in the enclosing query. Returns "" for an empty/unknown key.
//
// Bucket rules (must be kept in sync with wordTier in app.js
// in GetWordStats):
//
//	0-49   : < 3 attempts OR accuracy < 50 %   (includes new/unseen words)
//	50-69  : ≥ 3 attempts AND acc ≥ 50 % (but not qualifying for 70-84 or 85-100)
//	70-84  : ≥ 10 attempts AND 70 % ≤ acc < 85 %
//	85-100 : ≥ 10 attempts AND acc ≥ 85 %
func tierFilter(bucket string) string {
	const acc = `CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts`
	switch bucket {
	case "new":
		return ` AND p.learning_new_word = 1 AND p.first_seen_date IS NOT NULL`
	case "0-49":
		return ` AND p.learning_new_word = 0 AND (p.total_attempts < 3 OR ` + acc + ` < 0.50)`
	case "50-69":
		return ` AND p.learning_new_word = 0 AND p.total_attempts >= 3 AND ` + acc + ` >= 0.50 AND NOT (p.total_attempts >= 10 AND ` + acc + ` >= 0.70)`
	case "70-84":
		return ` AND p.learning_new_word = 0 AND p.total_attempts >= 10 AND ` + acc + ` >= 0.70 AND ` + acc + ` < 0.85`
	case "85-100":
		return ` AND p.learning_new_word = 0 AND p.total_attempts >= 10 AND ` + acc + ` >= 0.85`
	}
	return ""
}

// GetNextCard returns the most-overdue card. Falls back to nearest upcoming if none are due.
// Returns (word, progress, nil) or (nil, nil, nil) if no words exist.
// maxNew caps how many new words (first_seen_date IS NULL) can be introduced today; once
// the count for today reaches maxNew, only already-seen cards are returned.
// skipNew forces unseen words to be excluded regardless of the daily cap.
func (s *Store) GetNextCard(ctx context.Context, userID int64, tags []string, maxNew int, bucket string, skipNew bool) (*models.Word, *models.SM2Progress, error) {
	// Build optional tag filter
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

	// Count new words already introduced today (per user, not per-tag).
	var newToday int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND w.user_id = ? AND p.first_seen_date = date('now')`, userID).Scan(&newToday); err != nil {
		return nil, nil, fmt.Errorf("count new today: %w", err)
	}

	// When the daily cap is reached or the user chose to skip new words then exclude never-presented words.
	newWordFilter := ""
	if skipNew || newToday >= maxNew {
		newWordFilter = " AND p.first_seen_date IS NOT NULL"
	}

	// Only quiz on zh words — they are the canonical unit; en words are just
	// answer targets and should never appear as quiz prompts on their own.
	query := `
		SELECT w.id, w.text, w.language, w.pinyin, w.created_at,
		       p.repetitions, p.easiness, p.interval_days, p.due_date,
		       p.total_correct, p.total_attempts, p.streak_bonus, p.learning_new_word
		FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		WHERE w.language = 'zh' AND w.user_id = ?` + tagFilter + newWordFilter + bucketSQL + ` %s
		ORDER BY p.due_date ASC
		LIMIT 1`

	tryQuery := func(extra string) (*models.Word, *models.SM2Progress, error) {
		args := append([]any{userID}, tagArgs...)
		row := s.db.QueryRowContext(ctx, fmt.Sprintf(query, extra), args...)
		var w models.Word
		var p models.SM2Progress
		var createdAt, dueDate string
		var learning int
		err := row.Scan(
			&w.ID, &w.Text, &w.Language, &w.Pinyin, &createdAt,
			&p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDate,
			&p.TotalCorrect, &p.TotalAttempts, &p.StreakBonus, &learning,
		)
		w.CreatedAt = parseDateTime(createdAt)
		p.DueDate = parseDateTime(dueDate)
		p.LearningNewWord = learning == 1
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get next card: %w", err)
		}
		p.WordID = w.ID
		return &w, &p, nil
	}

	// Only consider cards due by end of today so we don't pull in future
	// cards and inflate the session beyond what due_today reports.
	todayBound := "AND p.due_date < date('now', '+1 day')"

	w, p, err := tryQuery("AND p.due_date <= CURRENT_TIMESTAMP")
	if err != nil || w != nil {
		return w, p, err
	}
	// No overdue cards — prefer cards outside the wrong-retry window so a
	// recently failed card is not immediately repeated.
	w, p, err = tryQuery(fmt.Sprintf("AND p.due_date > datetime('now', '+%d seconds') %s", int(sm2.WrongRetryDelay.Seconds()), todayBound))
	if err != nil || w != nil {
		return w, p, err
	}
	// All remaining cards are within the retry window; return the soonest one.
	return tryQuery(todayBound)
}
