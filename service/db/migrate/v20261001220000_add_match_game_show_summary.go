package migrate

import (
	"database/sql"
	"fmt"
)

// v20261001220000: add the "show round summary" gamification setting. When
// off, the match game skips its "Round complete" screen. Defaults to on so
// existing behaviour does not change.
func init() {
	register(migration{
		version: 20261001220000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'match_game_show_summary'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check user_settings.match_game_show_summary column: %w", err)
			}
			if count > 0 {
				return nil
			}
			if _, err := db.Exec(
				`ALTER TABLE user_settings ADD COLUMN match_game_show_summary INTEGER NOT NULL DEFAULT 1`,
			); err != nil {
				return fmt.Errorf("add user_settings.match_game_show_summary column: %w", err)
			}
			return nil
		},
	})
}
