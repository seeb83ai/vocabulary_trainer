package migrate

// Server-side sessions: the session cookie holds a random token and the
// table stores only its SHA-256, so logout and password change can revoke
// sessions. Tokens minted by the old signed-cookie scheme are not in the
// table, so every user logs in once after this migration.
func init() {
	register(migration{
		version: 20261106120000,
		sql: `
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
`,
	})
}
