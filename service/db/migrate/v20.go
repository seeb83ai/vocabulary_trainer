package migrate

import (
	"database/sql"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func init() {
	register(migration{
		// v20: create users table and seed the initial user.
		version: 20,
		sql: `
CREATE TABLE IF NOT EXISTS users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT    NOT NULL UNIQUE,
  password_hash TEXT    NOT NULL,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
`,
		fn: func(db *sql.DB) error {
			hash, err := bcrypt.GenerateFromPassword([]byte("I learn zh"), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hash initial user password: %w", err)
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO users (email, password_hash) VALUES (?, ?)`,
				"me@elygor.de", string(hash),
			); err != nil {
				return fmt.Errorf("seed initial user: %w", err)
			}
			return nil
		},
	})
}
