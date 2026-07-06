package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 62,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'cycle_advance_on_success_only'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check cycle_advance_on_success_only column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN cycle_advance_on_success_only INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add cycle_advance_on_success_only column: %w", err)
				}
			}
			return nil
		},
	})
}
