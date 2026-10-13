package migrate

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// v20261013203000 moves the sentence marker from the `s_` prefix to the `_s`
// suffix: every tag s_X (any case of the prefix) becomes X_s, e.g. s_hsk2-1
// becomes hsk2-1_s. The tag then falls into the same tag group as X (OR
// inside a group), so sentences are an addon to their list instead of a
// separate AND group. If X_s already exists, the two tags are merged. Saved
// training filters (user_settings.train_tags) are renamed too.
func init() {
	register(migration{
		version: 20261013203000,
		fn: func(db *sql.DB) error {
			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()

			rows, err := tx.Query(`SELECT name FROM tags WHERE name LIKE 's\_%' ESCAPE '\' AND length(name) > 2 ORDER BY id`)
			if err != nil {
				return fmt.Errorf("read sentence tags: %w", err)
			}
			var names []string
			for rows.Next() {
				var n string
				if err := rows.Scan(&n); err != nil {
					rows.Close()
					return err
				}
				names = append(names, n)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			for _, n := range names {
				if err := renameTag(tx, n, sentenceSuffixName(n)); err != nil {
					return fmt.Errorf("rename tag %s: %w", n, err)
				}
			}

			if err := renameTrainTags(tx, `%s\_%' ESCAPE '\'`, func(tg string) string {
				if len(tg) > 2 && strings.EqualFold(tg[:2], "s_") {
					return sentenceSuffixName(tg)
				}
				return tg
			}); err != nil {
				return err
			}
			return tx.Commit()
		},
	})
}

// sentenceSuffixName turns s_X into X_s.
func sentenceSuffixName(name string) string {
	return name[2:] + "_s"
}

// renameTrainTags rewrites every user's saved training filter whose JSON
// matches the LIKE pattern like (with its ESCAPE clause): each tag is passed
// through rename, and duplicates that result are dropped.
func renameTrainTags(tx *sql.Tx, like string, rename func(string) string) error {
	type setting struct {
		userID int64
		tags   string
	}
	rows, err := tx.Query(`SELECT user_id, train_tags FROM user_settings WHERE train_tags LIKE '` + like)
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
		seen := map[string]bool{}
		out := make([]string, 0, len(tags))
		for _, tg := range tags {
			tg = rename(tg)
			if seen[tg] {
				continue
			}
			seen[tg] = true
			out = append(out, tg)
		}
		b, err := json.Marshal(out)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE user_settings SET train_tags = ? WHERE user_id = ?`, string(b), s.userID); err != nil {
			return fmt.Errorf("update train_tags: %w", err)
		}
	}
	return nil
}
