package migrate

func init() {
	register(migration{
		version: 10,
		sql: `CREATE TABLE IF NOT EXISTS hanzi_decomposition (
			character     TEXT PRIMARY KEY,
			definition    TEXT,
			radical       TEXT,
			decomposition TEXT,
			etymology     TEXT
		);`,
	})
}
