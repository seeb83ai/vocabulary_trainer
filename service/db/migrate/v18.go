package migrate

import (
	"database/sql"
	"fmt"
	"strings"
)

func init() {
	register(migration{
		// v18: expand words.language CHECK to allow 'de' and add de_translations table.
		// SQLite does not support ALTER CONSTRAINT, so the words table is recreated
		// (same pattern as v12 for pinyin_sounds).
		version: 18,
		fn: func(db *sql.DB) error {
			// Check if 'de' is already in the CHECK — skip if so (idempotent).
			var wordsSql string
			if err := db.QueryRow(
				`SELECT sql FROM sqlite_master WHERE type='table' AND name='words'`,
			).Scan(&wordsSql); err != nil {
				return fmt.Errorf("read words schema: %w", err)
			}
			if !strings.Contains(wordsSql, "'de'") {
				// Disable FK enforcement so DROP TABLE words doesn't cascade-delete
				// translations, sm2_progress, word_tags, confusion_pairs, hmm_scenes.
				if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
					return fmt.Errorf("disable foreign keys: %w", err)
				}
				defer db.Exec(`PRAGMA foreign_keys = ON`)
				for _, stmt := range []string{
					`CREATE TABLE words_new (
					  id           INTEGER PRIMARY KEY AUTOINCREMENT,
					  text         TEXT    NOT NULL,
					  language     TEXT    NOT NULL CHECK(language IN ('en', 'zh', 'de')),
					  pinyin       TEXT,
					  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
					  needs_review INTEGER NOT NULL DEFAULT 0,
					  UNIQUE(text, language)
					)`,
					`INSERT OR IGNORE INTO words_new (id, text, language, pinyin, created_at, needs_review)
					 SELECT id, text, language, pinyin, created_at, COALESCE(needs_review, 0) FROM words`,
					`DROP TABLE words`,
					`ALTER TABLE words_new RENAME TO words`,
					`CREATE INDEX IF NOT EXISTS idx_words_text_lang ON words(text, language)`,
				} {
					if _, err := db.Exec(stmt); err != nil {
						return fmt.Errorf("expand words language check: %w", err)
					}
				}
			}
			return nil
		},
	})
}
