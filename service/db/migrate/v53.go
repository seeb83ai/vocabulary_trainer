package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 53,
		fn: func(db *sql.DB) error {
			// Add cooldown setting to user_settings.
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'new_word_cooldown_minutes'`).Scan(&count); err != nil {
				return fmt.Errorf("check new_word_cooldown_minutes column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN new_word_cooldown_minutes INTEGER NOT NULL DEFAULT 1`); err != nil {
					return fmt.Errorf("add new_word_cooldown_minutes column: %w", err)
				}
			}

			// Add first_seen_at (exact datetime) to sm2_progress for cooldown tracking.
			var count2 int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sm2_progress') WHERE name = 'first_seen_at'`).Scan(&count2); err != nil {
				return fmt.Errorf("check first_seen_at column: %w", err)
			}
			if count2 == 0 {
				if _, err := db.Exec(`ALTER TABLE sm2_progress ADD COLUMN first_seen_at TEXT`); err != nil {
					return fmt.Errorf("add first_seen_at column: %w", err)
				}
			}

			return nil
		},
	})
}
