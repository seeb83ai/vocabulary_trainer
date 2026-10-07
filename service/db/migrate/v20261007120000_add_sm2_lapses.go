package migrate

import (
	"database/sql"
	"fmt"
)

// v20261007120000: sm2_progress.lapses counts failed reviews (a wrong first
// answer of the day on a due word that left the New bucket);
// consecutive_lapses counts them in a row and drives the leech warning.
func init() {
	register(migration{
		version: 20261007120000,
		fn: func(db *sql.DB) error {
			for _, name := range []string{"lapses", "consecutive_lapses"} {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sm2_progress') WHERE name = ?`, name).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", name, err)
				}
				if count == 0 {
					if _, err := db.Exec(`ALTER TABLE sm2_progress ADD COLUMN ` + name + ` INTEGER NOT NULL DEFAULT 0`); err != nil {
						return fmt.Errorf("add %s column: %w", name, err)
					}
				}
			}
			return nil
		},
	})
}
