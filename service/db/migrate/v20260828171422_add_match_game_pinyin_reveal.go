package migrate

import (
	"database/sql"
	"fmt"
)

// v20260828171422: issue #375 — add the "match game pinyin reveal" gamification
// setting. Controls when the match-the-pairs mini-game shows a word tile's
// pinyin hint under its Chinese text: "off" (never), "always" (shown from the
// start, the pre-existing behaviour), or "after_correct" (hidden until the
// pair is matched correctly). Defaults to "always" for every user, existing
// and new, so this setting doesn't change existing behaviour by default.
func init() {
	register(migration{
		version: 20260828171422,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'match_game_pinyin_reveal'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check user_settings.match_game_pinyin_reveal column: %w", err)
			}
			if count > 0 {
				return nil
			}
			if _, err := db.Exec(
				`ALTER TABLE user_settings ADD COLUMN match_game_pinyin_reveal TEXT NOT NULL DEFAULT 'always'`,
			); err != nil {
				return fmt.Errorf("add user_settings.match_game_pinyin_reveal column: %w", err)
			}
			return nil
		},
	})
}
