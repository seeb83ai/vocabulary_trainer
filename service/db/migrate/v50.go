package migrate

import "database/sql"

func init() {
	register(migration{
		version: 50,
		fn: func(database *sql.DB) error {
			stmt := `ALTER TABLE users ADD COLUMN sessions_invalidated_at TEXT NOT NULL DEFAULT ''`
			if _, err := database.Exec(stmt); err != nil {
				if !columnExistsErr(err) {
					return err
				}
			}
			return nil
		},
	})
}
