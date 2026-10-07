package migrate

import (
	"database/sql"
	"fmt"
)

// v20261007121000: user_settings.leech_threshold — the answer screen warns
// after this many lapses in a row (and at each multiple). 0 turns it off.
func init() {
	register(migration{
		version: 20261007121000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'leech_threshold'`).Scan(&count); err != nil {
				return fmt.Errorf("check leech_threshold column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN leech_threshold INTEGER NOT NULL DEFAULT 5`); err != nil {
					return fmt.Errorf("add leech_threshold column: %w", err)
				}
			}
			return nil
		},
	})
}
