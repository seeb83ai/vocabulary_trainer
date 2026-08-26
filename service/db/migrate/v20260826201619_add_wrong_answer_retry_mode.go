package migrate

import (
	"database/sql"
	"fmt"
)

// v20260826201619 replaces the all-or-nothing retype_on_wrong boolean with a
// 3-way wrong_answer_retry_mode ("off"/"matched"/"both"), so a wrong answer
// can require retyping only the field(s) the current card direction actually
// tested (e.g. just the translation for a zh_to_transl card) instead of
// always both the Chinese word and the translation. Existing users who had
// retype_on_wrong enabled are migrated to "both" to preserve their behaviour.
func init() {
	register(migration{
		version: 20260826201619,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'wrong_answer_retry_mode'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check wrong_answer_retry_mode column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE user_settings ADD COLUMN wrong_answer_retry_mode TEXT NOT NULL DEFAULT 'off'`,
				); err != nil {
					return fmt.Errorf("add wrong_answer_retry_mode column: %w", err)
				}
			}

			var oldCount int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'retype_on_wrong'`,
			).Scan(&oldCount); err != nil {
				return fmt.Errorf("check retype_on_wrong column: %w", err)
			}
			if oldCount > 0 {
				if _, err := db.Exec(
					`UPDATE user_settings SET wrong_answer_retry_mode = 'both' WHERE retype_on_wrong = 1`,
				); err != nil {
					return fmt.Errorf("backfill wrong_answer_retry_mode: %w", err)
				}
				if _, err := db.Exec(
					`ALTER TABLE user_settings DROP COLUMN retype_on_wrong`,
				); err != nil {
					return fmt.Errorf("drop retype_on_wrong column: %w", err)
				}
			}
			return nil
		},
	})
}
