package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 4,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('daily_stats') WHERE name = 'words_seen'`).Scan(&count); err != nil {
				return fmt.Errorf("check words_seen column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE daily_stats ADD COLUMN words_seen INTEGER NOT NULL DEFAULT 0`); err != nil {
					return fmt.Errorf("add words_seen column: %w", err)
				}
			}
			return nil
		},
	})
}
