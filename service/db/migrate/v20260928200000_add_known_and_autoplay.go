package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260928200000,
		fn: func(db *sql.DB) error {
			cols := []struct {
				table string
				name  string
				def   string
			}{
				{"sm2_progress", "is_known", "INTEGER NOT NULL DEFAULT 0"},
				{"user_settings", "autoplay_always", "INTEGER NOT NULL DEFAULT 0"},
			}
			for _, c := range cols {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('`+c.table+`') WHERE name = ?`, c.name).Scan(&count); err != nil {
					return fmt.Errorf("check %s.%s column: %w", c.table, c.name, err)
				}
				if count == 0 {
					if _, err := db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.name + ` ` + c.def); err != nil {
						return fmt.Errorf("add %s.%s column: %w", c.table, c.name, err)
					}
				}
			}
			return nil
		},
	})
}
