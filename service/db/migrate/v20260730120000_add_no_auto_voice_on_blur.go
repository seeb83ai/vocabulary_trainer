package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260730120000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'no_auto_voice_on_blur'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check no_auto_voice_on_blur column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN no_auto_voice_on_blur INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add no_auto_voice_on_blur column: %w", err)
				}
			}
			return nil
		},
	})
}
