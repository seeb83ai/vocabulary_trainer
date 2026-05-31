package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 51,
		fn: func(db *sql.DB) error {
			cols := []struct{ name string }{
				{"new_word_require_zh"},
				{"new_word_require_trans"},
			}
			for _, c := range cols {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = ?`, c.name).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", c.name, err)
				}
				if count == 0 {
					if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN ` + c.name + ` INTEGER NOT NULL DEFAULT 1`); err != nil {
						return fmt.Errorf("add %s column: %w", c.name, err)
					}
				}
			}
			return nil
		},
	})
}
