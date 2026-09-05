package migrate

import (
	"database/sql"
	"fmt"
)

// fixStuckNewBucketWordsV20260905120000 repairs rows left in an inconsistent
// state by a bug in the match-game answer handler (issue #398): it used to
// call sm2.Update directly regardless of learning_new_word, applying the full
// graduated SM-2 algorithm to words still in the new-word introduction phase.
// UpdateLearning never sets interval_days above its schema default of 1
// except at graduation (where it resets it back to 1), so a
// learning_new_word=1 row with interval_days > 1 can only have gotten there
// via that bug — its easiness/interval_days/due_date are already legitimate
// SM-2 state computed by sm2.Update, so the fix is just to stop mislabeling
// the row as still-new; every other column is left untouched.
func fixStuckNewBucketWordsV20260905120000(db *sql.DB) error {
	if _, err := db.Exec(`UPDATE sm2_progress SET learning_new_word = 0
		WHERE learning_new_word = 1 AND interval_days > 1`); err != nil {
		return fmt.Errorf("fix stuck new-bucket words: %w", err)
	}
	return nil
}

func init() {
	register(migration{
		version: 20260905120000,
		fn:      fixStuckNewBucketWordsV20260905120000,
	})
}
