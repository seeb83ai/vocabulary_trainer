package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260905090000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('sm2_progress') WHERE name = 'cycle_pin_mode'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check cycle_pin_mode column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE sm2_progress ADD COLUMN cycle_pin_mode TEXT`,
				); err != nil {
					return fmt.Errorf("add cycle_pin_mode column: %w", err)
				}
			}
			return nil
		},
	})
}
