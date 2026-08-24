package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260816170000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'component_coverage_threshold'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check component_coverage_threshold column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN component_coverage_threshold REAL NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add component_coverage_threshold column: %w", err)
				}
			}
			return nil
		},
	})
}
