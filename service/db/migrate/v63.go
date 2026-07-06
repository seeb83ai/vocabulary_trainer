package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 63,
		fn: func(db *sql.DB) error {
			// Add drill_flag to sm2_progress. A flagged word is part of the
			// active "difficult words" drill; the flag is cleared once the word
			// is answered correctly.
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sm2_progress') WHERE name = 'drill_flag'`).Scan(&count); err != nil {
				return fmt.Errorf("check drill_flag column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE sm2_progress ADD COLUMN drill_flag INTEGER NOT NULL DEFAULT 0`); err != nil {
					return fmt.Errorf("add drill_flag column: %w", err)
				}
			}
			return nil
		},
	})
}
