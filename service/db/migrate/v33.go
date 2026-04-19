package migrate

import (
	"database/sql"
)

func init() {
	register(migration{
		// v33: add description and importable columns to tags.
		// description: optional human-readable note shown during import.
		// importable: whether other users may import words tagged with this tag (default 1 = yes).
		version: 33,
		fn: func(db *sql.DB) error {
			for _, stmt := range []string{
				`ALTER TABLE tags ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE tags ADD COLUMN importable INTEGER NOT NULL DEFAULT 1`,
			} {
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
