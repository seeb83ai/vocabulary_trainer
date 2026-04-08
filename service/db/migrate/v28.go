package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v28: add user_id to hmm_locations; PK changes from (final_key) → (user_id, final_key).
		version: 28,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS hmm_locations_new (
				  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  final_key     TEXT    NOT NULL,
				  location_name TEXT    NOT NULL DEFAULT '',
				  PRIMARY KEY (user_id, final_key)
				)`,
				`INSERT OR IGNORE INTO hmm_locations_new (user_id, final_key, location_name)
				 SELECT (SELECT id FROM users WHERE email = 'me@elygor.de'),
				        final_key, location_name FROM hmm_locations`,
				`DROP TABLE hmm_locations`,
				`ALTER TABLE hmm_locations_new RENAME TO hmm_locations`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild hmm_locations table: %w", err)
				}
			}
			return nil
		},
	})
}
