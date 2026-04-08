package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v22: add user_id to tags and change UNIQUE(name) → UNIQUE(name, user_id).
		// Existing tags stay user_id = NULL (global/shared labels).
		version: 22,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS tags_new (
				  id      INTEGER PRIMARY KEY AUTOINCREMENT,
				  name    TEXT    NOT NULL,
				  user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
				  UNIQUE(name, user_id)
				)`,
				`INSERT OR IGNORE INTO tags_new (id, name, user_id)
				 SELECT id, name, NULL FROM tags`,
				`DROP TABLE tags`,
				`ALTER TABLE tags_new RENAME TO tags`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild tags table: %w", err)
				}
			}
			return nil
		},
	})
}
