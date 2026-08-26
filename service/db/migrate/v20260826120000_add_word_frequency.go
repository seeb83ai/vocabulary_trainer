package migrate

func init() {
	register(migration{
		version: 20260826120000,
		sql: `
CREATE TABLE IF NOT EXISTS word_frequency (
  word TEXT    PRIMARY KEY,
  rank INTEGER NOT NULL
);
`,
	})
}
