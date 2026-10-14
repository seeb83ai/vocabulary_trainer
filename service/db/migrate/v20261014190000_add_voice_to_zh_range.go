package migrate

import (
	"database/sql"
	"fmt"
)

// v20261014190000: user_settings.random_mode_range_voice_to_zh — the bucket
// range for the Voice → Chinese mode in Random/Cycle (issue #571). ” uses
// the built-in default (Practicing–Mastered).
func init() {
	register(migration{
		version: 20261014190000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'random_mode_range_voice_to_zh'`).Scan(&count); err != nil {
				return fmt.Errorf("check random_mode_range_voice_to_zh column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN random_mode_range_voice_to_zh TEXT NOT NULL DEFAULT ''`); err != nil {
					return fmt.Errorf("add random_mode_range_voice_to_zh column: %w", err)
				}
			}
			return nil
		},
	})
}
