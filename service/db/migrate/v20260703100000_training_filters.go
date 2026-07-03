package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260703100000,
		fn: func(db *sql.DB) error {
			cols := []struct {
				name, definition string
			}{
				{"train_mode", "TEXT NOT NULL DEFAULT 'random'"},
				{"train_bucket", "TEXT NOT NULL DEFAULT ''"},
				{"train_langs", "TEXT NOT NULL DEFAULT '[\"en\"]'"},
				{"train_mnemonics", "INTEGER NOT NULL DEFAULT 1"},
				{"train_components", "INTEGER NOT NULL DEFAULT 1"},
				{"train_tags", "TEXT NOT NULL DEFAULT '[]'"},
			}
			for _, c := range cols {
				var count int
				if err := db.QueryRow(
					`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = ?`, c.name,
				).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", c.name, err)
				}
				if count == 0 {
					if _, err := db.Exec(
						`ALTER TABLE user_settings ADD COLUMN ` + c.name + ` ` + c.definition,
					); err != nil {
						return fmt.Errorf("add %s column: %w", c.name, err)
					}
				}
			}
			return nil
		},
	})
}
