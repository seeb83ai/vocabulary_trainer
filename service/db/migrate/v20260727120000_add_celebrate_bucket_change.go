package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260727120000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'celebrate_bucket_change'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check celebrate_bucket_change column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN celebrate_bucket_change INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add celebrate_bucket_change column: %w", err)
				}
			}
			return nil
		},
	})
}
