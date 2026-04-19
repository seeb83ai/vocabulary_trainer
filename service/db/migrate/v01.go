package migrate

import (
	"database/sql"
	"fmt"
	"strings"
)

func init() {
	register(migration{
		version: 1,
		sql: `
CREATE TABLE IF NOT EXISTS words (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  text       TEXT    NOT NULL,
  language   TEXT    NOT NULL CHECK(language IN ('en', 'zh')),
  pinyin     TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(text, language)
);

CREATE TABLE IF NOT EXISTS translations (
  en_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  zh_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  PRIMARY KEY (en_word_id, zh_word_id)
);

CREATE TABLE IF NOT EXISTS sm2_progress (
  word_id          INTEGER PRIMARY KEY REFERENCES words(id) ON DELETE CASCADE,
  repetitions      INTEGER NOT NULL DEFAULT 0,
  easiness         REAL    NOT NULL DEFAULT 2.5,
  interval_days    INTEGER NOT NULL DEFAULT 1,
  due_date         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  total_correct    INTEGER NOT NULL DEFAULT 0,
  total_attempts   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS confusion_pairs (
  zh_word_id       INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  confused_with_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  mode             TEXT    NOT NULL,
  count            INTEGER NOT NULL DEFAULT 1,
  last_seen        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (zh_word_id, confused_with_id, mode)
);

CREATE TABLE IF NOT EXISTS tags (
  id   INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS word_tags (
  word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (word_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_sm2_due ON sm2_progress(due_date);
CREATE INDEX IF NOT EXISTS idx_words_text_lang ON words(text, language);
CREATE INDEX IF NOT EXISTS idx_translations_zh ON translations(zh_word_id);
CREATE INDEX IF NOT EXISTS idx_word_tags_word ON word_tags(word_id);
`,
		fn: func(db *sql.DB) error {
			// Add first_seen_date to sm2_progress for existing databases
			// that pre-date this column. Fresh databases get it from the
			// CREATE TABLE above only starting from migration v2+; for v1
			// we always attempt the ALTER so existing production DBs are
			// covered.
			if _, err := db.Exec(`ALTER TABLE sm2_progress ADD COLUMN first_seen_date TEXT DEFAULT NULL`); err != nil {
				if !strings.Contains(err.Error(), "duplicate column name") {
					return fmt.Errorf("add first_seen_date column: %w", err)
				}
			} else {
				// Column was just added — backfill: mark already-tested
				// words as seen yesterday so they are not treated as "new"
				// by the daily new-word cap.
				if _, err := db.Exec(`UPDATE sm2_progress SET first_seen_date = DATE('now', '-1 day') WHERE total_attempts > 0`); err != nil {
					return fmt.Errorf("backfill first_seen_date: %w", err)
				}
			}
			if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sm2_first_seen ON sm2_progress(first_seen_date)`); err != nil {
				return fmt.Errorf("create first_seen index: %w", err)
			}
			return nil
		},
	})
}
