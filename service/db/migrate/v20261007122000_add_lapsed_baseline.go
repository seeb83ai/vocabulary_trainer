package migrate

import (
	"database/sql"
	"fmt"
)

// v20261007122000: lapsed-words baseline — new words pause while at least
// baseline_lapsed_value words failed their last review. On by default.
func init() {
	register(migration{
		version: 20261007122000,
		fn: func(db *sql.DB) error {
			userSettingsCols := []struct {
				name string
				def  string
			}{
				{"baseline_lapsed_enabled", "INTEGER NOT NULL DEFAULT 1"},
				{"baseline_lapsed_value", "INTEGER NOT NULL DEFAULT 10"},
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
