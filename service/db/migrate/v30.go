package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v30: add user_id to hmm_props; PK changes from (radical) → (user_id, radical).
		version: 30,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS hmm_props_new (
				  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  radical   TEXT    NOT NULL,
				  prop_name TEXT    NOT NULL DEFAULT '',
				  PRIMARY KEY (user_id, radical)
				)`,
				`INSERT OR IGNORE INTO hmm_props_new (user_id, radical, prop_name)
				 SELECT 2,
				        radical, prop_name FROM hmm_props`,
				`DROP TABLE hmm_props`,
				`ALTER TABLE hmm_props_new RENAME TO hmm_props`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild hmm_props table: %w", err)
				}
			}
			return nil
		},
	})
}
