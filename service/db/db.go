package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path and runs schema migrations.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	if err := Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// validSortExprs maps allowed sort keys to their SQL ORDER BY expressions.
// Values may contain multiple comma-separated terms; all use the same direction.
var validSortExprs = map[string]string{
	"zh":          "w.text",
	"pinyin":      "w.pinyin",
	"en":          "(SELECT MIN(ew.text) FROM words ew JOIN translations t ON t.en_word_id = ew.id AND t.zh_word_id = w.id)",
	"repetitions": "COALESCE(p.repetitions, 0)|CAST(COALESCE(p.total_correct + p.streak_bonus, 0) AS REAL) / NULLIF(COALESCE(p.total_attempts, 0), 0)",
	"due_date":    "COALESCE(p.due_date, CURRENT_TIMESTAMP)",
	"accuracy":    "CAST(COALESCE(p.total_correct + p.streak_bonus, 0) AS REAL) / NULLIF(COALESCE(p.total_attempts, 0), 0)|COALESCE(p.total_attempts, 0)",
}

// GetWords returns a paginated list of vocabulary entries (zh words with their en translations).
// If reviewOnly is true, only words with needs_review = 1 are returned.
// If hideUnseen is true, only words with at least one quiz attempt are returned.
// bucket filters by accuracy tier (same rules as tierFilter / wordTier in app.js).
func (s *Store) GetWords(ctx context.Context, q string, page, perPage int, sortBy, sortDir string, tags []string, reviewOnly bool, hideUnseen bool, bucket string, dueFilter string) ([]models.WordDetail, int, error) {
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
		  AND (? = '' OR w.text LIKE '%' || ? || '%'
		       OR w.pinyin LIKE '%' || ? || '%'
		       OR EXISTS (
		           SELECT 1 FROM words ew
		           JOIN translations t ON t.en_word_id = ew.id AND t.zh_word_id = w.id
		           WHERE ew.text LIKE '%' || ? || '%'
		       ))` + tagFilter + reviewFilter + hideUnseenFilter + bucketFilter + dueFilterSQL + `
		ORDER BY ` + orderClause + limitClause
	listArgs := []any{q, q, q, q}
	listArgs = append(listArgs, tagArgs...)
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
func (s *Store) GetWordByID(ctx context.Context, id int64) (*models.WordDetail, error) {
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
		 WHERE w.id = ? AND w.language = 'zh'`, id).
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
func (s *Store) CreateWord(ctx context.Context, req models.CreateWordRequest) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	zhID, err := upsertWord(ctx, tx, req.ZhText, "zh", &req.Pinyin)
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
		enID, err := upsertWord(ctx, tx, enText, "en", nil)
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
		deID, err := upsertWord(ctx, tx, deText, "de", nil)
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
func (s *Store) UpdateWord(ctx context.Context, id int64, req models.UpdateWordRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var pinyin *string
	if p := strings.TrimSpace(req.Pinyin); p != "" {
		pinyin = &p
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE words SET text = ?, pinyin = ?, needs_review = 0 WHERE id = ? AND language = 'zh'`,
		strings.TrimSpace(req.ZhText), pinyin, id)
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
		enID, err := upsertWord(ctx, tx, enText, "en", nil)
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
		deID, err := upsertWord(ctx, tx, deText, "de", nil)
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
func (s *Store) AddTranslation(ctx context.Context, zhID int64, enText string) error {
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

	enID, err := upsertWord(ctx, tx, enText, "en", nil)
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
func (s *Store) DeleteWord(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM words WHERE id = ?`, id)
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
func (s *Store) MarkWordForReview(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE words SET needs_review = 1 WHERE id = ? AND language = 'zh'`, id)
	if err != nil {
		return fmt.Errorf("mark for review: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
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
func (s *Store) GetNextCard(ctx context.Context, tags []string, maxNew int, bucket string, skipNew bool) (*models.Word, *models.SM2Progress, error) {
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

	// Count new words already introduced today (global, not per-tag).
	var newToday int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.first_seen_date = date('now')`).Scan(&newToday); err != nil {
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
		WHERE w.language = 'zh'` + tagFilter + newWordFilter + bucketSQL + ` %s
		ORDER BY p.due_date ASC
		LIMIT 1`

	tryQuery := func(extra string) (*models.Word, *models.SM2Progress, error) {
		args := append([]any{}, tagArgs...)
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

// GetTranslationsForWord returns all words in targetLang linked to wordID.
func (s *Store) GetTranslationsForWord(ctx context.Context, wordID int64, targetLang string) ([]models.Word, error) {
	var rows *sql.Rows
	var err error
	if targetLang == "en" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT w.id, w.text, w.language, w.pinyin, w.created_at
			 FROM words w
			 JOIN translations t ON t.en_word_id = w.id
			 WHERE t.zh_word_id = ?`, wordID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT w.id, w.text, w.language, w.pinyin, w.created_at
			 FROM words w
			 JOIN translations t ON t.zh_word_id = w.id
			 WHERE t.en_word_id = ?`, wordID)
	}
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
func (s *Store) SkipWord(ctx context.Context, wordID int64, days int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = datetime('now', ?) WHERE word_id = ?`,
		fmt.Sprintf("+%d days", days), wordID)
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
func (s *Store) AcknowledgeWord(ctx context.Context, wordID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress
		 SET total_attempts = CASE WHEN total_attempts = 0 THEN 1 ELSE total_attempts END,
		     first_seen_date = COALESCE(first_seen_date, date('now')),
		     due_date = CURRENT_TIMESTAMP
		 WHERE word_id = ?`, wordID)
	if err != nil {
		return fmt.Errorf("acknowledge word: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetStats returns due-today count, total word count (zh words only), and the number of
// new words introduced today (globally, not filtered by tag).
func (s *Store) GetStats(ctx context.Context, tags []string, bucket string) (dueToday, total, newToday int, err error) {
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
	totalArgs := append([]any{}, tagArgs...)
	if bucket != "" {
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM words w JOIN sm2_progress p ON p.word_id = w.id`+
				` WHERE w.language = 'zh'`+tagFilter+bucketSQL, totalArgs...).Scan(&total)
	} else {
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM words w WHERE w.language = 'zh'`+tagFilter, totalArgs...).Scan(&total)
	}
	if err != nil {
		return
	}
	dueArgs := append([]any{}, tagArgs...)
	// Count all words due by end of today (midnight) so the user sees the
	// full day's workload and the "done" screen only appears once every
	// card due today has been reviewed.
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.first_seen_date IS NOT NULL
		   AND p.due_date < date('now', '+1 day')`+tagFilter+bucketSQL, dueArgs...).Scan(&dueToday)
	if err != nil {
		return
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.first_seen_date = date('now')`).Scan(&newToday)
	return
}

// CountUnseenZhWords returns the number of zh words that have never been presented
// (first_seen_date IS NULL), optionally filtered by tags.
func (s *Store) CountUnseenZhWords(ctx context.Context, tags []string) (int, error) {
	tagFilter := ""
	var args []any
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
		 WHERE w.language = 'zh' AND p.first_seen_date IS NULL`+tagFilter,
		args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unseen zh words: %w", err)
	}
	return count, nil
}

// CountLearningNewWords returns the number of zh words still in the "new" learning
// phase (learning_new_word = 1), optionally filtered by tags.
func (s *Store) CountLearningNewWords(ctx context.Context, tags []string) (int, error) {
	tagFilter := ""
	var args []any
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
		 WHERE w.language = 'zh' AND p.learning_new_word = 1 AND p.first_seen_date IS NOT NULL`+tagFilter,
		args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count learning new words: %w", err)
	}
	return count, nil
}

// GetWordCountByDueDate returns the number of zh words grouped by due date,
// covering overdue words (grouped as today), today, and the next 30 days.
// Unseen words (first_seen_date IS NULL) are excluded.
func (s *Store) GetWordCountByDueDate(ctx context.Context, tags []string) ([]models.DueDateCount, error) {
	tagFilter := ""
	var args []any
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
// For zh_to_en / zh_pinyin_to_en: looks for an EN word matching the answer, then
// returns the zh word it belongs to (if different from zhWordID).
// For en_to_zh: looks for a ZH word whose text matches the answer (if different from zhWordID).
// Returns (confusedWithID, true, nil) if a confusion is found, (0, false, nil) if not.
func (s *Store) LookupConfusion(ctx context.Context, zhWordID int64, answer, mode string) (int64, bool, error) {
	normalized := sm2.NormalizeAnswer(answer)
	if normalized == "" {
		return 0, false, nil
	}

	var confusedWithID int64
	var err error

	switch mode {
	case "zh_to_en", "zh_pinyin_to_en":
		// Find the zh word linked to an EN word whose text matches the answer.
		err = s.db.QueryRowContext(ctx, `
			SELECT t.zh_word_id FROM words w
			JOIN translations t ON t.en_word_id = w.id
			WHERE w.language = 'en' AND LOWER(TRIM(w.text)) = ?
			  AND t.zh_word_id != ?
			LIMIT 1`, normalized, zhWordID).Scan(&confusedWithID)
	case "en_to_zh":
		// Find a ZH word whose text matches the answer.
		err = s.db.QueryRowContext(ctx, `
			SELECT id FROM words
			WHERE language = 'zh' AND LOWER(TRIM(text)) = ?
			  AND id != ?
			LIMIT 1`, normalized, zhWordID).Scan(&confusedWithID)
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
func (s *Store) GetConfusionDetail(ctx context.Context, zhWordID, confusedWithID int64, mode string) (*models.ConfusionDetail, error) {
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
	var ferr error
	d.ZhEnTexts, ferr = s.getEnTextsForZhWord(ctx, zhWordID)
	if ferr != nil {
		return nil, ferr
	}
	d.ConfusedWithEnTexts, ferr = s.getEnTextsForZhWord(ctx, confusedWithID)
	if ferr != nil {
		return nil, ferr
	}
	return &d, nil
}

// GetConfusions returns all confusion pairs ordered by last_seen DESC, with full word details.
func (s *Store) GetConfusions(ctx context.Context) ([]models.ConfusionDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT cp.zh_word_id, wz.text, wz.pinyin,
		       cp.confused_with_id, wc.text, wc.pinyin,
		       cp.mode, cp.count, cp.last_seen
		FROM confusion_pairs cp
		JOIN words wz ON wz.id = cp.zh_word_id
		JOIN words wc ON wc.id = cp.confused_with_id
		ORDER BY cp.last_seen DESC`)
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

	for i := range items {
		items[i].ZhEnTexts, err = s.getEnTextsForZhWord(ctx, items[i].ZhWordID)
		if err != nil {
			return nil, err
		}
		items[i].ConfusedWithEnTexts, err = s.getEnTextsForZhWord(ctx, items[i].ConfusedWithID)
		if err != nil {
			return nil, err
		}
	}
	if items == nil {
		items = []models.ConfusionDetail{}
	}
	return items, nil
}

// parseDateTime parses SQLite datetime strings into time.Time.
// SQLite stores datetimes as "2006-01-02 15:04:05" or RFC3339; handle both.
func parseDateTime(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t
		}
	}
	return time.Time{}
}

// upsertWord inserts a word if it doesn't exist and returns its ID.
func upsertWord(ctx context.Context, tx *sql.Tx, text, lang string, pinyin *string) (int64, error) {
	text = strings.TrimSpace(text)
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO words (text, language, pinyin) VALUES (?, ?, ?)`,
		text, lang, pinyin); err != nil {
		return 0, fmt.Errorf("upsert word: %w", err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM words WHERE text = ? AND language = ?`, text, lang).Scan(&id); err != nil {
		return 0, fmt.Errorf("get word id: %w", err)
	}
	return id, nil
}

// initSM2 inserts a sm2_progress row for wordID if one doesn't exist yet.
func initSM2(ctx context.Context, tx *sql.Tx, wordID int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO sm2_progress (word_id) VALUES (?)`, wordID)
	if err != nil {
		return fmt.Errorf("init sm2: %w", err)
	}
	return nil
}

func (s *Store) getTagsForWord(ctx context.Context, wordID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tg.name FROM tags tg
		 JOIN word_tags wt ON wt.tag_id = tg.id
		 WHERE wt.word_id = ?
		 ORDER BY tg.name`, wordID)
	if err != nil {
		return nil, fmt.Errorf("get tags: %w", err)
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, rows.Err()
}

func getOrCreateTag(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO tags (name) VALUES (?)`, name); err != nil {
		return 0, fmt.Errorf("upsert tag: %w", err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM tags WHERE name = ?`, name).Scan(&id); err != nil {
		return 0, fmt.Errorf("get tag id: %w", err)
	}
	return id, nil
}

func setWordTags(ctx context.Context, tx *sql.Tx, wordID int64, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM word_tags WHERE word_id = ?`, wordID); err != nil {
		return fmt.Errorf("delete word tags: %w", err)
	}
	for _, name := range tags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		tagID, err := getOrCreateTag(ctx, tx, name)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO word_tags (word_id, tag_id) VALUES (?, ?)`,
			wordID, tagID); err != nil {
			return fmt.Errorf("link tag: %w", err)
		}
	}
	return nil
}

func (s *Store) cleanOrphanTags(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM tags WHERE id NOT IN (SELECT DISTINCT tag_id FROM word_tags)`)
	if err != nil {
		return fmt.Errorf("clean orphan tags: %w", err)
	}
	return nil
}

// GetAllTags returns all tag names ordered alphabetically.
func (s *Store) GetAllTags(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM tags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("get all tags: %w", err)
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, rows.Err()
}

// RecordDailyStat upserts today's daily_stats row after an answer submission.
// It returns the updated session streak (consecutive correct answers today).
func (s *Store) RecordDailyStat(ctx context.Context, correct bool) (int, error) {
	var wordsSeen int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.first_seen_date IS NOT NULL`).Scan(&wordsSeen); err != nil {
		return 0, fmt.Errorf("count words seen: %w", err)
	}

	var bNew, bStruggling, bLearning, bPracticing, bMastered int
	if err := s.db.QueryRowContext(ctx, `
		SELECT
		  COALESCE(SUM(CASE WHEN p.learning_new_word = 1 THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN p.learning_new_word = 0
		    AND (p.total_attempts < 3 OR CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts < 0.50)
		    THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN p.learning_new_word = 0
		    AND p.total_attempts >= 3
		    AND CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts >= 0.50
		    AND NOT (p.total_attempts >= 10 AND CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts >= 0.70)
		    THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN p.learning_new_word = 0
		    AND p.total_attempts >= 10
		    AND CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts >= 0.70
		    AND CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts < 0.85
		    THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN p.learning_new_word = 0
		    AND p.total_attempts >= 10
		    AND CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts >= 0.85
		    THEN 1 ELSE 0 END), 0)
		FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh' AND p.first_seen_date IS NOT NULL`).Scan(
		&bNew, &bStruggling, &bLearning, &bPracticing, &bMastered,
	); err != nil {
		return 0, fmt.Errorf("count buckets: %w", err)
	}

	mistakeInc := 0
	streakInit := 0
	if correct {
		streakInit = 1
	} else {
		mistakeInc = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO daily_stats (date, attempts, mistakes, words_seen,
			correct_streak, current_streak,
			bucket_new, bucket_struggling, bucket_learning, bucket_practicing, bucket_mastered)
		VALUES (date('now'), 1, ?, ?, ?, ?,
			?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			attempts         = attempts + 1,
			mistakes         = mistakes + ?,
			words_seen       = ?,
			current_streak   = CASE WHEN ? = 0 THEN current_streak + 1 ELSE 0 END,
			correct_streak   = CASE WHEN ? = 0 THEN MAX(correct_streak, current_streak + 1) ELSE correct_streak END,
			bucket_new       = ?,
			bucket_struggling = ?,
			bucket_learning  = ?,
			bucket_practicing = ?,
			bucket_mastered  = ?`,
		// INSERT values
		mistakeInc, wordsSeen, streakInit, streakInit,
		bNew, bStruggling, bLearning, bPracticing, bMastered,
		// UPDATE values
		mistakeInc, wordsSeen, mistakeInc, mistakeInc,
		bNew, bStruggling, bLearning, bPracticing, bMastered,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert daily stat: %w", err)
	}
	var streak int
	_ = s.db.QueryRowContext(ctx, `SELECT current_streak FROM daily_stats WHERE date = date('now')`).Scan(&streak)
	return streak, nil
}

// GetDailyStatsHistory returns all daily stats ordered by date ascending.
func (s *Store) GetDailyStatsHistory(ctx context.Context) ([]models.DailyStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, attempts, mistakes, words_seen, correct_streak,
		       bucket_new, bucket_struggling, bucket_learning, bucket_practicing, bucket_mastered
		FROM daily_stats ORDER BY date ASC`)
	if err != nil {
		return nil, fmt.Errorf("get daily stats: %w", err)
	}
	defer rows.Close()
	var stats []models.DailyStat
	for rows.Next() {
		var d models.DailyStat
		if err := rows.Scan(&d.Date, &d.Attempts, &d.Mistakes, &d.WordsSeen, &d.CorrectStreak,
			&d.BucketNew, &d.BucketStruggling, &d.BucketLearning, &d.BucketPracticing, &d.BucketMastered); err != nil {
			return nil, fmt.Errorf("scan daily stat: %w", err)
		}
		stats = append(stats, d)
	}
	if stats == nil {
		stats = []models.DailyStat{}
	}
	return stats, rows.Err()
}

// GetWordStats returns aggregate statistics for all words seen at least once.
func (s *Store) GetWordStats(ctx context.Context) (*models.WordStatsResponse, error) {
	// Fetch per-word stats for all seen zh words in a single query.
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, w.text, w.pinyin,
		       p.total_correct, p.total_attempts, p.streak_bonus, p.easiness,
		       p.learning_new_word
		FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh' AND p.first_seen_date IS NOT NULL
		ORDER BY p.total_attempts DESC`)
	if err != nil {
		return nil, fmt.Errorf("get word stats: %w", err)
	}
	defer rows.Close()

	type row struct {
		id          int64
		text        string
		pinyin      *string
		correct     int
		attempts    int
		streakBonus int
		easiness    float64
		accuracy    float64
		learning    bool
	}
	var all []row
	for rows.Next() {
		var r row
		var learning int
		if err := rows.Scan(&r.id, &r.text, &r.pinyin, &r.correct, &r.attempts, &r.streakBonus, &r.easiness, &learning); err != nil {
			return nil, fmt.Errorf("scan word stat: %w", err)
		}
		r.learning = learning == 1
		if r.attempts > 0 {
			r.accuracy = float64(r.correct+r.streakBonus) / float64(r.attempts) * 100
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	resp := &models.WordStatsResponse{
		TotalSeen:  len(all),
		AccBuckets: map[string]int{"new": 0, "0-49": 0, "50-69": 0, "70-84": 0, "85-100": 0},
		Hardest:    []models.WordStatDetail{},
		MostPract:  []models.WordStatDetail{},
	}

	if len(all) == 0 {
		return resp, nil
	}

	for _, r := range all {
		if r.learning {
			resp.AccBuckets["new"]++
		} else if r.attempts > 0 {
			switch {
			case r.attempts >= 10 && r.accuracy >= 85:
				resp.AccBuckets["85-100"]++
			case r.attempts >= 10 && r.accuracy >= 70:
				resp.AccBuckets["70-84"]++
			case r.attempts >= 3 && r.accuracy >= 50:
				resp.AccBuckets["50-69"]++
			default:
				resp.AccBuckets["0-49"]++
			}
		}
	}

	// Hardest words: lowest accuracy, min 3 attempts, up to 20
	type scored struct {
		idx int
		acc float64
	}
	var candidates []scored
	for i, r := range all {
		if r.attempts >= 3 {
			candidates = append(candidates, scored{i, r.accuracy})
		}
	}
	// Sort by accuracy ascending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].acc < candidates[i].acc {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	limit := 20
	if limit > len(candidates) {
		limit = len(candidates)
	}

	// Collect IDs for batch en-text loading
	detailIDs := make([]int64, 0, limit*2)
	for k := 0; k < limit; k++ {
		r := all[candidates[k].idx]
		resp.Hardest = append(resp.Hardest, models.WordStatDetail{
			WordID: r.id, ZhText: r.text, Pinyin: r.pinyin,
			Correct: r.correct, Attempts: r.attempts, StreakBonus: r.streakBonus,
			Accuracy: r.accuracy, Easiness: r.easiness,
		})
		detailIDs = append(detailIDs, r.id)
	}

	// Most practiced: already sorted by total_attempts DESC from query
	mpLimit := 20
	if mpLimit > len(all) {
		mpLimit = len(all)
	}
	for k := 0; k < mpLimit; k++ {
		r := all[k]
		resp.MostPract = append(resp.MostPract, models.WordStatDetail{
			WordID: r.id, ZhText: r.text, Pinyin: r.pinyin,
			Correct: r.correct, Attempts: r.attempts, StreakBonus: r.streakBonus,
			Accuracy: r.accuracy, Easiness: r.easiness,
		})
		detailIDs = append(detailIDs, r.id)
	}

	// Batch-load en texts for all detail words
	if len(detailIDs) > 0 {
		placeholders := make([]string, len(detailIDs))
		args := make([]any, len(detailIDs))
		for i, id := range detailIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		enRows, err := s.db.QueryContext(ctx,
			`SELECT t.zh_word_id, ew.text FROM words ew
			 JOIN translations t ON t.en_word_id = ew.id
			 WHERE t.zh_word_id IN (`+strings.Join(placeholders, ",")+`)
			 ORDER BY ew.text`, args...)
		if err != nil {
			return nil, fmt.Errorf("batch en texts for word stats: %w", err)
		}
		defer enRows.Close()
		enMap := map[int64][]string{}
		for enRows.Next() {
			var zhID int64
			var text string
			if err := enRows.Scan(&zhID, &text); err != nil {
				return nil, err
			}
			enMap[zhID] = append(enMap[zhID], text)
		}
		if err := enRows.Err(); err != nil {
			return nil, err
		}
		for i := range resp.Hardest {
			resp.Hardest[i].EnTexts = enMap[resp.Hardest[i].WordID]
			if resp.Hardest[i].EnTexts == nil {
				resp.Hardest[i].EnTexts = []string{}
			}
		}
		for i := range resp.MostPract {
			resp.MostPract[i].EnTexts = enMap[resp.MostPract[i].WordID]
			if resp.MostPract[i].EnTexts == nil {
				resp.MostPract[i].EnTexts = []string{}
			}
		}
	}

	return resp, nil
}

// GetTodaySessionInfo returns today's attempt and mistake counts from daily_stats,
// and the number of seen zh words whose due date is still in the future.
func (s *Store) GetTodaySessionInfo(ctx context.Context) (attempts, mistakes, availableToAdvance int, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(attempts, 0), COALESCE(mistakes, 0) FROM daily_stats WHERE date = date('now')`).
		Scan(&attempts, &mistakes)
	if err == sql.ErrNoRows {
		err = nil
		attempts, mistakes = 0, 0
	}
	if err != nil {
		err = fmt.Errorf("get today session info: %w", err)
		return
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh'
		  AND p.first_seen_date IS NOT NULL
		  AND p.due_date > CURRENT_TIMESTAMP`).Scan(&availableToAdvance)
	if err != nil {
		err = fmt.Errorf("count available to advance: %w", err)
	}
	return
}

// AdvanceDueDates pulls forward the due dates of seen zh words so that at least
// n words become due now. It finds the Nth earliest future due date among seen
// zh words, computes the delta to now, and subtracts it from all seen zh words'
// due dates. Returns the number of zh words now due after the operation.
func (s *Store) AdvanceDueDates(ctx context.Context, n int) (int, error) {
	var nthDueDateStr string
	err := s.db.QueryRowContext(ctx, `
		SELECT p.due_date FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh'
		  AND p.first_seen_date IS NOT NULL
		  AND p.due_date > CURRENT_TIMESTAMP
		ORDER BY p.due_date ASC
		LIMIT 1 OFFSET ?`, n-1).Scan(&nthDueDateStr)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find nth due date: %w", err)
	}

	nthDueDate := parseDateTime(nthDueDateStr)
	delta := time.Until(nthDueDate)
	if delta <= 0 {
		return 0, nil
	}
	modifier := fmt.Sprintf("-%d seconds", int64(delta.Seconds())+1)

	if _, err := s.db.ExecContext(ctx, `
		UPDATE sm2_progress SET due_date = datetime(due_date, ?)
		WHERE first_seen_date IS NOT NULL
		  AND word_id IN (SELECT id FROM words WHERE language = 'zh')`, modifier); err != nil {
		return 0, fmt.Errorf("advance due dates: %w", err)
	}

	var nowDue int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh'
		  AND p.first_seen_date IS NOT NULL
		  AND p.due_date <= CURRENT_TIMESTAMP`).Scan(&nowDue); err != nil {
		return 0, fmt.Errorf("count now due: %w", err)
	}
	return nowDue, nil
}

// idsOperator returns true for IDS operator runes (U+2FF0–U+2FFB).
func idsOperator(r rune) bool {
	return r >= 0x2FF0 && r <= 0x2FFB
}

// extractComponents returns the non-operator, non-placeholder runes from an IDS string.
func extractComponents(decomposition string) []rune {
	var out []rune
	for _, r := range decomposition {
		if !idsOperator(r) && r != '？' && r != '?' {
			out = append(out, r)
		}
	}
	return out
}

func (s *Store) GetHanziDecomposition(ctx context.Context, chars []rune) ([]models.HanziDecomposition, error) {
	if len(chars) == 0 {
		return nil, nil
	}

	// Build placeholders for the IN clause.
	ph := make([]string, len(chars))
	args := make([]any, len(chars))
	for i, c := range chars {
		ph[i] = "?"
		args[i] = string(c)
	}

	query := `SELECT character, definition, radical, decomposition, etymology
		FROM hanzi_decomposition WHERE character IN (` + strings.Join(ph, ",") + `)`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query hanzi decomposition: %w", err)
	}

	type rawRow struct {
		character     string
		definition    sql.NullString
		radical       sql.NullString
		decomposition sql.NullString
		etymology     sql.NullString
	}
	var rawRows []rawRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.character, &r.definition, &r.radical, &r.decomposition, &r.etymology); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hanzi decomposition: %w", err)
		}
		rawRows = append(rawRows, r)
	}
	rows.Close()

	// Collect all component characters for a second-level lookup.
	compSet := map[rune]bool{}
	for _, r := range rawRows {
		if r.decomposition.Valid {
			for _, c := range extractComponents(r.decomposition.String) {
				compSet[c] = true
			}
		}
	}
	// Remove characters we already have from the top-level query.
	for _, c := range chars {
		delete(compSet, c)
	}

	// Second query for component definitions.
	compMap := map[string]*models.HanziDecomposition{}
	if len(compSet) > 0 {
		ph2 := make([]string, 0, len(compSet))
		args2 := make([]any, 0, len(compSet))
		for c := range compSet {
			ph2 = append(ph2, "?")
			args2 = append(args2, string(c))
		}
		rows2, err := s.db.QueryContext(ctx, `SELECT character, definition, radical, decomposition, etymology
			FROM hanzi_decomposition WHERE character IN (`+strings.Join(ph2, ",")+`)`, args2...)
		if err != nil {
			return nil, fmt.Errorf("query component decomposition: %w", err)
		}
		for rows2.Next() {
			var r rawRow
			if err := rows2.Scan(&r.character, &r.definition, &r.radical, &r.decomposition, &r.etymology); err != nil {
				rows2.Close()
				return nil, fmt.Errorf("scan component decomposition: %w", err)
			}
			d := buildDecomposition(r.character, r.definition, r.radical, r.decomposition, r.etymology)
			compMap[r.character] = &d
		}
		rows2.Close()
	}

	// Also index top-level results so components can reference siblings.
	for _, r := range rawRows {
		if _, ok := compMap[r.character]; !ok {
			d := buildDecomposition(r.character, r.definition, r.radical, r.decomposition, r.etymology)
			compMap[r.character] = &d
		}
	}

	// Build result with components attached.
	results := make([]models.HanziDecomposition, 0, len(rawRows))
	for _, r := range rawRows {
		d := buildDecomposition(r.character, r.definition, r.radical, r.decomposition, r.etymology)
		if r.decomposition.Valid {
			for _, c := range extractComponents(r.decomposition.String) {
				if comp, ok := compMap[string(c)]; ok && string(c) != r.character {
					appendComponent(&d, *comp)
				}
			}
		}
		results = append(results, d)
	}

	// Return in the same order as the input chars.
	ordered := make([]models.HanziDecomposition, 0, len(chars))
	idx := map[string]models.HanziDecomposition{}
	for _, d := range results {
		idx[d.Character] = d
	}
	for _, c := range chars {
		if d, ok := idx[string(c)]; ok {
			ordered = append(ordered, d)
		}
	}
	return ordered, nil
}

// GetHanziDecompositionString returns the raw decomposition string for a single character,
// or an empty string if none exists.
func (s *Store) GetHanziDecompositionString(ctx context.Context, char string) (string, error) {
	var decomp sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT decomposition FROM hanzi_decomposition WHERE character = ?`, char,
	).Scan(&decomp)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get hanzi decomposition string: %w", err)
	}
	if decomp.Valid {
		return decomp.String, nil
	}
	return "", nil
}

// UpsertHanziDecomposition inserts or updates the decomposition string for a character.
func (s *Store) UpsertHanziDecomposition(ctx context.Context, char, decomp string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, decomposition)
		 VALUES (?, ?)
		 ON CONFLICT(character) DO UPDATE SET decomposition = excluded.decomposition`,
		char, decomp)
	if err != nil {
		return fmt.Errorf("upsert hanzi decomposition: %w", err)
	}
	return nil
}

func appendComponent(parent *models.HanziDecomposition, comp models.HanziDecomposition) {
	comp.Components = nil
	comp.Decomposition = ""
	parent.Components = append(parent.Components, comp)
}

func buildDecomposition(character string, definition, radical, decomposition, etymology sql.NullString) models.HanziDecomposition {
	d := models.HanziDecomposition{Character: character}
	if definition.Valid {
		d.Definition = definition.String
	}
	if radical.Valid {
		d.Radical = radical.String
	}
	if decomposition.Valid {
		d.Decomposition = decomposition.String
	}
	if etymology.Valid && etymology.String != "" {
		var ety models.HanziEtymology
		if err := json.Unmarshal([]byte(etymology.String), &ety); err == nil && ety.Type != "" {
			d.Etymology = &ety
		}
	}
	return d
}

// ── Hanzi Movie Method (HMM) ────────────────────────────────────────────

func (s *Store) GetHMMActors(ctx context.Context) ([]models.HMMActor, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT initial, category, actor_name, hint FROM hmm_actors
		 ORDER BY CASE category
		   WHEN 'male' THEN 1 WHEN 'female' THEN 2
		   WHEN 'fictional' THEN 3 WHEN 'wildcard' THEN 4 END, initial`)
	if err != nil {
		return nil, fmt.Errorf("get hmm actors: %w", err)
	}
	var actors []models.HMMActor
	for rows.Next() {
		var a models.HMMActor
		if err := rows.Scan(&a.Initial, &a.Category, &a.ActorName, &a.Hint); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hmm actor: %w", err)
		}
		actors = append(actors, a)
	}
	rows.Close()
	return actors, rows.Err()
}

func (s *Store) UpdateHMMActor(ctx context.Context, initial, actorName string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE hmm_actors SET actor_name = ? WHERE initial = ?`, actorName, initial)
	if err != nil {
		return fmt.Errorf("update hmm actor: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("hmm actor %q not found", initial)
	}
	return nil
}

func (s *Store) GetHMMLocations(ctx context.Context) ([]models.HMMLocation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT final_key, location_name FROM hmm_locations ORDER BY final_key`)
	if err != nil {
		return nil, fmt.Errorf("get hmm locations: %w", err)
	}
	var locs []models.HMMLocation
	for rows.Next() {
		var l models.HMMLocation
		if err := rows.Scan(&l.FinalKey, &l.LocationName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hmm location: %w", err)
		}
		locs = append(locs, l)
	}
	rows.Close()
	return locs, rows.Err()
}

func (s *Store) UpdateHMMLocation(ctx context.Context, finalKey, locationName string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE hmm_locations SET location_name = ? WHERE final_key = ?`, locationName, finalKey)
	if err != nil {
		return fmt.Errorf("update hmm location: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("hmm location %q not found", finalKey)
	}
	return nil
}

func (s *Store) GetHMMToneRooms(ctx context.Context) ([]models.HMMToneRoom, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tone, room_name FROM hmm_tone_rooms ORDER BY tone`)
	if err != nil {
		return nil, fmt.Errorf("get hmm tone rooms: %w", err)
	}
	var rooms []models.HMMToneRoom
	for rows.Next() {
		var tr models.HMMToneRoom
		if err := rows.Scan(&tr.Tone, &tr.RoomName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hmm tone room: %w", err)
		}
		rooms = append(rooms, tr)
	}
	rows.Close()
	return rooms, rows.Err()
}

func (s *Store) UpdateHMMToneRoom(ctx context.Context, tone int, roomName string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE hmm_tone_rooms SET room_name = ? WHERE tone = ?`, roomName, tone)
	if err != nil {
		return fmt.Errorf("update hmm tone room: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("hmm tone room %d not found", tone)
	}
	return nil
}

func (s *Store) GetHMMProps(ctx context.Context) ([]models.HMMProp, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT radical, prop_name FROM hmm_props ORDER BY radical`)
	if err != nil {
		return nil, fmt.Errorf("get hmm props: %w", err)
	}
	var props []models.HMMProp
	for rows.Next() {
		var p models.HMMProp
		if err := rows.Scan(&p.Radical, &p.PropName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hmm prop: %w", err)
		}
		props = append(props, p)
	}
	rows.Close()
	return props, rows.Err()
}

func (s *Store) UpsertHMMProp(ctx context.Context, radical, propName string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hmm_props (radical, prop_name) VALUES (?, ?)
		 ON CONFLICT(radical) DO UPDATE SET prop_name = excluded.prop_name`,
		radical, propName)
	if err != nil {
		return fmt.Errorf("upsert hmm prop: %w", err)
	}
	return nil
}

func (s *Store) DeleteHMMProp(ctx context.Context, radical string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM hmm_props WHERE radical = ?`, radical)
	if err != nil {
		return fmt.Errorf("delete hmm prop: %w", err)
	}
	return nil
}

func (s *Store) GetHMMScene(ctx context.Context, wordID int64) (*models.HMMScene, error) {
	var sc models.HMMScene
	err := s.db.QueryRowContext(ctx,
		`SELECT word_id, scene_text FROM hmm_scenes WHERE word_id = ?`, wordID).
		Scan(&sc.WordID, &sc.SceneText)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hmm scene: %w", err)
	}
	return &sc, nil
}

func (s *Store) UpsertHMMScene(ctx context.Context, wordID int64, sceneText string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hmm_scenes (word_id, scene_text) VALUES (?, ?)
		 ON CONFLICT(word_id) DO UPDATE SET scene_text = excluded.scene_text`,
		wordID, sceneText)
	if err != nil {
		return fmt.Errorf("upsert hmm scene: %w", err)
	}
	return nil
}

func (s *Store) DeleteHMMScene(ctx context.Context, wordID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM hmm_scenes WHERE word_id = ?`, wordID)
	if err != nil {
		return fmt.Errorf("delete hmm scene: %w", err)
	}
	return nil
}

func (s *Store) GetHMMSceneText(ctx context.Context, wordID int64) (string, error) {
	var text string
	err := s.db.QueryRowContext(ctx,
		`SELECT scene_text FROM hmm_scenes WHERE word_id = ?`, wordID).Scan(&text)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get hmm scene text: %w", err)
	}
	return text, nil
}

func (s *Store) GetHMMActorByInitial(ctx context.Context, initial string) (*models.HMMActor, error) {
	var a models.HMMActor
	err := s.db.QueryRowContext(ctx,
		`SELECT initial, category, actor_name, hint FROM hmm_actors WHERE initial = ?`, initial).
		Scan(&a.Initial, &a.Category, &a.ActorName, &a.Hint)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hmm actor by initial: %w", err)
	}
	return &a, nil
}

func (s *Store) GetHMMLocationByFinal(ctx context.Context, finalKey string) (*models.HMMLocation, error) {
	var l models.HMMLocation
	err := s.db.QueryRowContext(ctx,
		`SELECT final_key, location_name FROM hmm_locations WHERE final_key = ?`, finalKey).
		Scan(&l.FinalKey, &l.LocationName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hmm location by final: %w", err)
	}
	return &l, nil
}

func (s *Store) GetHMMToneRoom(ctx context.Context, tone int) (*models.HMMToneRoom, error) {
	var tr models.HMMToneRoom
	err := s.db.QueryRowContext(ctx,
		`SELECT tone, room_name FROM hmm_tone_rooms WHERE tone = ?`, tone).
		Scan(&tr.Tone, &tr.RoomName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hmm tone room: %w", err)
	}
	return &tr, nil
}

// getTranslationsByZhTexts returns the first translation in the given language for each
// zh_text in the supplied slice, keyed by zh_text. Both EN and DE translations share
// the translations table; language is distinguished via words.language.
func (s *Store) getTranslationsByZhTexts(ctx context.Context, zhTexts []string, lang string) (map[string]string, error) {
	if len(zhTexts) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(zhTexts))
	args := make([]any, len(zhTexts)+1)
	args[0] = lang
	for i, t := range zhTexts {
		placeholders[i] = "?"
		args[i+1] = t
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.text, MIN(e.text)
		 FROM words w
		 JOIN translations t ON t.zh_word_id = w.id
		 JOIN words e ON e.id = t.en_word_id AND e.language = ?
		 WHERE w.language = 'zh' AND w.text IN (`+strings.Join(placeholders, ",")+`)
		 GROUP BY w.text`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("get %s translations by zh texts: %w", lang, err)
	}
	result := make(map[string]string)
	for rows.Next() {
		var zh, trans string
		if err := rows.Scan(&zh, &trans); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan %s translation: %w", lang, err)
		}
		result[zh] = trans
	}
	rows.Close()
	return result, rows.Err()
}

// GetEnTranslationsByZhTexts returns the first English translation for each
// zh_text in the supplied slice, keyed by zh_text.
func (s *Store) GetEnTranslationsByZhTexts(ctx context.Context, zhTexts []string) (map[string]string, error) {
	return s.getTranslationsByZhTexts(ctx, zhTexts, "en")
}

// GetDeTranslationsByZhTexts returns the first German translation for each
// zh_text in the supplied slice, keyed by zh_text.
func (s *Store) GetDeTranslationsByZhTexts(ctx context.Context, zhTexts []string) (map[string]string, error) {
	return s.getTranslationsByZhTexts(ctx, zhTexts, "de")
}

// StoreTranslationForZhChar stores an EN or DE translation for a Chinese character.
// Both languages use the translations table; words.language distinguishes them.
// If the zh character does not yet exist in the words table it is created with the
// supplied pinyin (which may be empty). No SM-2 progress row is initialised because
// the character is stored only as a reference, not as a quiz word.
func (s *Store) StoreTranslationForZhChar(ctx context.Context, zhText, pinyin, transText, lang string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var zhID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM words WHERE text = ? AND language = 'zh'`, zhText,
	).Scan(&zhID); err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("find zh word: %w", err)
		}
		// zh word doesn't exist yet — create it so the translation can be linked.
		var py *string
		if pinyin != "" {
			py = &pinyin
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO words (text, language, pinyin) VALUES (?, 'zh', ?)`, zhText, py,
		); err != nil {
			return fmt.Errorf("insert zh word: %w", err)
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM words WHERE text = ? AND language = 'zh'`, zhText,
		).Scan(&zhID); err != nil {
			return fmt.Errorf("get new zh word id: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO words (text, language) VALUES (?, ?)`, transText, lang,
	); err != nil {
		return fmt.Errorf("upsert %s word: %w", lang, err)
	}
	var transID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM words WHERE text = ? AND language = ?`, transText, lang,
	).Scan(&transID); err != nil {
		return fmt.Errorf("get %s word id: %w", lang, err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`, transID, zhID,
	); err != nil {
		return fmt.Errorf("link %s translation: %w", lang, err)
	}

	return tx.Commit()
}

func (s *Store) GetHMMPropsByRadicals(ctx context.Context, radicals []string) ([]models.HMMProp, error) {
	if len(radicals) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(radicals))
	args := make([]any, len(radicals))
	for i, r := range radicals {
		placeholders[i] = "?"
		args[i] = r
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT radical, prop_name FROM hmm_props WHERE radical IN (`+strings.Join(placeholders, ",")+`)`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("get hmm props by radicals: %w", err)
	}
	var props []models.HMMProp
	for rows.Next() {
		var p models.HMMProp
		if err := rows.Scan(&p.Radical, &p.PropName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hmm prop: %w", err)
		}
		props = append(props, p)
	}
	rows.Close()
	return props, rows.Err()
}

func (s *Store) SaveHMMSceneWithLibrary(ctx context.Context, wordID int64, initial, finalKey string, tone int, req models.HMMSaveSceneRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO hmm_scenes (word_id, scene_text) VALUES (?, ?)
		 ON CONFLICT(word_id) DO UPDATE SET scene_text = excluded.scene_text`,
		wordID, req.SceneText); err != nil {
		return fmt.Errorf("upsert scene: %w", err)
	}

	if req.ActorName != "" && initial != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE hmm_actors SET actor_name = ? WHERE initial = ?`,
			req.ActorName, initial); err != nil {
			return fmt.Errorf("update actor: %w", err)
		}
	}
	if req.LocationName != "" && finalKey != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE hmm_locations SET location_name = ? WHERE final_key = ?`,
			req.LocationName, finalKey); err != nil {
			return fmt.Errorf("update location: %w", err)
		}
	}
	if req.RoomName != "" && tone >= 1 && tone <= 5 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE hmm_tone_rooms SET room_name = ? WHERE tone = ?`,
			req.RoomName, tone); err != nil {
			return fmt.Errorf("update tone room: %w", err)
		}
	}
	for _, p := range req.Props {
		if p.Radical == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO hmm_props (radical, prop_name) VALUES (?, ?)
			 ON CONFLICT(radical) DO UPDATE SET prop_name = excluded.prop_name`,
			p.Radical, p.PropName); err != nil {
			return fmt.Errorf("upsert prop %s: %w", p.Radical, err)
		}
	}

	return tx.Commit()
}
