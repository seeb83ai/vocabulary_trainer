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
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'no_autoplay_if_pinyin_hidden'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check no_autoplay_if_pinyin_hidden column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN no_autoplay_if_pinyin_hidden INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add no_autoplay_if_pinyin_hidden column: %w", err)
				}
			}
			return nil
		},
	})
}
