package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260826120001,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('sm2_progress') WHERE name = 'known_correct_count'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check known_correct_count column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE sm2_progress ADD COLUMN known_correct_count INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add known_correct_count column: %w", err)
				}
			}
			return nil
		},
	})
}
