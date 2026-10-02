package migrate

import "database/sql"

// v20260928150000 adds import_jobs: one row per library-list import, so the
// import can run in the background and survive a server restart. and_tags
// narrows the import to words that also carry every one of those tags, and
// import_mode is include, review or known. status is queued, running, done or
// failed; total/done count source words, and imported/tagged/skipped are the
// same counters the import response used to return.
func init() {
	register(migration{
		version: 20260928150000,
		fn: func(db *sql.DB) error {
			_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS import_jobs (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  tag          TEXT NOT NULL,
  import_langs TEXT NOT NULL DEFAULT '[]',
  apply_tags   TEXT NOT NULL DEFAULT '[]',
  and_tags     TEXT NOT NULL DEFAULT '[]',
  import_mode  TEXT NOT NULL DEFAULT 'include',
  status       TEXT NOT NULL DEFAULT 'queued',
  total        INTEGER NOT NULL DEFAULT 0,
  done         INTEGER NOT NULL DEFAULT 0,
  imported     INTEGER NOT NULL DEFAULT 0,
  tagged       INTEGER NOT NULL DEFAULT 0,
  skipped      INTEGER NOT NULL DEFAULT 0,
  error        TEXT NOT NULL DEFAULT '',
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_import_jobs_user_status ON import_jobs(user_id, status);`)
			return err
		},
	})
}
