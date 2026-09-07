package migrate

import (
	"database/sql"
	"fmt"
)

// fixMatchGameIntervalV20260907130000 caps interval_days for rows where the
// stored interval exceeds the word's entire age (days since first_seen_at).
// This state can only arise from the match-game bug (issue #398): the handler
// used to call sm2.Update directly, allowing a single match-game session to
// jump a word from interval=1 to interval=3720 via repeated compounding, while
// legitimate SM-2 would take years to reach that figure.
//
// Fix: set interval_days = age_in_days and recompute due_date from last_attempt_at
// (or first_seen_at when last_attempt_at is NULL).  repetitions is left
// untouched — the user did review the word; only the timing was wrong.
func fixMatchGameIntervalV20260907130000(db *sql.DB) error {
	_, err := db.Exec(`
		UPDATE sm2_progress
		SET
			interval_days = CAST(JULIANDAY('now') - JULIANDAY(first_seen_at) AS INTEGER),
			due_date = datetime(
				COALESCE(last_attempt_at, first_seen_at),
				'+' || CAST(CAST(JULIANDAY('now') - JULIANDAY(first_seen_at) AS INTEGER) AS TEXT) || ' days'
			)
		WHERE first_seen_at IS NOT NULL
		  AND interval_days > CAST(JULIANDAY('now') - JULIANDAY(first_seen_at) AS INTEGER)`)
	if err != nil {
		return fmt.Errorf("fix match-game inflated interval: %w", err)
	}
	return nil
}

func init() {
	register(migration{
		version: 20260907130000,
		fn:      fixMatchGameIntervalV20260907130000,
	})
}
