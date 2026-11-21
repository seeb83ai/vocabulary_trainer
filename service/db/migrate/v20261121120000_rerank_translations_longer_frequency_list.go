package migrate

import (
	"database/sql"
	"fmt"
)

// v20261121120000 loads the longer en/de frequency lists (about 50,000 words
// instead of 8,000) into databases that were migrated with the short ones, so
// less common glosses such as "verleihen" get a rank (issue #658). Then it
// rebuilds the gloss_rank cache and computes the stored rank of every
// dictionary gloss again with FrequencyWords, which also ignores text in
// parentheses such as the HanDeDict tag "(V)". User translations keep rank 0.
func init() {
	register(migration{
		version: 20261121120000,
		fn: func(db *sql.DB) error {
			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()
			for lang, data := range map[string]string{"en": frequencyDataEN, "de": frequencyDataDE} {
				// A database created after the lists got longer has them already.
				var longer int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM word_frequency_lang WHERE lang = ? AND rank > 8000`, lang).Scan(&longer); err != nil {
					return err
				}
				if longer > 0 {
					continue
				}
				if err := importFrequencyLangData(tx, data, lang); err != nil {
					return err
				}
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			if err := RebuildGlossRank(db); err != nil {
				return err
			}
			return rerankCedictTranslations(db)
		},
	})
}

// rerankCedictTranslations sets translations.rank of every en/de dictionary
// gloss to the rank db.computeTranslationRank gives it.
func rerankCedictTranslations(db *sql.DB) error {
	freq, err := loadFrequencyRanks(db)
	if err != nil {
		return err
	}

	type link struct {
		transID, zhID int64
		rank          sql.NullInt64
	}
	rows, err := db.Query(
		`SELECT t.translation_word_id, t.zh_word_id, t.rank, w.language, w.text
		 FROM translations t JOIN words w ON w.id = t.translation_word_id
		 WHERE t.source = 'cedict' AND w.language IN ('en', 'de')`)
	if err != nil {
		return fmt.Errorf("read cedict translations: %w", err)
	}
	var changed []link
	for rows.Next() {
		var l link
		var old sql.NullInt64
		var lang, text string
		if err := rows.Scan(&l.transID, &l.zhID, &old, &lang, &text); err != nil {
			rows.Close()
			return err
		}
		l.rank = glossRank(freq[lang], text)
		if l.rank != old {
			changed = append(changed, l)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE translations SET rank = ? WHERE translation_word_id = ? AND zh_word_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, l := range changed {
		if _, err := stmt.Exec(l.rank, l.transID, l.zhID); err != nil {
			return fmt.Errorf("update translation rank: %w", err)
		}
	}
	return tx.Commit()
}
