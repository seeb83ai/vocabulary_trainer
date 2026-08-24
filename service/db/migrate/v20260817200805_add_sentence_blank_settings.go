package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260817200805,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'sentence_blank_enabled'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check sentence_blank_enabled column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN sentence_blank_enabled INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add sentence_blank_enabled column: %w", err)
				}
			}
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'sentence_blank_ratio'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check sentence_blank_ratio column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN sentence_blank_ratio INTEGER NOT NULL DEFAULT 20`,
				); err != nil {
					return fmt.Errorf("add sentence_blank_ratio column: %w", err)
				}
			}
			return nil
		},
	})
}
