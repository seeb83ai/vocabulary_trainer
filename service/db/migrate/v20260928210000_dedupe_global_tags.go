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
			//
			// We build a temp table of (canonical_id, dup_id) first so the
			// correlated subquery runs once, not once per row in the large queries.
			stmts := []string{
				`CREATE TEMP TABLE _tag_canon AS
				 SELECT t.id AS dup_id,
				        MIN(m.id) AS canon_id
				 FROM tags t
				 JOIN tags m ON m.name = t.name AND m.user_id IS NULL
				 WHERE t.user_id IS NULL
				 GROUP BY t.id`,
				`CREATE INDEX _tag_canon_dup ON _tag_canon(dup_id)`,
				// Re-point word_tags rows that reference a dup to the canonical id.
				`INSERT OR IGNORE INTO word_tags (word_id, tag_id)
				 SELECT wt.word_id, c.canon_id
				 FROM word_tags wt
				 JOIN _tag_canon c ON c.dup_id = wt.tag_id
				 WHERE c.dup_id != c.canon_id`,
				`DELETE FROM word_tags
				 WHERE tag_id IN (SELECT dup_id FROM _tag_canon WHERE dup_id != canon_id)`,
				`DELETE FROM tags
				 WHERE id IN (SELECT dup_id FROM _tag_canon WHERE dup_id != canon_id)`,
				`DROP TABLE _tag_canon`,
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
