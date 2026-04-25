package migrate

import "database/sql"

func init() {
	register(migration{
		version: 40,
		fn:      migrateV40,
	})
}

func migrateV40(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(component_stats)`)
	if err != nil {
		return err
	}
	hasCol := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "components_total" {
			hasCol = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if hasCol {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE component_stats ADD COLUMN components_total INTEGER NOT NULL DEFAULT 0`)
	return err
}
