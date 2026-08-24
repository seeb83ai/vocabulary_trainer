package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260813083053,
		fn: func(db *sql.DB) error {
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

			cols := []string{
				"random_mode_range_transl_to_zh",
				"random_mode_range_zh_to_transl",
				"random_mode_range_zh_pinyin_to_transl",
				"random_mode_range_zh_to_transl_no_sound",
				"random_mode_range_voice_to_transl",
			}
			for _, c := range cols {
				if existing[c] {
					continue
				}
				if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN ` + c + ` TEXT NOT NULL DEFAULT ''`); err != nil {
					return fmt.Errorf("add %s column: %w", c, err)
				}
			}
			return nil
		},
	})
}
