package migrate

import (
	"database/sql"
	"fmt"
)

// v20261004204500: issue #536 — "Skip for today" can show the answer of the
// skipped card. Off by default.
func init() {
	register(migration{
		version: 20261004204500,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'skip_reveal_answer'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check skip_reveal_answer column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN skip_reveal_answer INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add skip_reveal_answer column: %w", err)
				}
			}
			return nil
		},
	})
}
