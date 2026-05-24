package migrate

func init() {
	register(migration{
		version: 45,
		sql: `CREATE TABLE IF NOT EXISTS audit_log (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER NOT NULL DEFAULT 0,
			action     TEXT NOT NULL,
			ip_address TEXT NOT NULL DEFAULT '',
			detail     TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_audit_log_user_created
			ON audit_log (user_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_audit_log_ip_created
			ON audit_log (ip_address, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_audit_log_action_created
			ON audit_log (action, created_at DESC);`,
	})
}
