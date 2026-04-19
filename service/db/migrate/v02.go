package migrate

func init() {
	register(migration{
		version: 2,
		sql: `
CREATE TABLE IF NOT EXISTS daily_stats (
  date            TEXT PRIMARY KEY,
  attempts        INTEGER NOT NULL DEFAULT 0,
  mistakes        INTEGER NOT NULL DEFAULT 0,
  words_known     INTEGER NOT NULL DEFAULT 0,
  new_words       INTEGER NOT NULL DEFAULT 0,
  correct_streak  INTEGER NOT NULL DEFAULT 0,
  current_streak  INTEGER NOT NULL DEFAULT 0
);
`,
	})
}
