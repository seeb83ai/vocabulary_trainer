package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260703100000,
		fn: func(db *sql.DB) error {
			// Read all existing columns in one query to minimise round-trips.
			rows, err := db.Query(`SELECT name FROM pragma_table_info('user_settings')`)
			if err != nil {
				return fmt.Errorf("check user_settings columns: %w", err)
			}
			existing := make(map[string]bool)
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err != nil {
					rows.Close()
					return fmt.Errorf("scan column name: %w", err)
				}
				existing[name] = true
			}
			rows.Close()

			cols := []struct{ name, def string }{
				{"train_mode", "TEXT NOT NULL DEFAULT 'random'"},
				{"train_bucket", "TEXT NOT NULL DEFAULT ''"},
				{"train_langs", `TEXT NOT NULL DEFAULT '["en"]'`},
				{"train_mnemonics", "INTEGER NOT NULL DEFAULT 1"},
				{"train_components", "INTEGER NOT NULL DEFAULT 1"},
				{"train_tags", "TEXT NOT NULL DEFAULT '[]'"},
			}
			for _, c := range cols {
				if existing[c.name] {
					continue
				}
				if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN ` + c.name + ` ` + c.def); err != nil {
					return fmt.Errorf("add %s column: %w", c.name, err)
				}
			}
			return nil
		},
	})
}
