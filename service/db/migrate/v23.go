package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v23: add user_id to daily_stats; PK changes from (date) → (user_id, date).
		// Existing rows are assigned to the initial user.
		version: 23,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS daily_stats_new (
				  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  date            TEXT    NOT NULL,
				  attempts        INTEGER NOT NULL DEFAULT 0,
				  mistakes        INTEGER NOT NULL DEFAULT 0,
				  words_seen      INTEGER NOT NULL DEFAULT 0,
				  correct_streak  INTEGER NOT NULL DEFAULT 0,
				  current_streak  INTEGER NOT NULL DEFAULT 0,
				  bucket_new      INTEGER NOT NULL DEFAULT 0,
				  bucket_struggling INTEGER NOT NULL DEFAULT 0,
				  bucket_learning INTEGER NOT NULL DEFAULT 0,
				  bucket_practicing INTEGER NOT NULL DEFAULT 0,
				  bucket_mastered INTEGER NOT NULL DEFAULT 0,
				  PRIMARY KEY (user_id, date)
				)`,
				`INSERT OR IGNORE INTO daily_stats_new
				   (user_id, date, attempts, mistakes, words_seen, correct_streak, current_streak,
				    bucket_new, bucket_struggling, bucket_learning, bucket_practicing, bucket_mastered)
				 SELECT
				   2,
				   date, attempts, mistakes,
				   COALESCE(words_seen, 0), COALESCE(correct_streak, 0), COALESCE(current_streak, 0),
				   COALESCE(bucket_new, 0), COALESCE(bucket_struggling, 0), COALESCE(bucket_learning, 0),
				   COALESCE(bucket_practicing, 0), COALESCE(bucket_mastered, 0)
				 FROM daily_stats`,
				`DROP TABLE daily_stats`,
				`ALTER TABLE daily_stats_new RENAME TO daily_stats`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild daily_stats table: %w", err)
				}
			}
			return nil
		},
	})
}
