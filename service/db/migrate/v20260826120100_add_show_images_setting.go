package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260826120100,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'show_images_with_chinese_text'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check show_images_with_chinese_text column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN show_images_with_chinese_text INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add show_images_with_chinese_text column: %w", err)
				}
			}
			return nil
		},
	})
}
