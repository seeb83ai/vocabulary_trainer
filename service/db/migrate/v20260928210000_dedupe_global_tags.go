package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 20260928210000,
		fn: func(db *sql.DB) error {
			// getOrCreateTag used INSERT OR IGNORE with user_id NULL. NULLs never
			// collide in UNIQUE(name, user_id), so every tagging call added another
			// row with the same name. Merge those into the lowest id per name.
			const dups = `SELECT t.id FROM tags t WHERE t.user_id IS NULL
				AND t.id != (SELECT MIN(m.id) FROM tags m WHERE m.name = t.name AND m.user_id IS NULL)`
			stmts := []string{
				`INSERT OR IGNORE INTO word_tags (word_id, tag_id)
				 SELECT wt.word_id,
				        (SELECT MIN(m.id) FROM tags m WHERE m.name = t.name AND m.user_id IS NULL)
				 FROM word_tags wt JOIN tags t ON t.id = wt.tag_id
				 WHERE wt.tag_id IN (` + dups + `)`,
				`DELETE FROM word_tags WHERE tag_id IN (` + dups + `)`,
				`DELETE FROM tags WHERE id IN (` + dups + `)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_global_name ON tags(name) WHERE user_id IS NULL`,
			}
			for _, s := range stmts {
				if _, err := db.Exec(s); err != nil {
					return fmt.Errorf("dedupe global tags: %w", err)
				}
			}
			return nil
		},
	})
}
