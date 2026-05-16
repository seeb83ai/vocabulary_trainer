package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 45,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'cycle_sequence'`).Scan(&count); err != nil {
				return fmt.Errorf("check cycle_sequence column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN cycle_sequence TEXT NOT NULL DEFAULT 'zh_pinyin_to_transl,transl_to_zh,zh_to_transl'`); err != nil {
					return fmt.Errorf("add cycle_sequence column: %w", err)
				}
			}
			return nil
		},
	})
}
