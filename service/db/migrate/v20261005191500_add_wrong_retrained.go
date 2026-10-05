package migrate

import (
	"database/sql"
	"fmt"
)

// v20261005191500: issue #537 — sm2_progress.wrong_retrained is 1 after
// "Train them again now" on the all-done screen and 0 again after the next
// wrong answer. Flagged words are not in the "today's mistakes" list.
func init() {
	register(migration{
		version: 20261005191500,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('sm2_progress') WHERE name = 'wrong_retrained'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check wrong_retrained column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(
					`ALTER TABLE sm2_progress ADD COLUMN wrong_retrained INTEGER NOT NULL DEFAULT 0`,
				); err != nil {
					return fmt.Errorf("add wrong_retrained column: %w", err)
				}
			}
			return nil
		},
	})
}
