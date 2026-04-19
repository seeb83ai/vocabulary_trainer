package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v27: add user_id to hmm_actors; PK changes from (initial) → (user_id, initial).
		version: 27,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS hmm_actors_new (
				  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  initial    TEXT    NOT NULL,
				  category   TEXT    NOT NULL,
				  actor_name TEXT    NOT NULL DEFAULT '',
				  hint       TEXT    NOT NULL DEFAULT '',
				  PRIMARY KEY (user_id, initial)
				)`,
				`INSERT OR IGNORE INTO hmm_actors_new (user_id, initial, category, actor_name, hint)
				 SELECT 2,
				        initial, category, actor_name, hint FROM hmm_actors`,
				`DROP TABLE hmm_actors`,
				`ALTER TABLE hmm_actors_new RENAME TO hmm_actors`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild hmm_actors table: %w", err)
				}
			}
			return nil
		},
	})
}
