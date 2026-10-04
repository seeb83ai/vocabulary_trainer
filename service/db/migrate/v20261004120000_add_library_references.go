package migrate

import "database/sql"

// v20261004120000 adds library references (see ADR-0005). A learner's zh word
// with library_word_id set is a library reference. Its glosses are the library
// word's links in the learner's primary/secondary language, minus the glosses
// the learner deleted (translation_deletions), plus the learner's own links on
// the reference row (added glosses). The user_translations view applies that
// overlay, so every per-user gloss read uses it instead of the translations
// table. It is a single SELECT (no UNION) so SQLite can flatten it into the
// calling query and use the zh_word_id index.
//
// overrides_updated_at on a reference is set whenever the learner changes its
// glosses; library_updated_at on a library word is set when a dictionary
// refresh changes its gloss set. A reference edited before the library changed
// is a conflict. library_removed flags library words the dictionaries dropped.
// word_tombstones remembers library words a learner deleted, so a list sync
// does not add them again.
func init() {
	register(migration{
		version: 20261004120000,
		fn: func(db *sql.DB) error {
			for _, col := range []struct{ name, def string }{
				{"library_word_id", "INTEGER"},
				{"library_updated_at", "TEXT"},
				{"library_removed", "INTEGER NOT NULL DEFAULT 0"},
				{"overrides_updated_at", "TEXT"},
			} {
				var n int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('words') WHERE name = ?`, col.name).Scan(&n); err != nil {
					return err
				}
				if n == 0 {
					if _, err := db.Exec(`ALTER TABLE words ADD COLUMN ` + col.name + ` ` + col.def); err != nil {
						return err
					}
				}
			}
			_, err := db.Exec(`
CREATE INDEX IF NOT EXISTS idx_words_library_word ON words(library_word_id) WHERE library_word_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS translation_deletions (
  user_word_id        INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  translation_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  PRIMARY KEY (user_word_id, translation_word_id)
);

CREATE TABLE IF NOT EXISTS word_tombstones (
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  library_word_id INTEGER NOT NULL,
  created_at      TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, library_word_id)
);

DROP VIEW IF EXISTS user_translations;
CREATE VIEW user_translations AS
SELECT w.id AS zh_word_id, t.translation_word_id, t.source, t.rank
FROM words w
JOIN translations t ON t.zh_word_id = w.id OR t.zh_word_id = w.library_word_id
JOIN words tw ON tw.id = t.translation_word_id
WHERE t.zh_word_id = w.id
   OR ((tw.language = COALESCE((SELECT us.primary_lang FROM user_settings us WHERE us.user_id = w.user_id), 'en')
        OR tw.language = COALESCE((SELECT us.secondary_lang FROM user_settings us WHERE us.user_id = w.user_id), 'de'))
       AND NOT EXISTS (
           SELECT 1 FROM translation_deletions d
           WHERE d.user_word_id = w.id AND d.translation_word_id = t.translation_word_id));
`)
			return err
		},
	})
}
