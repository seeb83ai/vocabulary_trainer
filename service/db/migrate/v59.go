package migrate

import (
	"database/sql"
	"fmt"
)

// v59 makes hanzi_decomposition_translation a per-user copy-on-write overlay.
// Previously the table was global (PRIMARY KEY (character, lang)), so any user
// editing a component definition overwrote it for everyone. We add a nullable
// user_id: seeded defaults keep user_id IS NULL (shared, read-only), and a
// user's edit is stored as a separate row keyed to that user. Partial unique
// indexes enforce one global row and one per-user row per (character, lang).
func init() {
	register(migration{
		version: 59,
		fn: func(db *sql.DB) error {
			var c int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('hanzi_decomposition_translation') WHERE name = 'user_id'`,
			).Scan(&c); err != nil {
				return fmt.Errorf("check user_id column: %w", err)
			}
			if c > 0 {
				return nil // already migrated
			}

			stmts := []string{
				// FK off so the table swap doesn't cascade-delete dependent rows.
				`PRAGMA foreign_keys=OFF`,
				`CREATE TABLE hanzi_decomposition_translation_new (
				    character  TEXT NOT NULL REFERENCES hanzi_decomposition(character) ON DELETE CASCADE,
				    lang       TEXT NOT NULL,
				    definition TEXT NOT NULL,
				    user_id    INTEGER REFERENCES users(id) ON DELETE CASCADE
				)`,
				`INSERT INTO hanzi_decomposition_translation_new (character, lang, definition, user_id)
				 SELECT character, lang, definition, NULL FROM hanzi_decomposition_translation`,
				`DROP TABLE hanzi_decomposition_translation`,
				`ALTER TABLE hanzi_decomposition_translation_new RENAME TO hanzi_decomposition_translation`,
				// One shared default per (character, lang); one override per user.
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_hanzi_trans_global
				 ON hanzi_decomposition_translation(character, lang) WHERE user_id IS NULL`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_hanzi_trans_user
				 ON hanzi_decomposition_translation(character, lang, user_id) WHERE user_id IS NOT NULL`,
				`CREATE INDEX IF NOT EXISTS idx_hanzi_trans_lang ON hanzi_decomposition_translation(lang)`,
				`PRAGMA foreign_keys=ON`,
			}
			for _, st := range stmts {
				if _, err := db.Exec(st); err != nil {
					return fmt.Errorf("v59 migration: %w", err)
				}
			}
			return nil
		},
	})
}
