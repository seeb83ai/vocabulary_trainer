package migrate

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// v20260928120000 deletes the sample tag "a1" (from chinese_a1.txt). Only the
// exact name "a1" is removed: its word links, the tag row, and the entry in
// saved training filters (user_settings.train_tags). Words are kept.
func init() {
	register(migration{
		version: 20260928120000,
		fn: func(db *sql.DB) error {
			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()

			for _, s := range []string{
				`DELETE FROM word_tags WHERE tag_id IN (SELECT id FROM tags WHERE name = 'a1')`,
				`DELETE FROM tags WHERE name = 'a1'`,
			} {
				if _, err := tx.Exec(s); err != nil {
					return fmt.Errorf("delete tag a1: %w", err)
				}
			}

			type setting struct {
				userID int64
				tags   string
			}
			rows, err := tx.Query(`SELECT user_id, train_tags FROM user_settings WHERE train_tags LIKE '%a1%'`)
			if err != nil {
				return fmt.Errorf("read train_tags: %w", err)
			}
			var settings []setting
			for rows.Next() {
				var s setting
				if err := rows.Scan(&s.userID, &s.tags); err != nil {
					rows.Close()
					return err
				}
				settings = append(settings, s)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			for _, s := range settings {
				var tags []string
				if err := json.Unmarshal([]byte(s.tags), &tags); err != nil {
					continue
				}
				out := make([]string, 0, len(tags))
				for _, tg := range tags {
					if tg != "a1" {
						out = append(out, tg)
					}
				}
				if len(out) == len(tags) {
					continue
				}
				b, err := json.Marshal(out)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(`UPDATE user_settings SET train_tags = ? WHERE user_id = ?`, string(b), s.userID); err != nil {
					return fmt.Errorf("update train_tags: %w", err)
				}
			}
			return tx.Commit()
		},
	})
}
