package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20261004065000,
		fn: func(db *sql.DB) error {
			cols := []struct {
				name string
				def  string
			}{
				{"autofocus_desktop", "INTEGER NOT NULL DEFAULT 1"},
				{"autofocus_mobile", "INTEGER NOT NULL DEFAULT 0"},
			}
			for _, c := range cols {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = ?`, c.name).Scan(&count); err != nil {
					return fmt.Errorf("check user_settings.%s column: %w", c.name, err)
				}
				if count == 0 {
					if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN ` + c.name + ` ` + c.def); err != nil {
						return fmt.Errorf("add user_settings.%s column: %w", c.name, err)
					}
				}
			}
			return nil
		},
	})
}
