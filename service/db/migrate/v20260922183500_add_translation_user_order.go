package migrate

import "database/sql"

// v20260922183500 adds the per-user translation_user_order setting: whether
// user-added translations appear before ("first", default) or after ("last")
// dictionary glosses on quiz cards when translation ranking is enabled.
func init() {
	register(migration{
		version: 20260922183500,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'translation_user_order'`).Scan(&count); err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
			_, err := db.Exec(`ALTER TABLE user_settings ADD COLUMN translation_user_order TEXT NOT NULL DEFAULT 'first'`)
			return err
		},
	})
}
