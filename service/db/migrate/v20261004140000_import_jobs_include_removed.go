package migrate

import "database/sql"

// v20261004140000 adds import_jobs.include_removed: by default an import or
// list sync skips library words the learner deleted (word_tombstones); a job
// with include_removed = 1 adds them again.
func init() {
	register(migration{
		version: 20261004140000,
		fn: func(db *sql.DB) error {
			var n int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('import_jobs') WHERE name = 'include_removed'`).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return nil
			}
			_, err := db.Exec(`ALTER TABLE import_jobs ADD COLUMN include_removed INTEGER NOT NULL DEFAULT 0`)
			return err
		},
	})
}
