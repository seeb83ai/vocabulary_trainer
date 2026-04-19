package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 8,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sm2_progress') WHERE name = 'streak_bonus'`).Scan(&count); err != nil {
				return fmt.Errorf("check streak_bonus column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE sm2_progress ADD COLUMN streak_bonus INTEGER NOT NULL DEFAULT 0`); err != nil {
					return fmt.Errorf("add streak_bonus column: %w", err)
				}
			}
			return nil
		},
	})
}
