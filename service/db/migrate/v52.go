package migrate

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
)

func init() {
	register(migration{
		version: 52,
		fn: func(db *sql.DB) error {
			// Add daily learning columns to user_settings
			userSettingsCols := []struct {
				name         string
				def          string
			}{
				{"max_new_words_per_day", "INTEGER NOT NULL DEFAULT 5"},
				{"skip_new_words_visible", "INTEGER NOT NULL DEFAULT 1"},
				{"baseline_due_today_enabled", "INTEGER NOT NULL DEFAULT 0"},
				{"baseline_due_today_value", "INTEGER NOT NULL DEFAULT 20"},
				{"baseline_struggling_enabled", "INTEGER NOT NULL DEFAULT 0"},
				{"baseline_struggling_value", "INTEGER NOT NULL DEFAULT 10"},
				{"baseline_learning_enabled", "INTEGER NOT NULL DEFAULT 0"},
				{"baseline_learning_value", "INTEGER NOT NULL DEFAULT 20"},
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

			// Add due_at_day_start to daily_stats (-1 = snapshot not yet taken)
			var dsCount int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('daily_stats') WHERE name = 'due_at_day_start'`).Scan(&dsCount); err != nil {
				return fmt.Errorf("check due_at_day_start column: %w", err)
			}
			if dsCount == 0 {
				if _, err := db.Exec(`ALTER TABLE daily_stats ADD COLUMN due_at_day_start INTEGER NOT NULL DEFAULT -1`); err != nil {
					return fmt.Errorf("add due_at_day_start column: %w", err)
				}
			}

			// Apply MAX_NEW_WORDS env var as the default for all existing users.
			maxNew := 5
			if v := os.Getenv("MAX_NEW_WORDS"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n >= 1 {
					maxNew = n
				}
			}
			if _, err := db.Exec(`UPDATE user_settings SET max_new_words_per_day = ? WHERE max_new_words_per_day = 5`, maxNew); err != nil {
				return fmt.Errorf("apply MAX_NEW_WORDS default: %w", err)
			}

			return nil
		},
	})
}
