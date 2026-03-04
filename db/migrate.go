package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// migration describes a single schema migration step.
type migration struct {
	version int
	sql     string            // executed first (may be empty)
	fn      func(*sql.DB) error // executed after sql (may be nil)
}

// migrations is the ordered list of all schema migrations.
// Version 1 corresponds to the full initial schema (works on both fresh and
// existing databases thanks to IF NOT EXISTS / duplicate-column guards).
// Append new migrations at the end with incrementing version numbers.
var migrations = []migration{
	{
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
	},
	{
		version: 2,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('words') WHERE name = 'needs_review'`).Scan(&count); err != nil {
				return fmt.Errorf("check needs_review column: %w", err)
			}
			if count > 0 {
				return nil // column already exists
			}
			if _, err := db.Exec(`ALTER TABLE words ADD COLUMN needs_review INTEGER DEFAULT 0`); err != nil {
				return fmt.Errorf("add needs_review column: %w", err)
			}
			return nil
		},
	},
}

// Migrate runs all pending migrations on the given database.
// Exported so cmd/import and cmd/import-hsk can call it directly on a *sql.DB.
func Migrate(database *sql.DB) error {
	// Ensure the version-tracking table exists.
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL DEFAULT 0)`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	// Seed with version 0 if the table is empty (fresh DB or first run of
	// the migration system on an existing DB).
	if _, err := database.Exec(`INSERT INTO schema_version (version) SELECT 0 WHERE NOT EXISTS (SELECT 1 FROM schema_version)`); err != nil {
		return fmt.Errorf("seed schema_version: %w", err)
	}

	var current int
	if err := database.QueryRow(`SELECT version FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if m.sql != "" {
			if _, err := database.Exec(m.sql); err != nil {
				return fmt.Errorf("migration %d sql: %w", m.version, err)
			}
		}
		if m.fn != nil {
			if err := m.fn(database); err != nil {
				return fmt.Errorf("migration %d fn: %w", m.version, err)
			}
		}
		if _, err := database.Exec(`UPDATE schema_version SET version = ?`, m.version); err != nil {
			return fmt.Errorf("update schema version to %d: %w", m.version, err)
		}
	}
	return nil
}
