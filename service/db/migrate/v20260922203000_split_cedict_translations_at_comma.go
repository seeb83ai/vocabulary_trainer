package migrate

import (
	"database/sql"
	"strings"
	"unicode"
)

// v20260922203000 splits dictionary translations that were stored as one
// comma-joined gloss (HanDeDict separates senses with ",", e.g. "wegen, um
// (P), im Bestreben (S)") into one translation per sense. Only links with
// source='cedict' change; user-entered translations stay as they are. Commas
// inside brackets do not split. A translation word that has no links left
// afterwards is deleted with its progress row. The split and rank logic
// mirrors db.splitSenses and db.computeTranslationRank (the migrate package
// cannot import db).
func init() {
	register(migration{
		version: 20260922203000,
		fn: func(db *sql.DB) error {
			type link struct {
				transID, zhID, userID int64
				text, lang            string
			}
			rows, err := db.Query(`
				SELECT t.translation_word_id, t.zh_word_id, w.user_id, w.text, w.language
				FROM translations t
				JOIN words w ON w.id = t.translation_word_id
				WHERE t.source = 'cedict' AND w.text LIKE '%,%'`)
			if err != nil {
				return err
			}
			var links []link
			for rows.Next() {
				var l link
				if err := rows.Scan(&l.transID, &l.zhID, &l.userID, &l.text, &l.lang); err != nil {
					rows.Close()
					return err
				}
				links = append(links, l)
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

			for _, l := range links {
				senses := splitCedictSenses(l.text)
				if len(senses) < 2 {
					continue
				}
				for _, sense := range senses {
					if _, err := tx.Exec(`INSERT OR IGNORE INTO words (text, language, user_id) VALUES (?, ?, ?)`,
						sense, l.lang, l.userID); err != nil {
						return err
					}
					var id int64
					if err := tx.QueryRow(`SELECT id FROM words WHERE text = ? AND language = ? AND user_id = ?`,
						sense, l.lang, l.userID).Scan(&id); err != nil {
						return err
					}
					if _, err := tx.Exec(`INSERT OR IGNORE INTO sm2_progress (word_id) VALUES (?)`, id); err != nil {
						return err
					}
					rank, err := cedictSenseRank(tx, l.lang, sense)
					if err != nil {
						return err
					}
					if _, err := tx.Exec(`INSERT OR IGNORE INTO translations (translation_word_id, zh_word_id, source, rank) VALUES (?, ?, 'cedict', ?)`,
						id, l.zhID, rank); err != nil {
						return err
					}
				}
				if _, err := tx.Exec(`DELETE FROM translations WHERE translation_word_id = ? AND zh_word_id = ?`,
					l.transID, l.zhID); err != nil {
					return err
				}
				var remaining int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM translations WHERE translation_word_id = ?`, l.transID).Scan(&remaining); err != nil {
					return err
				}
				if remaining == 0 {
					if _, err := tx.Exec(`DELETE FROM sm2_progress WHERE word_id = ?`, l.transID); err != nil {
						return err
					}
					if _, err := tx.Exec(`DELETE FROM words WHERE id = ?`, l.transID); err != nil {
						return err
					}
				}
			}
			return tx.Commit()
		},
	})
}

func splitCedictSenses(def string) []string {
	var senses []string
	depth := 0
	start := 0
	flush := func(end int) {
		if s := strings.TrimSpace(def[start:end]); s != "" {
			senses = append(senses, s)
		}
	}
	for i, r := range def {
		switch r {
		case '(', '[', '（':
			depth++
		case ')', ']', '）':
			if depth > 0 {
				depth--
			}
		case ';', ',':
			if depth == 0 {
				flush(i)
				start = i + 1
			}
		}
	}
	flush(len(def))
	return senses
}

func cedictSenseRank(tx *sql.Tx, lang, text string) (sql.NullInt64, error) {
	var worst sql.NullInt64
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && r != '\''
	})
	for _, word := range words {
		var rank int64
		err := tx.QueryRow(`SELECT rank FROM word_frequency_lang WHERE word = ? AND lang = ?`, word, lang).Scan(&rank)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return sql.NullInt64{}, err
		}
		if !worst.Valid || rank > worst.Int64 {
			worst = sql.NullInt64{Int64: rank, Valid: true}
		}
	}
	return worst, nil
}
