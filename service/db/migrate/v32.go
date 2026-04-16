package migrate

import (
	"database/sql"
)

func init() {
	register(migration{
		// v32: add email verification columns to users table.
		// Pre-existing users are marked as already verified since they were
		// provisioned manually via migration prompts or env vars.
		version: 32,
		fn: func(db *sql.DB) error {
			for _, stmt := range []string{
				`ALTER TABLE users ADD COLUMN email_verified INTEGER NOT NULL DEFAULT 0`,
				`ALTER TABLE users ADD COLUMN verification_token TEXT`,
				`ALTER TABLE users ADD COLUMN verification_expires_at DATETIME`,
			} {
				if _, err := db.Exec(stmt); err != nil {
					// Ignore "duplicate column" errors — idempotent.
					if sqliteIsDuplicateColumn(err) {
						continue
					}
					return err
				}
			}
			// Mark all existing users as verified.
			_, err := db.Exec(`UPDATE users SET email_verified = 1 WHERE email_verified = 0`)
			return err
		},
	})
}

// sqliteIsDuplicateColumn reports whether err is a SQLite "duplicate column name" error.
func sqliteIsDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return len(s) >= 21 && s[:21] == "duplicate column name"
}
