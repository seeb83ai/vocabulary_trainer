package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v31: add user_id to hmm_progress; PK changes from (entity_type, entity_key)
		// → (user_id, entity_type, entity_key). Existing rows → initial user.
		version: 31,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS hmm_progress_new (
				  user_id         INTEGER  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  entity_type     TEXT     NOT NULL,
				  entity_key      TEXT     NOT NULL,
				  repetitions     INTEGER  NOT NULL DEFAULT 0,
				  easiness        REAL     NOT NULL DEFAULT 2.5,
				  interval_days   INTEGER  NOT NULL DEFAULT 1,
				  due_date        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				  total_correct   INTEGER  NOT NULL DEFAULT 0,
				  total_attempts  INTEGER  NOT NULL DEFAULT 0,
				  learning        INTEGER  NOT NULL DEFAULT 1,
				  streak_bonus    INTEGER  NOT NULL DEFAULT 0,
				  first_seen_date TEXT     DEFAULT NULL,
				  PRIMARY KEY (user_id, entity_type, entity_key)
				)`,
				`INSERT OR IGNORE INTO hmm_progress_new
				   (user_id, entity_type, entity_key, repetitions, easiness, interval_days,
				    due_date, total_correct, total_attempts, learning, streak_bonus, first_seen_date)
				 SELECT
				   2,
				   entity_type, entity_key, repetitions, easiness, interval_days,
				   due_date, total_correct, total_attempts, learning, streak_bonus, first_seen_date
				 FROM hmm_progress`,
				`DROP TABLE hmm_progress`,
				`ALTER TABLE hmm_progress_new RENAME TO hmm_progress`,
				`CREATE INDEX IF NOT EXISTS idx_hmm_progress_due ON hmm_progress(user_id, due_date)`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild hmm_progress table: %w", err)
				}
			}
			return nil
		},
	})
}
