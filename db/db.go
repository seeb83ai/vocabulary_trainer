package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"
	"vocabulary_trainer/models"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

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
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("run schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// GetWords returns a paginated list of vocabulary entries (zh words with their en translations).
func (s *Store) GetWords(ctx context.Context, q string, page, perPage int) ([]models.WordDetail, int, error) {
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

	// Count total
	var total int
	countQuery := `
		SELECT COUNT(DISTINCT w.id) FROM words w
		WHERE w.language = 'zh'
		  AND (? = '' OR w.text LIKE '%' || ? || '%'
		       OR w.pinyin LIKE '%' || ? || '%'
		       OR EXISTS (
		           SELECT 1 FROM words ew
		           JOIN translations t ON t.en_word_id = ew.id AND t.zh_word_id = w.id
		           WHERE ew.text LIKE '%' || ? || '%'
		       ))`
	if err := s.db.QueryRowContext(ctx, countQuery, q, q, q, q).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count words: %w", err)
	}

	listQuery := `
		SELECT w.id, w.text, w.pinyin, w.created_at FROM words w
		WHERE w.language = 'zh'
		  AND (? = '' OR w.text LIKE '%' || ? || '%'
		       OR w.pinyin LIKE '%' || ? || '%'
		       OR EXISTS (
		           SELECT 1 FROM words ew
		           JOIN translations t ON t.en_word_id = ew.id AND t.zh_word_id = w.id
		           WHERE ew.text LIKE '%' || ? || '%'
		       ))
		ORDER BY w.created_at DESC
		LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, listQuery, q, q, q, q, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list words: %w", err)
	}
	defer rows.Close()

	// Collect rows first, then close before issuing per-word queries.
	// Required because db.SetMaxOpenConns(1) — nested queries on the same
	// connection while rows is open causes a deadlock.
	var words []models.WordDetail
	for rows.Next() {
		var wd models.WordDetail
		var createdAt string
		if err := rows.Scan(&wd.ID, &wd.ZhText, &wd.Pinyin, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("scan word: %w", err)
		}
		wd.CreatedAt = parseDateTime(createdAt)
		words = append(words, wd)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close() // release connection before issuing per-word queries

	for i := range words {
		enTexts, err := s.getEnTextsForZhWord(ctx, words[i].ID)
		if err != nil {
			return nil, 0, err
		}
		words[i].EnTexts = enTexts
	}
	if words == nil {
		words = []models.WordDetail{}
	}
	return words, total, nil
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
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, text, pinyin, created_at FROM words WHERE id = ? AND language = 'zh'`, id).
		Scan(&wd.ID, &wd.ZhText, &wd.Pinyin, &createdAt)
	wd.CreatedAt = parseDateTime(createdAt)
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
		`UPDATE words SET text = ?, pinyin = ? WHERE id = ? AND language = 'zh'`,
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
	return nil
}

// GetNextCard returns the most-overdue card. Falls back to nearest upcoming if none are due.
// Returns (word, progress, nil) or (nil, nil, nil) if no words exist.
func (s *Store) GetNextCard(ctx context.Context) (*models.Word, *models.SM2Progress, error) {
	query := `
		SELECT w.id, w.text, w.language, w.pinyin, w.created_at,
		       p.repetitions, p.easiness, p.interval_days, p.due_date,
		       p.total_correct, p.total_attempts
		FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		%s
		ORDER BY p.due_date ASC
		LIMIT 1`

	tryQuery := func(where string) (*models.Word, *models.SM2Progress, error) {
		row := s.db.QueryRowContext(ctx, fmt.Sprintf(query, where))
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

	w, p, err := tryQuery("WHERE p.due_date <= CURRENT_TIMESTAMP")
	if err != nil || w != nil {
		return w, p, err
	}
	// No overdue cards — pick the nearest upcoming one
	return tryQuery("")
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
		p.DueDate.UTC().Format(time.RFC3339),
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

// GetStats returns due-today count and total word count (zh words only).
func (s *Store) GetStats(ctx context.Context) (dueToday, total int, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM words WHERE language = 'zh'`).Scan(&total)
	if err != nil {
		return
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.due_date <= CURRENT_TIMESTAMP`).Scan(&dueToday)
	return
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
