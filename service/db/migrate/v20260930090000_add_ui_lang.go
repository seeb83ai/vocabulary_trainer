package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260930090000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'ui_lang'`).Scan(&count); err != nil {
				return fmt.Errorf("check user_settings.ui_lang column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN ui_lang TEXT NOT NULL DEFAULT ''`); err != nil {
					return fmt.Errorf("add user_settings.ui_lang column: %w", err)
				}
			}
			return nil
		},
	})
}
