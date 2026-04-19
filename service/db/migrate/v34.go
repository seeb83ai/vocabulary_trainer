package migrate

import (
	"database/sql"
)

func init() {
	register(migration{
		// v34: add role column to users (admin, plus, free).
		// id=1 seeded as admin, id=2 as plus, all others default to free.
		version: 34,
		fn: func(db *sql.DB) error {
			stmts := []string{
				`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'free'`,
				`UPDATE users SET role = 'admin' WHERE id = 1`,
				`UPDATE users SET role = 'plus'  WHERE id = 2`,
			}
			for _, stmt := range stmts {
				if _, err := db.Exec(stmt); err != nil {
					if sqliteIsDuplicateColumn(err) {
						continue
					}
					return err
				}
			}
			return nil
		},
	})
}
