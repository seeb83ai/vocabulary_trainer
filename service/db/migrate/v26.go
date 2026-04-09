package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v26: add user_id to pinyin_daily_stats; PK changes from (date) → (user_id, date).
		version: 26,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return fmt.Errorf("disable foreign keys: %w", err)
			}
			defer db.Exec(`PRAGMA foreign_keys = ON`)

			stmts := []string{
				`CREATE TABLE IF NOT EXISTS pinyin_daily_stats_new (
				  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  date          TEXT    NOT NULL,
				  attempts      INTEGER NOT NULL DEFAULT 0,
				  mistakes      INTEGER NOT NULL DEFAULT 0,
				  sounds_seen   INTEGER NOT NULL DEFAULT 0,
				  tone1_correct INTEGER NOT NULL DEFAULT 0,
				  tone1_wrong   INTEGER NOT NULL DEFAULT 0,
				  tone2_correct INTEGER NOT NULL DEFAULT 0,
				  tone2_wrong   INTEGER NOT NULL DEFAULT 0,
				  tone3_correct INTEGER NOT NULL DEFAULT 0,
				  tone3_wrong   INTEGER NOT NULL DEFAULT 0,
				  tone4_correct INTEGER NOT NULL DEFAULT 0,
				  tone4_wrong   INTEGER NOT NULL DEFAULT 0,
				  tone5_correct INTEGER NOT NULL DEFAULT 0,
				  tone5_wrong   INTEGER NOT NULL DEFAULT 0,
				  PRIMARY KEY (user_id, date)
				)`,
				`INSERT OR IGNORE INTO pinyin_daily_stats_new
				   (user_id, date, attempts, mistakes, sounds_seen,
				    tone1_correct, tone1_wrong, tone2_correct, tone2_wrong,
				    tone3_correct, tone3_wrong, tone4_correct, tone4_wrong,
				    tone5_correct, tone5_wrong)
				 SELECT
				   2,
				   date, attempts, mistakes, COALESCE(sounds_seen, 0),
				   COALESCE(tone1_correct, 0), COALESCE(tone1_wrong, 0),
				   COALESCE(tone2_correct, 0), COALESCE(tone2_wrong, 0),
				   COALESCE(tone3_correct, 0), COALESCE(tone3_wrong, 0),
				   COALESCE(tone4_correct, 0), COALESCE(tone4_wrong, 0),
				   COALESCE(tone5_correct, 0), COALESCE(tone5_wrong, 0)
				 FROM pinyin_daily_stats`,
				`DROP TABLE pinyin_daily_stats`,
				`ALTER TABLE pinyin_daily_stats_new RENAME TO pinyin_daily_stats`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("rebuild pinyin_daily_stats table: %w", err)
				}
			}
			return nil
		},
	})
}
