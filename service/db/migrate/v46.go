package migrate

import "database/sql"

func init() {
	register(migration{
		version: 46,
		fn: func(database *sql.DB) error {
			for _, stmt := range []string{
				`ALTER TABLE users ADD COLUMN failed_logins INTEGER NOT NULL DEFAULT 0`,
				`ALTER TABLE users ADD COLUMN lockout_until TEXT NOT NULL DEFAULT ''`,
			} {
				if _, err := database.Exec(stmt); err != nil {
					// Duplicate-column guard: ignore failures for columns that
					// already exist (re-running migrations on partial schemas).
					if !columnExistsErr(err) {
						return err
					}
				}
			}
			return nil
		},
	})
}

// columnExistsErr returns true when err looks like SQLite's "duplicate
// column name" error, so the migration is idempotent.
func columnExistsErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, frag := range []string{"duplicate column name", "already exists"} {
		if containsCI(s, frag) {
			return true
		}
	}
	return false
}

func containsCI(s, sub string) bool {
	// Avoid pulling in strings just for a case-insensitive contains check.
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
