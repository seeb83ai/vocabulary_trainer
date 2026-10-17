package migrate

// v20261017064855: hmm_stats — mnemonic scene answers per user and day, so
// the Train session bar counts them as done (issue #583).
func init() {
	register(migration{
		version: 20261017064855,
		sql: `
CREATE TABLE IF NOT EXISTS hmm_stats (
  user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date     TEXT    NOT NULL,
  correct  INTEGER NOT NULL DEFAULT 0,
  wrong    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, date)
);
`,
	})
}
