package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 3,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('words') WHERE name = 'needs_review'`).Scan(&count); err != nil {
				return fmt.Errorf("check needs_review column: %w", err)
			}
			if count > 0 {
				return nil // column already exists
			}
			if _, err := db.Exec(`ALTER TABLE words ADD COLUMN needs_review INTEGER DEFAULT 0`); err != nil {
				return fmt.Errorf("add needs_review column: %w", err)
			}
			return nil
		},
	})
}
