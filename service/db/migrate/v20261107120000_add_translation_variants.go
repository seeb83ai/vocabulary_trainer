package migrate

import (
	"database/sql"
	"vocabulary_trainer/sm2"
)

// v20261107120000 adds translation_variants: every accepted answer form
// (sm2.ExpandVariants) of every translation word, with an index on the form.
// The wrong-answer lookups (mix-ups, homophones) then find the words whose
// translation equals the typed answer with the index, instead of expanding
// every translation of the user on each wrong answer.
//
// A translation word's text never changes, so its variants are written once,
// when the word is inserted, and removed with the word (ON DELETE CASCADE).
// FillTranslationVariants also runs on every Migrate, so translations that a
// CLI tool or a migration inserts with plain SQL get their variants at the next
// start. If the answer rules in sm2 change, add a migration that deletes all
// rows and calls FillTranslationVariants (TestTranslationVariants_RulesUnchanged
// fails as a reminder).
func init() {
	register(migration{
		version: 20261107120000,
		sql: `
CREATE TABLE IF NOT EXISTS translation_variants (
  word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  variant TEXT NOT NULL,
  PRIMARY KEY (word_id, variant)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_translation_variants_variant ON translation_variants(variant);`,
		fn: FillTranslationVariants,
	})
}

// FillTranslationVariants writes the variants of every translation word that
// has none yet.
func FillTranslationVariants(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT w.id, w.text FROM words w
		WHERE w.language != 'zh'
		  AND NOT EXISTS (SELECT 1 FROM translation_variants v WHERE v.word_id = w.id)`)
	if err != nil {
		return err
	}
	type word struct {
		id   int64
		text string
	}
	var missing []word
	for rows.Next() {
		var w word
		if err := rows.Scan(&w.id, &w.text); err != nil {
			rows.Close()
			return err
		}
		missing = append(missing, w)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ins, err := tx.Prepare(`INSERT OR IGNORE INTO translation_variants (word_id, variant) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for _, w := range missing {
		for _, v := range sm2.ExpandVariants(w.text) {
			if _, err := ins.Exec(w.id, v); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
