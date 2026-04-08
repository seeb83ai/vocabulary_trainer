package migrate

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"
)

func init() {
	register(migration{
		// v15: add hmm_progress table for SM-2 spaced repetition on mnemonic
		// library entries (actors, locations, tone rooms, props).
		// Also seeds progress rows for any already-named library entries,
		// with shuffled due_dates so review order is random.
		version: 15,
		sql: `
CREATE TABLE IF NOT EXISTS hmm_progress (
  entity_type     TEXT    NOT NULL,
  entity_key      TEXT    NOT NULL,
  repetitions     INTEGER NOT NULL DEFAULT 0,
  easiness        REAL    NOT NULL DEFAULT 2.5,
  interval_days   INTEGER NOT NULL DEFAULT 1,
  due_date        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  total_correct   INTEGER NOT NULL DEFAULT 0,
  total_attempts  INTEGER NOT NULL DEFAULT 0,
  learning        INTEGER NOT NULL DEFAULT 1,
  streak_bonus    INTEGER NOT NULL DEFAULT 0,
  first_seen_date TEXT DEFAULT NULL,
  PRIMARY KEY (entity_type, entity_key)
);
CREATE INDEX IF NOT EXISTS idx_hmm_progress_due ON hmm_progress(due_date);
`,
		fn: func(db *sql.DB) error {
			type seed struct {
				typ   string
				query string
			}
			seeds := []seed{
				{"actor", `SELECT initial FROM hmm_actors WHERE actor_name != ''`},
				{"location", `SELECT final_key FROM hmm_locations WHERE location_name != ''`},
				{"tone_room", `SELECT CAST(tone AS TEXT) FROM hmm_tone_rooms WHERE room_name != ''`},
				{"prop", `SELECT radical FROM hmm_props WHERE prop_name != ''`},
			}
			now := time.Now().UTC()
			for _, s := range seeds {
				rows, err := db.Query(s.query)
				if err != nil {
					return fmt.Errorf("seed hmm_progress %s: %w", s.typ, err)
				}
				var keys []string
				for rows.Next() {
					var k string
					if err := rows.Scan(&k); err != nil {
						rows.Close()
						return fmt.Errorf("scan hmm %s key: %w", s.typ, err)
					}
					keys = append(keys, k)
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					return fmt.Errorf("iterate hmm %s keys: %w", s.typ, err)
				}
				rand.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
				for i, key := range keys {
					dueDate := now.Add(-time.Duration(len(keys)-i) * time.Second).Format("2006-01-02 15:04:05")
					if _, err := db.Exec(
						`INSERT OR IGNORE INTO hmm_progress (entity_type, entity_key, due_date) VALUES (?, ?, ?)`,
						s.typ, key, dueDate); err != nil {
						return fmt.Errorf("insert hmm_progress %s/%s: %w", s.typ, key, err)
					}
				}
			}
			return nil
		},
	})
}
