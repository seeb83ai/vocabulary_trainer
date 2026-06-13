package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 57,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('daily_stats') WHERE name = 'training_seconds'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check training_seconds column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE daily_stats ADD COLUMN training_seconds INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add training_seconds column: %w", err)
				}
			}
			return nil
		},
	})
}
