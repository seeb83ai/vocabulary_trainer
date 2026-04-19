package migrate

func init() {
	register(migration{
		// Clean up words that were stamped by the now-removed GetNextCard stamp()
		// but never acknowledged by the user. These words have first_seen_date set
		// despite total_attempts = 0, which made them count in CountLearningNewWords
		// and block new word introductions. Resetting first_seen_date to NULL returns
		// them to the unseen pool so they can be properly introduced.
		version: 9,
		sql: `UPDATE sm2_progress
		      SET first_seen_date = NULL
		      WHERE total_attempts = 0
		        AND first_seen_date IS NOT NULL;`,
	})
}
