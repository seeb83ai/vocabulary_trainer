package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260704120000,
		fn: func(db *sql.DB) error {
			for _, col := range []struct {
				name string
				def  string
			}{
				{"quiz_mode", "TEXT NOT NULL DEFAULT 'random'"},
				{"quiz_bucket", "TEXT NOT NULL DEFAULT ''"},
				{"quiz_langs", `TEXT NOT NULL DEFAULT '["en"]'`},
				{"quiz_tags", "TEXT NOT NULL DEFAULT '[]'"},
				{"quiz_mnemonics", "INTEGER NOT NULL DEFAULT 1"},
				{"quiz_components", "INTEGER NOT NULL DEFAULT 1"},
			} {
				var count int
				if err := db.QueryRow(
					`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = ?`, col.name,
				).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", col.name, err)
				}
				if count == 0 {
					if _, err := db.Exec(
						`ALTER TABLE user_settings ADD COLUMN ` + col.name + ` ` + col.def,
					); err != nil {
						return fmt.Errorf("add %s column: %w", col.name, err)
					}
				}
			}
			return nil
		},
	})
}
