package migrate

import (
	"database/sql"
	"fmt"
)

// fixMatchGameFirstSeenAtV20260907120000 repairs sm2_progress rows that have
// been answered (total_attempts > 0) but have first_seen_at = NULL. This
// state arises when a word is answered exclusively via the match-game handler,
// which calls UpdateSM2Progress (setting total_attempts, interval_days, etc.)
// but never writes first_seen_at. GetNextCard treats first_seen_at IS NULL as
// "unseen new word", so those words appear as new in every quiz session.
//
// Fix: set first_seen_at = COALESCE(last_attempt_at, CURRENT_TIMESTAMP).
// Words with total_attempts = 0 are genuinely unseen and are left alone.
func fixMatchGameFirstSeenAtV20260907120000(db *sql.DB) error {
	_, err := db.Exec(`UPDATE sm2_progress
		SET first_seen_at = COALESCE(last_attempt_at, CURRENT_TIMESTAMP)
		WHERE first_seen_at IS NULL AND total_attempts > 0`)
	if err != nil {
		return fmt.Errorf("fix match-game first_seen_at: %w", err)
	}
	return nil
}

func init() {
	register(migration{
		version: 20260907120000,
		fn:      fixMatchGameFirstSeenAtV20260907120000,
	})
}
