package migrate

// v20261004130000 deletes the library user's (user_id=1) progress rows. The
// library is never quizzed; learners reference its words (ADR-0005).
func init() {
	register(migration{
		version: 20261004130000,
		sql:     `DELETE FROM sm2_progress WHERE word_id IN (SELECT id FROM words WHERE user_id = 1)`,
	})
}
