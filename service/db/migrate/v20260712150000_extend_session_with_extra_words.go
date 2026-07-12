package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260712150000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'extend_session_with_extra_words'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check extend_session_with_extra_words column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN extend_session_with_extra_words INTEGER NOT NULL DEFAULT 1`,
				); err != nil {
					return fmt.Errorf("add extend_session_with_extra_words column: %w", err)
				}
			}
			return nil
		},
	})
}
