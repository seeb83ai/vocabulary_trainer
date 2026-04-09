package migrate

import (
	"database/sql"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func init() {
	register(migration{
		// v20: create users table and seed the two initial users.
		// admin@elygor.de (id=1) is the template user whose vocabulary serves as
		// the importable baseline for new users. me@elygor.de (id=2) is the
		// original single user who owns all pre-migration data.
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
			adminHash, err := bcrypt.GenerateFromPassword([]byte("I am the admin"), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hash admin password: %w", err)
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO users (email, password_hash) VALUES (?, ?)`,
				"admin@elygor.de", string(adminHash),
			); err != nil {
				return fmt.Errorf("seed admin user: %w", err)
			}

			meHash, err := bcrypt.GenerateFromPassword([]byte("I learn zh"), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hash initial user password: %w", err)
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO users (email, password_hash) VALUES (?, ?)`,
				"me@elygor.de", string(meHash),
			); err != nil {
				return fmt.Errorf("seed initial user: %w", err)
			}
			return nil
		},
	})
}
