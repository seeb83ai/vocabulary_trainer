package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v17: add per-tone correct/wrong columns to pinyin_daily_stats.
		version: 17,
		fn: func(db *sql.DB) error {
			cols := []struct{ name, def string }{
				{"tone1_correct", "INTEGER NOT NULL DEFAULT 0"},
				{"tone1_wrong", "INTEGER NOT NULL DEFAULT 0"},
				{"tone2_correct", "INTEGER NOT NULL DEFAULT 0"},
				{"tone2_wrong", "INTEGER NOT NULL DEFAULT 0"},
				{"tone3_correct", "INTEGER NOT NULL DEFAULT 0"},
				{"tone3_wrong", "INTEGER NOT NULL DEFAULT 0"},
				{"tone4_correct", "INTEGER NOT NULL DEFAULT 0"},
				{"tone4_wrong", "INTEGER NOT NULL DEFAULT 0"},
				{"tone5_correct", "INTEGER NOT NULL DEFAULT 0"},
				{"tone5_wrong", "INTEGER NOT NULL DEFAULT 0"},
			}
			for _, c := range cols {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('pinyin_daily_stats') WHERE name = ?`, c.name).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", c.name, err)
				}
				if count == 0 {
					if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE pinyin_daily_stats ADD COLUMN %s %s`, c.name, c.def)); err != nil {
						return fmt.Errorf("add %s column: %w", c.name, err)
					}
				}
			}
			return nil
		},
	})
}
