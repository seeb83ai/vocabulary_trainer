package migrate

import (
	"database/sql"
	"fmt"
)

// v20260826220458: issue #349 — add the "hide pinyin from bucket" gamification
// setting. Lets a user choose the minimum SM-2 bucket (tierFilter/TIERS key:
// new, 0-49, 50-69, 70-84, 85-100) at and above which the match-the-pairs
// mini-game stops showing the pinyin hint under a word's tile. Defaults to
// "70-84" (Practicing) for every user, existing and new, per the issue.
func init() {
	register(migration{
		version: 20260826220458,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'gamification_hide_pinyin_from_bucket'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check user_settings.gamification_hide_pinyin_from_bucket column: %w", err)
			}
			if count > 0 {
				return nil
			}
			if _, err := db.Exec(
				`ALTER TABLE user_settings ADD COLUMN gamification_hide_pinyin_from_bucket TEXT NOT NULL DEFAULT '70-84'`,
			); err != nil {
				return fmt.Errorf("add user_settings.gamification_hide_pinyin_from_bucket column: %w", err)
			}
			return nil
		},
	})
}
