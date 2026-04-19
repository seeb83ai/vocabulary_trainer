package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v19: create missing hmm_progress rows for any named library entries that
		// were added after migration v15 (which only ran once). Sets first_seen_date
		// to today so they are immediately eligible for training.
		version: 19,
		fn: func(db *sql.DB) error {
			seeds := []struct{ typ, query string }{
				{"actor", `SELECT initial FROM hmm_actors WHERE actor_name != ''`},
				{"location", `SELECT final_key FROM hmm_locations WHERE location_name != ''`},
				{"tone_room", `SELECT CAST(tone AS TEXT) FROM hmm_tone_rooms WHERE room_name != ''`},
				{"prop", `SELECT radical FROM hmm_props WHERE prop_name != ''`},
			}
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
				for _, key := range keys {
					if _, err := db.Exec(
						`INSERT OR IGNORE INTO hmm_progress (entity_type, entity_key, first_seen_date) VALUES (?, ?, date('now'))`,
						s.typ, key); err != nil {
						return fmt.Errorf("insert hmm_progress %s/%s: %w", s.typ, key, err)
					}
				}
			}
			return nil
		},
	})
}
