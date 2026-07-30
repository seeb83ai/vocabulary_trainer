package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 64,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'voice_unavailable'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check voice_unavailable column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN voice_unavailable INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add voice_unavailable column: %w", err)
				}
			}
			return nil
		},
	})
}
