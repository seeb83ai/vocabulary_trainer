package db

import (
	"context"
	"fmt"
	"strings"
	"vocabulary_trainer/models"
)

// LibraryUserID owns the shared library (see ADR-0005).
const LibraryUserID int64 = 1

// libraryLangs are the languages the library takes glosses for: CC-CEDICT
// for en, HanDeDict for de.
var libraryLangs = []string{"en", "de"}

type libraryWord struct {
	id      int64
	text    string
	removed bool
}

// RefreshLibrary brings every library zh word's glosses in line with the
// dictionaries (each definition split into senses, as the import does).
// Rows are upserted by text, so unchanged glosses keep their ids and learner
// references and deletions stay valid. A word whose existing gloss set changes
// gets library_updated_at; a word with no dictionary entry any more keeps its
// glosses and is flagged library_removed. Gloss words left without any link
// are deleted.
func (s *Store) RefreshLibrary(ctx context.Context) (models.LibraryRefreshReport, error) {
	var report models.LibraryRefreshReport

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, text, library_removed FROM words WHERE user_id = ? AND language = 'zh' ORDER BY id`, LibraryUserID)
	if err != nil {
		return report, fmt.Errorf("load library words: %w", err)
	}
	var words []libraryWord
	for rows.Next() {
		var w libraryWord
		if err := rows.Scan(&w.id, &w.text, &w.removed); err != nil {
			rows.Close()
			return report, err
		}
		words = append(words, w)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return report, err
	}
	rows.Close()

	current, err := s.libraryLinks(ctx)
	if err != nil {
		return report, err
	}

	texts := make([]string, len(words))
	for i, w := range words {
		texts[i] = w.text
	}
	dict, err := s.LookupDictionaryBatch(ctx, texts, libraryLangs)
	if err != nil {
		return report, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	bw, err := newBatchWriter(ctx, tx, LibraryUserID)
	if err != nil {
		return report, err
	}
	defer bw.close()
	bw.noProgress = true

	for _, w := range words {
		report.Words++
		want := map[wordKey]bool{}
		for lang, senses := range dict[w.text] {
			for _, sense := range senses {
				if sense = strings.TrimSpace(sense); sense != "" {
					want[wordKey{sense, lang}] = true
				}
			}
		}
		if len(want) == 0 {
			if !w.removed {
				if _, err := tx.ExecContext(ctx, `UPDATE words SET library_removed = 1 WHERE id = ?`, w.id); err != nil {
					return report, fmt.Errorf("flag removed library word: %w", err)
				}
			}
			report.Missing++
			continue
		}
		if w.removed {
			if _, err := tx.ExecContext(ctx, `UPDATE words SET library_removed = 0 WHERE id = ?`, w.id); err != nil {
				return report, fmt.Errorf("unflag library word: %w", err)
			}
		}

		have := current[w.id]
		changed := false
		for key := range want {
			if _, ok := have[key]; ok {
				continue
			}
			transID, err := bw.word(ctx, key.text, key.lang, nil)
			if err != nil {
				return report, err
			}
			rank, err := bw.rank(ctx, key.lang, key.text, "cedict")
			if err != nil {
				return report, err
			}
			if _, err := bw.insLink.ExecContext(ctx, transID, w.id, "cedict", rank); err != nil {
				return report, fmt.Errorf("link library gloss: %w", err)
			}
			report.Added++
			changed = true
		}
		for key, transID := range have {
			if want[key] {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM translations WHERE translation_word_id = ? AND zh_word_id = ?`, transID, w.id); err != nil {
				return report, fmt.Errorf("drop library gloss: %w", err)
			}
			report.Dropped++
			changed = true
		}
		if changed && len(have) > 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE words SET library_updated_at = CURRENT_TIMESTAMP WHERE id = ?`, w.id); err != nil {
				return report, fmt.Errorf("mark library word updated: %w", err)
			}
			report.Changed++
		}
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM words WHERE user_id = ? AND language != 'zh'
		   AND NOT EXISTS (SELECT 1 FROM translations t WHERE t.translation_word_id = words.id)`, LibraryUserID); err != nil {
		return report, fmt.Errorf("delete orphan library glosses: %w", err)
	}
	return report, tx.Commit()
}

// libraryLinks returns every library zh word's gloss links: zh id → (text,
// lang) → gloss word id.
func (s *Store) libraryLinks(ctx context.Context) (map[int64]map[wordKey]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.zh_word_id, tw.text, tw.language, tw.id
		 FROM translations t
		 JOIN words z  ON z.id  = t.zh_word_id
		 JOIN words tw ON tw.id = t.translation_word_id
		 WHERE z.user_id = ? AND z.language = 'zh'`, LibraryUserID)
	if err != nil {
		return nil, fmt.Errorf("load library links: %w", err)
	}
	defer rows.Close()
	out := map[int64]map[wordKey]int64{}
	for rows.Next() {
		var zhID, transID int64
		var key wordKey
		if err := rows.Scan(&zhID, &key.text, &key.lang, &transID); err != nil {
			return nil, err
		}
		if out[zhID] == nil {
			out[zhID] = map[wordKey]int64{}
		}
		out[zhID][key] = transID
	}
	return out, rows.Err()
}

// LibraryNeedsPrefill reports whether the library has zh words but no gloss
// at all while the dictionaries are loaded — the state right after upgrading
// to library references, or after the first dictionary import.
func (s *Store) LibraryNeedsPrefill(ctx context.Context) (bool, error) {
	var need bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM words WHERE user_id = ? AND language = 'zh')
		   AND NOT EXISTS (SELECT 1 FROM translations t JOIN words z ON z.id = t.zh_word_id
		                   WHERE z.user_id = ? AND z.language = 'zh')
		   AND EXISTS (SELECT 1 FROM cedict_entries)`, LibraryUserID, LibraryUserID).Scan(&need)
	if err != nil {
		return false, fmt.Errorf("check library prefill: %w", err)
	}
	return need, nil
}
