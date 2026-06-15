package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 58,
		fn: func(db *sql.DB) error {
			for _, col := range []struct {
				name string
				def  string
			}{
				{"gamification_enabled", "INTEGER NOT NULL DEFAULT 0"},
				{"gamification_frequency", "INTEGER NOT NULL DEFAULT 5"},
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
