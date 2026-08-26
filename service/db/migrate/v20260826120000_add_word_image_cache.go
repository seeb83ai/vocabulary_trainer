package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260826120000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('words') WHERE name = 'image_url'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check image_url column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE words ADD COLUMN image_url TEXT`,
				); err != nil {
					return fmt.Errorf("add image_url column: %w", err)
				}
			}
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('words') WHERE name = 'image_fetched_at'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check image_fetched_at column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE words ADD COLUMN image_fetched_at DATETIME`,
				); err != nil {
					return fmt.Errorf("add image_fetched_at column: %w", err)
				}
			}
			return nil
		},
	})
}
