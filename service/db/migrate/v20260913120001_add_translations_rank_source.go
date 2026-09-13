package migrate

import "database/sql"

// v20260913120001 adds per-translation importance ranking. `source`
// distinguishes translations a human typed/edited directly (via
// CreateWord/UpdateWord/AddTranslation) from ones an automated pipeline
// derived from CC-CEDICT/HanDeDict (subword segmentation, hanzi lookups) —
// only the latter ever get hidden during training. `rank` is the frequency
// rank of the translation's rarest word (from word_frequency_lang), NULL
// when unranked. Existing rows predate this feature and can't be
// attributed to either path, so they default to source='user' (never
// hidden) rather than guessing.
func init() {
	register(migration{
		version: 20260913120001,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('translations') WHERE name = 'source'`).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE translations ADD COLUMN source TEXT NOT NULL DEFAULT 'user'`); err != nil {
					return err
				}
			}
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('translations') WHERE name = 'rank'`).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE translations ADD COLUMN rank INTEGER DEFAULT 0`); err != nil {
					return err
				}
			}
			return nil
		},
	})
}
