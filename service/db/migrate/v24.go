package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v24: add user_id to pinyin_progress; PK changes from (sound_id) → (user_id, sound_id).
		// pinyin_sounds remains shared content. Existing rows → initial user.
		version: 24,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS pinyin_progress_new (
				  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  sound_id        INTEGER NOT NULL REFERENCES pinyin_sounds(id) ON DELETE CASCADE,
				  repetitions     INTEGER NOT NULL DEFAULT 0,
				  easiness        REAL    NOT NULL DEFAULT 2.5,
				  interval_days   INTEGER NOT NULL DEFAULT 1,
				  due_date        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				  total_correct   INTEGER NOT NULL DEFAULT 0,
				  total_attempts  INTEGER NOT NULL DEFAULT 0,
				  learning        INTEGER NOT NULL DEFAULT 1,
				  streak_bonus    INTEGER NOT NULL DEFAULT 0,
				  first_seen_date TEXT DEFAULT NULL,
				  PRIMARY KEY (user_id, sound_id)
				)`,
				`INSERT OR IGNORE INTO pinyin_progress_new
				   (user_id, sound_id, repetitions, easiness, interval_days, due_date,
				    total_correct, total_attempts, learning, streak_bonus, first_seen_date)
				 SELECT
				   (SELECT id FROM users WHERE email = 'me@elygor.de'),
				   sound_id, repetitions, easiness, interval_days, due_date,
				   total_correct, total_attempts, learning, streak_bonus, first_seen_date
				 FROM pinyin_progress`,
				`DROP TABLE pinyin_progress`,
				`ALTER TABLE pinyin_progress_new RENAME TO pinyin_progress`,
				`CREATE INDEX IF NOT EXISTS idx_pinyin_progress_due ON pinyin_progress(user_id, due_date)`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild pinyin_progress table: %w", err)
				}
			}
			return nil
		},
	})
}
