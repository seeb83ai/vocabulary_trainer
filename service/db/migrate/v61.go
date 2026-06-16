package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 61,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('confusion_pairs') WHERE name = 'last_shown_in_game'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check last_shown_in_game column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE confusion_pairs ADD COLUMN last_shown_in_game TEXT`,
				); err != nil {
					return fmt.Errorf("add last_shown_in_game column: %w", err)
				}
			}
			return nil
		},
	})
}
