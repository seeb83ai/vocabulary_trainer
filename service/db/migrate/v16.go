package migrate

func init() {
	register(migration{
		// v16: add pinyin_daily_stats table to track per-day pinyin training stats.
		version: 16,
		sql: `
CREATE TABLE IF NOT EXISTS pinyin_daily_stats (
  date        TEXT PRIMARY KEY,
  attempts    INTEGER NOT NULL DEFAULT 0,
  mistakes    INTEGER NOT NULL DEFAULT 0,
  sounds_seen INTEGER NOT NULL DEFAULT 0
);
`,
	})
}
