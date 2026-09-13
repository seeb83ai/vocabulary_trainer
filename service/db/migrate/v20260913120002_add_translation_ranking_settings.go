package migrate

import "database/sql"

// v20260913120002 adds per-user settings controlling translation ranking
// during training: an opt-in toggle (default off), how many translations to
// show per language (default 3), and whether unranked (no frequency-list
// match) translations count as low-priority (default: always shown).
func init() {
	register(migration{
		version: 20260913120002,
		fn: func(db *sql.DB) error {
			cols := []struct {
				name string
				ddl  string
			}{
				{"translation_ranking_enabled", `ALTER TABLE user_settings ADD COLUMN translation_ranking_enabled INTEGER NOT NULL DEFAULT 0`},
				{"max_translations_shown", `ALTER TABLE user_settings ADD COLUMN max_translations_shown INTEGER NOT NULL DEFAULT 3`},
				{"translation_hide_unranked", `ALTER TABLE user_settings ADD COLUMN translation_hide_unranked INTEGER NOT NULL DEFAULT 0`},
			}
			for _, c := range cols {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = ?`, c.name).Scan(&count); err != nil {
					return err
				}
				if count == 0 {
					if _, err := db.Exec(c.ddl); err != nil {
						return err
					}
				}
			}
			return nil
		},
	})
}
