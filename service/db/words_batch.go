package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"vocabulary_trainer/models"
)

type wordKey struct{ text, lang string }

// batchWriter writes many words inside one transaction. It prepares each
// statement once and remembers the IDs of words and tags it has already
// resolved, so a translation shared by many zh words (or a tag shared by the
// whole import) costs one lookup instead of one per word.
type batchWriter struct {
	tx      *sql.Tx
	userID  int64
	stmts   []*sql.Stmt
	wordIDs map[wordKey]int64
	tagIDs  map[string]int64

	insWord, selWord, insSM2, insLink *sql.Stmt
	selRank, delWordTags, insWordTag  *sql.Stmt
	selTag, insTag                    *sql.Stmt
}

func newBatchWriter(ctx context.Context, tx *sql.Tx, userID int64) (*batchWriter, error) {
	w := &batchWriter{tx: tx, userID: userID, wordIDs: map[wordKey]int64{}, tagIDs: map[string]int64{}}
	for dst, query := range map[**sql.Stmt]string{
		&w.insWord:     `INSERT OR IGNORE INTO words (text, language, pinyin, user_id) VALUES (?, ?, ?, ?)`,
		&w.selWord:     `SELECT id FROM words WHERE text = ? AND language = ? AND user_id = ?`,
		&w.insSM2:      `INSERT OR IGNORE INTO sm2_progress (word_id) VALUES (?)`,
		&w.insLink:     `INSERT OR IGNORE INTO translations (translation_word_id, zh_word_id, source, rank) VALUES (?, ?, ?, ?)`,
		&w.selRank:     `SELECT rank FROM gloss_rank WHERE lang = ? AND gloss = ?`,
		&w.delWordTags: `DELETE FROM word_tags WHERE word_id = ?`,
		&w.insWordTag:  `INSERT OR IGNORE INTO word_tags (word_id, tag_id) VALUES (?, ?)`,
		&w.selTag:      `SELECT id FROM tags WHERE name = ?`,
		&w.insTag:      `INSERT INTO tags (name) VALUES (?)`,
	} {
		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			w.close()
			return nil, fmt.Errorf("prepare batch statement: %w", err)
		}
		*dst = stmt
		w.stmts = append(w.stmts, stmt)
	}
	return w, nil
}

func (w *batchWriter) close() {
	for _, stmt := range w.stmts {
		stmt.Close()
	}
}

// word returns the ID of the user's word, creating it (with its progress row)
// when missing. Like upsertWord, an existing word keeps its pinyin.
func (w *batchWriter) word(ctx context.Context, text, lang string, pinyin *string) (int64, error) {
	text = strings.TrimSpace(text)
	key := wordKey{text, lang}
	if id, ok := w.wordIDs[key]; ok {
		return id, nil
	}
	if _, err := w.insWord.ExecContext(ctx, text, lang, pinyin, w.userID); err != nil {
		return 0, fmt.Errorf("upsert word: %w", err)
	}
	var id int64
	if err := w.selWord.QueryRowContext(ctx, text, lang, w.userID).Scan(&id); err != nil {
		return 0, fmt.Errorf("get word id: %w", err)
	}
	if _, err := w.insSM2.ExecContext(ctx, id); err != nil {
		return 0, fmt.Errorf("init sm2: %w", err)
	}
	w.wordIDs[key] = id
	return id, nil
}

func (w *batchWriter) tag(ctx context.Context, name string) (int64, error) {
	if id, ok := w.tagIDs[name]; ok {
		return id, nil
	}
	var id int64
	err := w.selTag.QueryRowContext(ctx, name).Scan(&id)
	if err == sql.ErrNoRows {
		res, insErr := w.insTag.ExecContext(ctx, name)
		if insErr != nil {
			return 0, fmt.Errorf("insert tag: %w", insErr)
		}
		id, err = res.LastInsertId()
	}
	if err != nil {
		return 0, fmt.Errorf("get tag id: %w", err)
	}
	w.tagIDs[name] = id
	return id, nil
}

// rank mirrors linkTranslation: a "user" translation is always rank 0, a
// "cedict" one uses the gloss_rank cache with computeTranslationRank as
// fallback.
func (w *batchWriter) rank(ctx context.Context, lang, text, source string) (sql.NullInt64, error) {
	rank := sql.NullInt64{Int64: 0, Valid: true}
	if source != "cedict" {
		return rank, nil
	}
	err := w.selRank.QueryRowContext(ctx, lang, text).Scan(&rank)
	if err == sql.ErrNoRows {
		return computeTranslationRank(ctx, w.tx, lang, text)
	}
	return rank, err
}

func (w *batchWriter) create(ctx context.Context, req models.CreateWordRequest) (int64, error) {
	zhID, err := w.word(ctx, req.ZhText, "zh", &req.Pinyin)
	if err != nil {
		return 0, err
	}
	for lang, texts := range req.Translations {
		for i, text := range texts {
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			transID, err := w.word(ctx, text, lang, nil)
			if err != nil {
				return 0, err
			}
			source := sourceAt(req.TranslationSources[lang], i)
			rank, err := w.rank(ctx, lang, text, source)
			if err != nil {
				return 0, fmt.Errorf("link %s translation: %w", lang, err)
			}
			if _, err := w.insLink.ExecContext(ctx, transID, zhID, source, rank); err != nil {
				return 0, fmt.Errorf("link %s translation: %w", lang, err)
			}
		}
	}
	if _, err := w.delWordTags.ExecContext(ctx, zhID); err != nil {
		return 0, fmt.Errorf("delete word tags: %w", err)
	}
	for _, name := range req.Tags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		tagID, err := w.tag(ctx, name)
		if err != nil {
			return 0, err
		}
		if _, err := w.insWordTag.ExecContext(ctx, zhID, tagID); err != nil {
			return 0, fmt.Errorf("link tag: %w", err)
		}
	}
	return zhID, nil
}

// CreateWordsBatch creates every word in reqs inside one transaction and
// returns the zh word IDs in request order. The stored result equals calling
// CreateWord for each request, except that StartTraining is not supported and
// that one failing word rolls back the whole batch.
func (s *Store) CreateWordsBatch(ctx context.Context, userID int64, reqs []models.CreateWordRequest) ([]int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	w, err := newBatchWriter(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	defer w.close()

	ids := make([]int64, 0, len(reqs))
	for _, req := range reqs {
		id, err := w.create(ctx, req)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}
