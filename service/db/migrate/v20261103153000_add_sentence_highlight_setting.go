package migrate

import (
	"database/sql"
	"fmt"
)

// Issue #640: show the missing word of a sentence card in bold; on by default.
func init() {
	register(migration{
		version: 20261103153000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'sentence_highlight_enabled'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check sentence_highlight_enabled column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN sentence_highlight_enabled INTEGER NOT NULL DEFAULT 1`,
				); err != nil {
					return fmt.Errorf("add sentence_highlight_enabled column: %w", err)
				}
			}
			return nil
		},
	})
}
