package migrate

import (
	"database/sql"
	"slices"
	"strings"
)

// v20261003120000 repairs example sentences ("Bsp.: 我帮助你学习汉语。 -- Ich
// helfe dir, Chinesisch zu lernen") that earlier imports split at the comma
// into several translations (issue #523). For every dictionary sense that
// starts with "Bsp.:" and contains a comma, a zh word that has the first
// fragment linked as a 'cedict' translation gets the whole sentence instead.
// The other fragments lose their link too, unless the same text is a sense of
// its own in that definition. User-entered translations stay as they are, and
// a fragment word with no links left is deleted with its progress row. The
// gloss_rank cache is rebuilt at the end because the fixed split changes the
// glosses it holds.
func init() {
	register(migration{
		version: 20261003120000,
		fn: func(db *sql.DB) error {
			type example struct {
				simplified, lang, whole string
				fragments, standalone   []string
			}
			rows, err := db.Query(`SELECT simplified, lang, definition FROM cedict_entries WHERE lang IN ('en', 'de')`)
			if err != nil {
				return err
			}
			var examples []example
			for rows.Next() {
				var simplified, lang, def string
				if err := rows.Scan(&simplified, &lang, &def); err != nil {
					rows.Close()
					return err
				}
				senses := splitCedictSenses(def)
				for _, sense := range senses {
					if !strings.HasPrefix(sense, "Bsp.:") || !strings.Contains(sense, ",") {
						continue
					}
					examples = append(examples, example{
						simplified: simplified, lang: lang, whole: sense,
						fragments: splitAtCommas(sense), standalone: senses,
					})
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

			for _, ex := range examples {
				zhRows, err := tx.Query(`SELECT id, user_id FROM words WHERE text = ? AND language = 'zh'`, ex.simplified)
				if err != nil {
					return err
				}
				type zhWord struct{ id, userID int64 }
				var zhWords []zhWord
				for zhRows.Next() {
					var z zhWord
					if err := zhRows.Scan(&z.id, &z.userID); err != nil {
						zhRows.Close()
						return err
					}
					zhWords = append(zhWords, z)
				}
				zhRows.Close()
				if err := zhRows.Err(); err != nil {
					return err
				}

				for _, z := range zhWords {
					if err := rejoinExample(tx, z.id, z.userID, ex.lang, ex.whole, ex.fragments, ex.standalone); err != nil {
						return err
					}
				}
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			return RebuildGlossRank(db)
		},
	})
}

func rejoinExample(tx *sql.Tx, zhID, userID int64, lang, whole string, fragments, standalone []string) error {
	fragIDs := map[string]int64{}
	for _, f := range fragments {
		var id int64
		err := tx.QueryRow(`
			SELECT w.id FROM translations t JOIN words w ON w.id = t.translation_word_id
			WHERE t.zh_word_id = ? AND t.source = 'cedict' AND w.text = ? AND w.language = ? AND w.user_id = ?`,
			zhID, f, lang, userID).Scan(&id)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		fragIDs[f] = id
	}
	if _, ok := fragIDs[fragments[0]]; !ok {
		return nil
	}

	if _, err := tx.Exec(`INSERT OR IGNORE INTO words (text, language, user_id) VALUES (?, ?, ?)`, whole, lang, userID); err != nil {
		return err
	}
	var wholeID int64
	if err := tx.QueryRow(`SELECT id FROM words WHERE text = ? AND language = ? AND user_id = ?`, whole, lang, userID).Scan(&wholeID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO sm2_progress (word_id) VALUES (?)`, wholeID); err != nil {
		return err
	}
	rank, err := cedictSenseRank(tx, lang, whole)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO translations (translation_word_id, zh_word_id, source, rank) VALUES (?, ?, 'cedict', ?)`,
		wholeID, zhID, rank); err != nil {
		return err
	}

	for i, f := range fragments {
		id, ok := fragIDs[f]
		if !ok || (i > 0 && slices.Contains(standalone, f)) {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM translations WHERE translation_word_id = ? AND zh_word_id = ?`, id, zhID); err != nil {
			return err
		}
		var remaining int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM translations WHERE translation_word_id = ?`, id).Scan(&remaining); err != nil {
			return err
		}
		if remaining == 0 {
			if _, err := tx.Exec(`DELETE FROM sm2_progress WHERE word_id = ?`, id); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM words WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// splitAtCommas is the split the earlier imports applied: at every comma
// outside brackets.
func splitAtCommas(s string) []string {
	var parts []string
	depth := 0
	start := 0
	flush := func(end int) {
		if p := strings.TrimSpace(s[start:end]); p != "" {
			parts = append(parts, p)
		}
	}
	for i, r := range s {
		switch r {
		case '(', '[', '（':
			depth++
		case ')', ']', '）':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				flush(i)
				start = i + 1
			}
		}
	}
	flush(len(s))
	return parts
}
