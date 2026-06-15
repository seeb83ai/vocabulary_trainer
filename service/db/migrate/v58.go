package migrate

import (
	"database/sql"
	"fmt"
)

// v58 consolidates the duplicate first-seen timestamps on sm2_progress. The
// table carried both first_seen_date (a DATE) and first_seen_at (a DATETIME),
// updated in lockstep, which was a maintenance hazard. We keep the more capable
// first_seen_at (needed for the sub-day new-word cooldown) and derive the date
// from it where required, then drop first_seen_date and its index.
func init() {
	register(migration{
		version: 58,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('sm2_progress') WHERE name = 'first_seen_date'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check first_seen_date column: %w", err)
			}
			if count == 0 {
				return nil // already consolidated
			}

			// Backfill first_seen_at for any row that only had first_seen_date set
			// (e.g. words acknowledged via the old AcknowledgeRandomWords path).
			if _, err := db.Exec(
				`UPDATE sm2_progress
				 SET first_seen_at = first_seen_date || ' 00:00:00'
				 WHERE first_seen_at IS NULL AND first_seen_date IS NOT NULL`,
			); err != nil {
				return fmt.Errorf("backfill first_seen_at: %w", err)
			}

			// The column is indexed; drop the index before dropping the column.
			if _, err := db.Exec(`DROP INDEX IF EXISTS idx_sm2_first_seen`); err != nil {
				return fmt.Errorf("drop idx_sm2_first_seen: %w", err)
			}
			if _, err := db.Exec(`ALTER TABLE sm2_progress DROP COLUMN first_seen_date`); err != nil {
				return fmt.Errorf("drop first_seen_date column: %w", err)
			}

			// Keep the new-word filters fast with an index on the surviving column.
			if _, err := db.Exec(
				`CREATE INDEX IF NOT EXISTS idx_sm2_first_seen_at ON sm2_progress(first_seen_at)`,
			); err != nil {
				return fmt.Errorf("create idx_sm2_first_seen_at: %w", err)
			}
			return nil
		},
	})
}
