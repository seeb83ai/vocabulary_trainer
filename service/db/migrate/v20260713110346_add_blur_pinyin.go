package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260713110346,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'blur_pinyin'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check blur_pinyin column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN blur_pinyin INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add blur_pinyin column: %w", err)
				}
			}
			return nil
		},
	})
}
