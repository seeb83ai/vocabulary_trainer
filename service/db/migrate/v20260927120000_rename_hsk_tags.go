package migrate

import (
	"database/sql"
	"fmt"
)

// v20260927120000 renames the HSK 2.0 library tags hsk1..hsk6 (and their
// sentence variants s_hsk1..s_hsk6) to hsk2-1..hsk2-6, so they do not clash
// with the HSK 3.0 tags hsk3-N. Only these exact names change. If the new
// name already exists, the two tags are merged into one row. Saved training
// filters (user_settings.train_tags) are renamed too.
func init() {
	register(migration{
		version: 20260927120000,
		fn: func(db *sql.DB) error {
			renames := map[string]string{}
			var order [][2]string
			for level := 1; level <= 6; level++ {
				for _, prefix := range []string{"", "s_"} {
					oldName := fmt.Sprintf("%shsk%d", prefix, level)
					newName := fmt.Sprintf("%shsk2-%d", prefix, level)
					renames[oldName] = newName
					order = append(order, [2]string{oldName, newName})
				}
			}

			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()

			for _, r := range order {
				if err := renameTag(tx, r[0], r[1]); err != nil {
					return fmt.Errorf("rename tag %s: %w", r[0], err)
				}
			}

			if err := renameTrainTags(tx, `%hsk%'`, func(tg string) string {
				if n, ok := renames[tg]; ok {
					return n
				}
				return tg
			}); err != nil {
				return err
			}
			return tx.Commit()
		},
	})
}

// renameTag gives every tag row named oldName the name newName. All rows
// that end up with newName are merged into one: word links move to the
// lowest id, which keeps the highest importable flag and the first
// non-empty description.
func renameTag(tx *sql.Tx, oldName, newName string) error {
	var target sql.NullInt64
	if err := tx.QueryRow(`SELECT MIN(id) FROM tags WHERE name IN (?, ?)`, oldName, newName).Scan(&target); err != nil {
		return err
	}
	if !target.Valid {
		return nil
	}
	stmts := []string{
		`UPDATE tags SET
		   importable = (SELECT MAX(importable) FROM tags WHERE name IN (?2, ?3)),
		   description = COALESCE((SELECT description FROM tags WHERE name IN (?2, ?3) AND description != '' ORDER BY id LIMIT 1), '')
		 WHERE id = ?1`,
		`INSERT OR IGNORE INTO word_tags (word_id, tag_id)
		 SELECT word_id, ?1 FROM word_tags
		 WHERE tag_id IN (SELECT id FROM tags WHERE name IN (?2, ?3) AND id != ?1)`,
		`DELETE FROM word_tags WHERE tag_id IN (SELECT id FROM tags WHERE name IN (?2, ?3) AND id != ?1)`,
		`DELETE FROM tags WHERE name IN (?2, ?3) AND id != ?1`,
		`UPDATE tags SET name = ?3 WHERE id = ?1`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s, target.Int64, oldName, newName); err != nil {
			return err
		}
	}
	return nil
}
