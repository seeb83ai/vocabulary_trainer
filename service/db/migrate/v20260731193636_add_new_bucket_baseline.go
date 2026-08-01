package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260731193636,
		fn: func(db *sql.DB) error {
			userSettingsCols := []struct {
				name string
				def  string
			}{
				{"baseline_new_bucket_enabled", "INTEGER NOT NULL DEFAULT 0"},
				{"baseline_new_bucket_value", "INTEGER NOT NULL DEFAULT 10"},
			}
			for _, c := range userSettingsCols {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = ?`, c.name).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", c.name, err)
				}
				if count == 0 {
					if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN ` + c.name + ` ` + c.def); err != nil {
						return fmt.Errorf("add %s column: %w", c.name, err)
					}
				}
			}
			return nil
		},
	})
}
