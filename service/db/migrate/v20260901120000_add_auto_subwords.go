package migrate

import "database/sql"

func init() {
	register(migration{
		version: 20260901120000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'auto_subwords'`).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				_, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN auto_subwords INTEGER NOT NULL DEFAULT 1`)
				return err
			}
			return nil
		},
	})
}
