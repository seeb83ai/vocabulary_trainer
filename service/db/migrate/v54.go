package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 54,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('component_progress') WHERE name = 'prev_state'`).Scan(&count); err != nil {
				return fmt.Errorf("check prev_state column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE component_progress ADD COLUMN prev_state TEXT`); err != nil {
					return fmt.Errorf("add prev_state column: %w", err)
				}
			}
			return nil
		},
	})
}
