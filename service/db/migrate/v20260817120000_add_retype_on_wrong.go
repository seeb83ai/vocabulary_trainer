package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260817120000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'retype_on_wrong'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check retype_on_wrong column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN retype_on_wrong INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add retype_on_wrong column: %w", err)
				}
			}
			return nil
		},
	})
}
