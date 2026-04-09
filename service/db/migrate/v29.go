package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v29: add user_id to hmm_tone_rooms; PK changes from (tone) → (user_id, tone).
		version: 29,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS hmm_tone_rooms_new (
				  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  tone      INTEGER NOT NULL,
				  room_name TEXT    NOT NULL DEFAULT '',
				  PRIMARY KEY (user_id, tone)
				)`,
				`INSERT OR IGNORE INTO hmm_tone_rooms_new (user_id, tone, room_name)
				 SELECT 2,
				        tone, room_name FROM hmm_tone_rooms`,
				`DROP TABLE hmm_tone_rooms`,
				`ALTER TABLE hmm_tone_rooms_new RENAME TO hmm_tone_rooms`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild hmm_tone_rooms table: %w", err)
				}
			}
			return nil
		},
	})
}
