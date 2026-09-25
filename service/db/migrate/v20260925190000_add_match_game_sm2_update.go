package migrate

import (
	"database/sql"
	"fmt"
)

// v20260925190000: issue #472 — add the "match game progress" gamification
// setting. Controls which match-the-pairs answers change word/component
// progress: "never", "wrong_only", or "always" (the pre-existing behaviour).
// Defaults to "always" for every user, so existing behaviour does not change.
func init() {
	register(migration{
		version: 20260925190000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'match_game_sm2_update'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check user_settings.match_game_sm2_update column: %w", err)
			}
			if count > 0 {
				return nil
			}
			if _, err := db.Exec(
				`ALTER TABLE user_settings ADD COLUMN match_game_sm2_update TEXT NOT NULL DEFAULT 'always'`,
			); err != nil {
				return fmt.Errorf("add user_settings.match_game_sm2_update column: %w", err)
			}
			return nil
		},
	})
}
