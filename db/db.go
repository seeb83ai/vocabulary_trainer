package db

import (
	"context"
	"database/sql"
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
	"repetitions": "COALESCE(p.repetitions, 0)|CAST(COALESCE(p.total_correct, 0) AS REAL) / NULLIF(COALESCE(p.total_attempts, 0), 0)",
	"due_date":    "COALESCE(p.due_date, CURRENT_TIMESTAMP)",
}

// GetWords returns a paginated list of vocabulary entries (zh words with their en translations).
// If reviewOnly is true, only words with needs_review = 1 are returned.
func (s *Store) GetWords(ctx context.Context, q string, page, perPage int, sortBy, sortDir string, tags []string, reviewOnly bool) ([]models.WordDetail, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
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

	// Single query: COUNT(*) OVER() returns the total alongside each row,
	// eliminating the separate count query.
	listQuery := `
		SELECT w.id, w.text, w.pinyin, w.created_at,
		       COALESCE(p.repetitions, 0), COALESCE(p.easiness, 2.5),
		       COALESCE(p.interval_days, 1),
		       COALESCE(p.total_correct, 0), COALESCE(p.total_attempts, 0),
		       COALESCE(p.due_date, CURRENT_TIMESTAMP),
		       COALESCE(w.needs_review, 0),
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
		       ))` + tagFilter + reviewFilter + `
		ORDER BY ` + orderClause + `
		LIMIT ? OFFSET ?`
	listArgs := []any{q, q, q, q}
	listArgs = append(listArgs, tagArgs...)
	listArgs = append(listArgs, perPage, offset)
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
		var needsReview int
		if err := rows.Scan(
			&wd.ID, &wd.ZhText, &wd.Pinyin, &createdAt,
			&wd.Repetitions, &wd.Easiness, &wd.IntervalDays,
			&wd.TotalCorrect, &wd.TotalAttempts, &dueDate,
			&needsReview,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan word: %w", err)
		}
		wd.NeedsReview = needsReview == 1
		wd.CreatedAt = parseDateTime(createdAt)
		wd.DueDate = parseDateTime(dueDate)
		words = append(words, wd)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()

	// Batch-load English texts and tags for all page results in two queries
	// instead of 2×N per-word queries.
	if len(words) > 0 {
		ids := make([]int64, len(words))
		idIndex := make(map[int64]int, len(words))
		for i, w := range words {
			ids[i] = w.ID
			idIndex[w.ID] = i
		}
		if err := s.batchLoadEnTexts(ctx, words, ids, idIndex); err != nil {
			return nil, 0, err
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

func (s *Store) batchLoadEnTexts(ctx context.Context, words []models.WordDetail, ids []int64, idIndex map[int64]int) error {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.zh_word_id, w.text FROM words w
		 JOIN translations t ON t.en_word_id = w.id
		 WHERE t.zh_word_id IN (`+strings.Join(placeholders, ",")+`)
		 ORDER BY w.text`, args...)
	if err != nil {
		return fmt.Errorf("batch en texts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var zhID int64
		var text string
		if err := rows.Scan(&zhID, &text); err != nil {
			return err
		}
		idx := idIndex[zhID]
		words[idx].EnTexts = append(words[idx].EnTexts, text)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range words {
		if words[i].EnTexts == nil {
			words[i].EnTexts = []string{}
		}
	}
	return nil
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

func (s *Store) getEnTextsForZhWord(ctx context.Context, zhID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.text FROM words w
		 JOIN translations t ON t.en_word_id = w.id
		 WHERE t.zh_word_id = ?
		 ORDER BY w.text`, zhID)
	if err != nil {
		return nil, fmt.Errorf("get en texts: %w", err)
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		texts = append(texts, t)
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
		        COALESCE(p.due_date, CURRENT_TIMESTAMP),
		        COALESCE(w.needs_review, 0)
		 FROM words w
		 LEFT JOIN sm2_progress p ON p.word_id = w.id
		 WHERE w.id = ? AND w.language = 'zh'`, id).
		Scan(&wd.ID, &wd.ZhText, &wd.Pinyin, &createdAt,
			&wd.Repetitions, &wd.Easiness, &wd.IntervalDays,
			&wd.TotalCorrect, &wd.TotalAttempts, &dueDate, &needsReview)
	wd.CreatedAt = parseDateTime(createdAt)
	wd.DueDate = parseDateTime(dueDate)
	wd.NeedsReview = needsReview == 1
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get word by id: %w", err)
	}
	enTexts, err := s.getEnTextsForZhWord(ctx, id)
	if err != nil {
		return nil, err
	}
	wd.EnTexts = enTexts
	wd.Tags, err = s.getTagsForWord(ctx, id)
	if err != nil {
		return nil, err
	}
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
			return 0, fmt.Errorf("link translation: %w", err)
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
			return fmt.Errorf("link translation: %w", err)
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

// GetNextCard returns the most-overdue card. Falls back to nearest upcoming if none are due.
// Returns (word, progress, nil) or (nil, nil, nil) if no words exist.
// maxNew caps how many new words (first_seen_date IS NULL) can be introduced today; once
// the count for today reaches maxNew, only already-seen cards are returned.
func (s *Store) GetNextCard(ctx context.Context, tags []string, maxNew int) (*models.Word, *models.SM2Progress, error) {
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

	// Count new words already introduced today (global, not per-tag).
	var newToday int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.first_seen_date = date('now')`).Scan(&newToday); err != nil {
		return nil, nil, fmt.Errorf("count new today: %w", err)
	}

	// When the daily cap is reached, skip words that have never been presented.
	newWordFilter := ""
	if newToday >= maxNew {
		newWordFilter = " AND p.first_seen_date IS NOT NULL"
	}

	// Only quiz on zh words — they are the canonical unit; en words are just
	// answer targets and should never appear as quiz prompts on their own.
	query := `
		SELECT w.id, w.text, w.language, w.pinyin, w.created_at,
		       p.repetitions, p.easiness, p.interval_days, p.due_date,
		       p.total_correct, p.total_attempts
		FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		WHERE w.language = 'zh'` + tagFilter + newWordFilter + ` %s
		ORDER BY p.due_date ASC
		LIMIT 1`

	tryQuery := func(extra string) (*models.Word, *models.SM2Progress, error) {
		args := append([]any{}, tagArgs...)
		row := s.db.QueryRowContext(ctx, fmt.Sprintf(query, extra), args...)
		var w models.Word
		var p models.SM2Progress
		var createdAt, dueDate string
		err := row.Scan(
			&w.ID, &w.Text, &w.Language, &w.Pinyin, &createdAt,
			&p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDate,
			&p.TotalCorrect, &p.TotalAttempts,
		)
		w.CreatedAt = parseDateTime(createdAt)
		p.DueDate = parseDateTime(dueDate)
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get next card: %w", err)
		}
		p.WordID = w.ID
		return &w, &p, nil
	}

	// stamp sets first_seen_date the first time a card is presented.
	stamp := func(w *models.Word) {
		if w != nil {
			_, _ = s.db.ExecContext(ctx,
				`UPDATE sm2_progress SET first_seen_date = date('now') WHERE word_id = ? AND first_seen_date IS NULL`,
				w.ID)
		}
	}

	w, p, err := tryQuery("AND p.due_date <= CURRENT_TIMESTAMP")
	if err != nil || w != nil {
		stamp(w)
		return w, p, err
	}
	// No overdue cards — prefer cards outside the wrong-retry window so a
	// recently failed card is not immediately repeated.
	w, p, err = tryQuery(fmt.Sprintf("AND p.due_date > datetime('now', '+%d seconds')", int(sm2.WrongRetryDelay.Seconds())))
	if err != nil || w != nil {
		stamp(w)
		return w, p, err
	}
	// All remaining cards are within the retry window; return the soonest one.
	w, p, err = tryQuery("")
	stamp(w)
	return w, p, err
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
	err := s.db.QueryRowContext(ctx,
		`SELECT word_id, repetitions, easiness, interval_days, due_date, total_correct, total_attempts
		 FROM sm2_progress WHERE word_id = ?`, wordID).
		Scan(&p.WordID, &p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDate,
			&p.TotalCorrect, &p.TotalAttempts)
	p.DueDate = parseDateTime(dueDate)
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
	res, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress
		 SET repetitions = ?, easiness = ?, interval_days = ?, due_date = ?,
		     total_correct = ?, total_attempts = ?
		 WHERE word_id = ?`,
		p.Repetitions, p.Easiness, p.IntervalDays,
		p.DueDate.UTC().Format("2006-01-02 15:04:05"),
		p.TotalCorrect, p.TotalAttempts, p.WordID)
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
func (s *Store) GetStats(ctx context.Context, tags []string) (dueToday, total, newToday int, err error) {
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

	totalArgs := append([]any{}, tagArgs...)
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM words w WHERE w.language = 'zh'`+tagFilter, totalArgs...).Scan(&total)
	if err != nil {
		return
	}
	dueArgs := append([]any{}, tagArgs...)
	// Include words in the wrong-retry window (due within the next
	// WrongRetryDelay*3 seconds) so that due_today stays > 0 while
	// recently-failed cards are waiting for their short re-test delay.
	retryWindowSec := int((sm2.WrongRetryDelay * 3).Seconds())
	err = s.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.first_seen_date IS NOT NULL
		   AND p.due_date <= datetime('now', '+%d seconds')`, retryWindowSec)+tagFilter, dueArgs...).Scan(&dueToday)
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
func (s *Store) RecordDailyStat(ctx context.Context, correct bool) error {
	var wordsKnown, newWords, wordsSeen int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.first_seen_date IS NOT NULL AND p.total_correct >= 5`).Scan(&wordsKnown); err != nil {
		return fmt.Errorf("count words known: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.first_seen_date = date('now')`).Scan(&newWords); err != nil {
		return fmt.Errorf("count new words: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.first_seen_date IS NOT NULL`).Scan(&wordsSeen); err != nil {
		return fmt.Errorf("count words seen: %w", err)
	}

	mistakeInc := 0
	streakInit := 0
	if correct {
		streakInit = 1
	} else {
		mistakeInc = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO daily_stats (date, attempts, mistakes, words_known, new_words, words_seen, correct_streak, current_streak)
		VALUES (date('now'), 1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			attempts       = attempts + 1,
			mistakes       = mistakes + ?,
			words_known    = ?,
			new_words      = ?,
			words_seen     = ?,
			current_streak = CASE WHEN ? = 0 THEN current_streak + 1 ELSE 0 END,
			correct_streak = CASE WHEN ? = 0 THEN MAX(correct_streak, current_streak + 1) ELSE correct_streak END`,
		mistakeInc, wordsKnown, newWords, wordsSeen, streakInit, streakInit,
		mistakeInc, wordsKnown, newWords, wordsSeen, mistakeInc, mistakeInc,
	)
	if err != nil {
		return fmt.Errorf("upsert daily stat: %w", err)
	}
	return nil
}

// GetDailyStatsHistory returns all daily stats ordered by date ascending.
func (s *Store) GetDailyStatsHistory(ctx context.Context) ([]models.DailyStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, attempts, mistakes, words_known, new_words, words_seen, correct_streak
		FROM daily_stats ORDER BY date ASC`)
	if err != nil {
		return nil, fmt.Errorf("get daily stats: %w", err)
	}
	defer rows.Close()
	var stats []models.DailyStat
	for rows.Next() {
		var d models.DailyStat
		if err := rows.Scan(&d.Date, &d.Attempts, &d.Mistakes, &d.WordsKnown, &d.NewWords, &d.WordsSeen, &d.CorrectStreak); err != nil {
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
		       p.total_correct, p.total_attempts, p.easiness
		FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh' AND p.first_seen_date IS NOT NULL
		ORDER BY p.total_attempts DESC`)
	if err != nil {
		return nil, fmt.Errorf("get word stats: %w", err)
	}
	defer rows.Close()

	type row struct {
		id       int64
		text     string
		pinyin   *string
		correct  int
		attempts int
		easiness float64
		accuracy float64
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.text, &r.pinyin, &r.correct, &r.attempts, &r.easiness); err != nil {
			return nil, fmt.Errorf("scan word stat: %w", err)
		}
		if r.attempts > 0 {
			r.accuracy = float64(r.correct) / float64(r.attempts) * 100
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	resp := &models.WordStatsResponse{
		TotalSeen:  len(all),
		Milestones: map[string]int{"1+": 0, "3+": 0, "5+": 0, "10+": 0},
		AccBuckets: map[string]int{"0-49": 0, "50-79": 0, "80-99": 0, "100": 0},
		Hardest:    []models.WordStatDetail{},
		MostPract:  []models.WordStatDetail{},
	}

	if len(all) == 0 {
		return resp, nil
	}

	corrects := make([]float64, len(all))
	attempts := make([]float64, len(all))
	accuracies := make([]float64, 0, len(all))
	easinesses := make([]float64, len(all))

	for i, r := range all {
		corrects[i] = float64(r.correct)
		attempts[i] = float64(r.attempts)
		easinesses[i] = r.easiness

		if r.attempts > 0 {
			accuracies = append(accuracies, r.accuracy)
		}

		if r.correct >= 1 {
			resp.Milestones["1+"]++
		}
		if r.correct >= 3 {
			resp.Milestones["3+"]++
		}
		if r.correct >= 5 {
			resp.Milestones["5+"]++
		}
		if r.correct >= 10 {
			resp.Milestones["10+"]++
		}

		if r.attempts > 0 {
			switch {
			case r.accuracy >= 100:
				resp.AccBuckets["100"]++
			case r.accuracy >= 80:
				resp.AccBuckets["80-99"]++
			case r.accuracy >= 50:
				resp.AccBuckets["50-79"]++
			default:
				resp.AccBuckets["0-49"]++
			}
		}
	}

	resp.Aggregates.Correct = calcDistStats(corrects)
	resp.Aggregates.Attempts = calcDistStats(attempts)
	if len(accuracies) > 0 {
		resp.Aggregates.Accuracy = calcDistStats(accuracies)
	}
	resp.Aggregates.Easiness = calcDistStats(easinesses)

	// Hardest words: lowest accuracy, min 3 attempts, up to 5
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
	limit := 5
	if limit > len(candidates) {
		limit = len(candidates)
	}

	// Collect IDs for batch en-text loading
	detailIDs := make([]int64, 0, limit*2)
	for k := 0; k < limit; k++ {
		r := all[candidates[k].idx]
		resp.Hardest = append(resp.Hardest, models.WordStatDetail{
			WordID: r.id, ZhText: r.text, Pinyin: r.pinyin,
			Correct: r.correct, Attempts: r.attempts,
			Accuracy: r.accuracy, Easiness: r.easiness,
		})
		detailIDs = append(detailIDs, r.id)
	}

	// Most practiced: already sorted by total_attempts DESC from query
	mpLimit := 5
	if mpLimit > len(all) {
		mpLimit = len(all)
	}
	for k := 0; k < mpLimit; k++ {
		r := all[k]
		resp.MostPract = append(resp.MostPract, models.WordStatDetail{
			WordID: r.id, ZhText: r.text, Pinyin: r.pinyin,
			Correct: r.correct, Attempts: r.attempts,
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

func calcDistStats(vals []float64) models.DistStats {
	if len(vals) == 0 {
		return models.DistStats{}
	}
	// Copy and sort for percentile calculations
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	var sum float64
	for _, v := range sorted {
		sum += v
	}
	n := len(sorted)
	avg := sum / float64(n)

	var median float64
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	} else {
		median = sorted[n/2]
	}

	p95Idx := int(float64(n-1) * 0.95)
	p95 := sorted[p95Idx]

	// Round to 1 decimal
	round := func(v float64) float64 {
		return float64(int(v*10+0.5)) / 10
	}
	return models.DistStats{
		Avg:        round(avg),
		Median:     round(median),
		Percentile: round(p95),
	}
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
