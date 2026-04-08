package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v25: add user_id to pinyin_confusions; PK changes to (user_id, sound_id, confused_with_id).
		version: 25,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS pinyin_confusions_new (
				  user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  sound_id         INTEGER NOT NULL REFERENCES pinyin_sounds(id) ON DELETE CASCADE,
				  confused_with_id INTEGER NOT NULL REFERENCES pinyin_sounds(id) ON DELETE CASCADE,
				  count            INTEGER NOT NULL DEFAULT 1,
				  last_seen        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				  PRIMARY KEY (user_id, sound_id, confused_with_id)
				)`,
				`INSERT OR IGNORE INTO pinyin_confusions_new
				   (user_id, sound_id, confused_with_id, count, last_seen)
				 SELECT
				   (SELECT id FROM users WHERE email = 'me@elygor.de'),
				   sound_id, confused_with_id, count, last_seen
				 FROM pinyin_confusions`,
				`DROP TABLE pinyin_confusions`,
				`ALTER TABLE pinyin_confusions_new RENAME TO pinyin_confusions`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild pinyin_confusions table: %w", err)
				}
			}
			return nil
		},
	})
}
