package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 6,
		fn: func(db *sql.DB) error {
			cols := []struct{ name, def string }{
				{"bucket_new", "INTEGER NOT NULL DEFAULT 0"},
				{"bucket_struggling", "INTEGER NOT NULL DEFAULT 0"},
				{"bucket_learning", "INTEGER NOT NULL DEFAULT 0"},
				{"bucket_practicing", "INTEGER NOT NULL DEFAULT 0"},
				{"bucket_mastered", "INTEGER NOT NULL DEFAULT 0"},
			}
			for _, c := range cols {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('daily_stats') WHERE name = ?`, c.name).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", c.name, err)
				}
				if count == 0 {
					if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE daily_stats ADD COLUMN %s %s`, c.name, c.def)); err != nil {
						return fmt.Errorf("add %s column: %w", c.name, err)
					}
				}
			}
			return nil
		},
	})
}
