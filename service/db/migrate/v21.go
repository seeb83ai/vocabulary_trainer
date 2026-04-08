package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v21: add user_id to words and change UNIQUE(text, language) →
		// UNIQUE(text, language, user_id) so the same word can exist independently
		// for different users. Existing rows are reassigned to the initial user;
		// template rows (user_id=NULL) are seeded by copying that user's vocabulary
		// (without progress).
		version: 21,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS words_new (
				  id           INTEGER PRIMARY KEY AUTOINCREMENT,
				  text         TEXT    NOT NULL,
				  language     TEXT    NOT NULL CHECK(language IN ('en', 'zh', 'de')),
				  pinyin       TEXT,
				  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
				  needs_review INTEGER NOT NULL DEFAULT 0,
				  user_id      INTEGER REFERENCES users(id) ON DELETE CASCADE,
				  UNIQUE(text, language, user_id)
				)`,
				`INSERT OR IGNORE INTO words_new (id, text, language, pinyin, created_at, needs_review, user_id)
				 SELECT id, text, language, pinyin, created_at, COALESCE(needs_review, 0), NULL FROM words`,
				`DROP TABLE words`,
				`ALTER TABLE words_new RENAME TO words`,
				`CREATE INDEX IF NOT EXISTS idx_words_text_lang ON words(text, language, user_id)`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild words table: %w", err)
				}
			}

			// Assign all existing words to the initial user.
			if _, err := db.Exec(
				`UPDATE words SET user_id = (SELECT id FROM users WHERE email = 'me@elygor.de') WHERE user_id IS NULL`,
			); err != nil {
				return fmt.Errorf("assign words to initial user: %w", err)
			}

			// Seed template words (user_id = NULL) by copying the initial user's
			// vocabulary without progress data.
			var userID int64
			if err := db.QueryRow(`SELECT id FROM users WHERE email = 'me@elygor.de'`).Scan(&userID); err != nil {
				return fmt.Errorf("get initial user id: %w", err)
			}

			// Copy non-zh words first (translations reference both sides).
			enRows, err := db.Query(
				`SELECT id, text, language FROM words WHERE user_id = ? AND language != 'zh'`, userID)
			if err != nil {
				return fmt.Errorf("query non-zh words: %w", err)
			}
			type wordRow struct {
				id   int64
				text string
				lang string
			}
			var nonZhWords []wordRow
			for enRows.Next() {
				var w wordRow
				if err := enRows.Scan(&w.id, &w.text, &w.lang); err != nil {
					enRows.Close()
					return fmt.Errorf("scan non-zh word: %w", err)
				}
				nonZhWords = append(nonZhWords, w)
			}
			enRows.Close()
			if err := enRows.Err(); err != nil {
				return fmt.Errorf("iterate non-zh words: %w", err)
			}

			// oldID → newTemplateID mapping for non-zh words.
			tmplNonZh := make(map[int64]int64, len(nonZhWords))
			for _, w := range nonZhWords {
				res, err := db.Exec(
					`INSERT OR IGNORE INTO words (text, language, user_id) VALUES (?, ?, NULL)`,
					w.text, w.lang)
				if err != nil {
					return fmt.Errorf("insert template non-zh word: %w", err)
				}
				newID, err := res.LastInsertId()
				if err != nil || newID == 0 {
					// Row already existed (INSERT OR IGNORE skipped it); look it up.
					if err2 := db.QueryRow(
						`SELECT id FROM words WHERE text = ? AND language = ? AND user_id IS NULL`,
						w.text, w.lang,
					).Scan(&newID); err2 != nil {
						return fmt.Errorf("lookup template non-zh word %q: %w", w.text, err2)
					}
				}
				tmplNonZh[w.id] = newID
			}

			// Copy zh words.
			zhRows, err := db.Query(
				`SELECT id, text, pinyin FROM words WHERE user_id = ? AND language = 'zh'`, userID)
			if err != nil {
				return fmt.Errorf("query zh words: %w", err)
			}
			type zhWordRow struct {
				id     int64
				text   string
				pinyin *string
			}
			var zhWords []zhWordRow
			for zhRows.Next() {
				var w zhWordRow
				if err := zhRows.Scan(&w.id, &w.text, &w.pinyin); err != nil {
					zhRows.Close()
					return fmt.Errorf("scan zh word: %w", err)
				}
				zhWords = append(zhWords, w)
			}
			zhRows.Close()
			if err := zhRows.Err(); err != nil {
				return fmt.Errorf("iterate zh words: %w", err)
			}

			// oldID → newTemplateID mapping for zh words.
			tmplZh := make(map[int64]int64, len(zhWords))
			for _, w := range zhWords {
				res, err := db.Exec(
					`INSERT OR IGNORE INTO words (text, language, pinyin, user_id) VALUES (?, 'zh', ?, NULL)`,
					w.text, w.pinyin)
				if err != nil {
					return fmt.Errorf("insert template zh word: %w", err)
				}
				newID, err := res.LastInsertId()
				if err != nil || newID == 0 {
					if err2 := db.QueryRow(
						`SELECT id FROM words WHERE text = ? AND language = 'zh' AND user_id IS NULL`,
						w.text,
					).Scan(&newID); err2 != nil {
						return fmt.Errorf("lookup template zh word %q: %w", w.text, err2)
					}
				}
				tmplZh[w.id] = newID
			}

			// Copy translations: re-link using template IDs.
			tRows, err := db.Query(
				`SELECT en_word_id, zh_word_id FROM translations
				 WHERE zh_word_id IN (SELECT id FROM words WHERE user_id = ?)`, userID)
			if err != nil {
				return fmt.Errorf("query translations: %w", err)
			}
			type translationRow struct{ enID, zhID int64 }
			var translations []translationRow
			for tRows.Next() {
				var tr translationRow
				if err := tRows.Scan(&tr.enID, &tr.zhID); err != nil {
					tRows.Close()
					return fmt.Errorf("scan translation: %w", err)
				}
				translations = append(translations, tr)
			}
			tRows.Close()
			if err := tRows.Err(); err != nil {
				return fmt.Errorf("iterate translations: %w", err)
			}
			for _, tr := range translations {
				newEnID, okEn := tmplNonZh[tr.enID]
				newZhID, okZh := tmplZh[tr.zhID]
				if !okEn || !okZh {
					continue
				}
				if _, err := db.Exec(
					`INSERT OR IGNORE INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`,
					newEnID, newZhID,
				); err != nil {
					return fmt.Errorf("insert template translation: %w", err)
				}
			}

			// Copy word_tags: reuse global tag IDs, apply to template word IDs.
			wtRows, err := db.Query(
				`SELECT word_id, tag_id FROM word_tags
				 WHERE word_id IN (SELECT id FROM words WHERE user_id = ?)`, userID)
			if err != nil {
				return fmt.Errorf("query word_tags: %w", err)
			}
			type wordTagRow struct{ wordID, tagID int64 }
			var wordTags []wordTagRow
			for wtRows.Next() {
				var wt wordTagRow
				if err := wtRows.Scan(&wt.wordID, &wt.tagID); err != nil {
					wtRows.Close()
					return fmt.Errorf("scan word_tag: %w", err)
				}
				wordTags = append(wordTags, wt)
			}
			wtRows.Close()
			if err := wtRows.Err(); err != nil {
				return fmt.Errorf("iterate word_tags: %w", err)
			}
			for _, wt := range wordTags {
				newID, ok := tmplZh[wt.wordID]
				if !ok {
					newID, ok = tmplNonZh[wt.wordID]
				}
				if !ok {
					continue
				}
				if _, err := db.Exec(
					`INSERT OR IGNORE INTO word_tags (word_id, tag_id) VALUES (?, ?)`,
					newID, wt.tagID,
				); err != nil {
					return fmt.Errorf("insert template word_tag: %w", err)
				}
			}

			return nil
		},
	})
}
