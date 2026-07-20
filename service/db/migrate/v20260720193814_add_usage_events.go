package migrate

func init() {
	register(migration{
		version: 20260720193814,
		sql: `CREATE TABLE IF NOT EXISTS usage_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			last_seen TEXT NOT NULL,
			UNIQUE(user_id, name)
		)`,
	})
}
