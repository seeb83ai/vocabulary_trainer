package migrate

import (
	"database/sql"
	"fmt"
)

// v20260813090000: issue #288 — add more gamification match-game modes.
//
// user_settings gets 4 individual on/off toggles, one per match-game mode
// (mismatch is the pre-existing mode; newest/hardest/last_mistakes are new).
// All default to enabled (1) for every user, existing and new — an explicit
// product decision, not an oversight.
//
// sm2_progress gets two nullable per-word attempt timestamps: last_attempt_at
// (stamped on every answer, right or wrong) and last_wrong_at (stamped only on
// a wrong answer). These drive the "hardest words" and "last mistakes" game
// modes' repeat-avoidance rules and are independent bookkeeping, not part of
// the SM-2 algorithm itself.
//
// word_game_shown mirrors confusion_pairs' last_shown_in_game idiom but keyed
// per (user, word, game_mode) so the hardest/last-mistakes modes can each
// suppress a word until their own qualifying event happens again. The
// "newest words" mode needs no such table — see GetNewestWordsForGame.
func init() {
	register(migration{
		version: 20260813090000,
		fn: func(db *sql.DB) error {
			addColumnIfMissing := func(table, column, ddl string) error {
				var count int
				if err := db.QueryRow(
					`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column,
				).Scan(&count); err != nil {
					return fmt.Errorf("check %s.%s column: %w", table, column, err)
				}
				if count > 0 {
					return nil
				}
				if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + ddl); err != nil {
					return fmt.Errorf("add %s.%s column: %w", table, column, err)
				}
				return nil
			}

			for _, c := range []struct{ column, ddl string }{
				{"game_mode_mismatch", "game_mode_mismatch INTEGER NOT NULL DEFAULT 1"},
				{"game_mode_newest", "game_mode_newest INTEGER NOT NULL DEFAULT 1"},
				{"game_mode_hardest", "game_mode_hardest INTEGER NOT NULL DEFAULT 1"},
				{"game_mode_last_mistakes", "game_mode_last_mistakes INTEGER NOT NULL DEFAULT 1"},
			} {
				if err := addColumnIfMissing("user_settings", c.column, c.ddl); err != nil {
					return err
				}
			}

			for _, c := range []struct{ column, ddl string }{
				{"last_attempt_at", "last_attempt_at TEXT"},
				{"last_wrong_at", "last_wrong_at TEXT"},
			} {
				if err := addColumnIfMissing("sm2_progress", c.column, c.ddl); err != nil {
					return err
				}
			}

			if _, err := db.Exec(`
				CREATE TABLE IF NOT EXISTS word_game_shown (
				  user_id            INTEGER  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  word_id            INTEGER  NOT NULL REFERENCES words(id) ON DELETE CASCADE,
				  game_mode          TEXT     NOT NULL,
				  last_shown_in_game TEXT,
				  PRIMARY KEY (user_id, word_id, game_mode)
				)`); err != nil {
				return fmt.Errorf("create word_game_shown: %w", err)
			}
			return nil
		},
	})
}
