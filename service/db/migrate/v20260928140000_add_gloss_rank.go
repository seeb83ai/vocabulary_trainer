package migrate

import (
	"database/sql"
	"regexp"
	"strings"
	"unicode"
)

// v20260928140000 adds gloss_rank, a cache of the frequency rank of every
// dictionary gloss (one sense of a cedict_entries definition), keyed by
// (lang, gloss). The rank of a gloss depends only on its text and
// word_frequency_lang, so it is computed once here instead of once per
// imported word and user. A NULL rank means no word of the gloss is in the
// frequency list. RebuildGlossRank must run again whenever cedict_entries or
// word_frequency_lang change (the import-cedict and import-frequency tools do).
func init() {
	register(migration{
		version: 20260928140000,
		fn: func(db *sql.DB) error {
			if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS gloss_rank (
  lang  TEXT NOT NULL,
  gloss TEXT NOT NULL,
  rank  INTEGER,
  PRIMARY KEY (lang, gloss)
)`); err != nil {
				return err
			}
			return RebuildGlossRank(db)
		},
	})
}

var parenthesised = regexp.MustCompile(`\([^()]*\)`)

// FrequencyWords lowercases a gloss and splits it into the words that are
// looked up in word_frequency_lang, on anything that isn't a letter or
// apostrophe ("to be able to" -> ["to", "be", "able", "to"], "don't" stays one
// word). Text in parentheses — a HanDeDict tag such as "(V)" or a CEDICT
// explanation — is ignored, unless the gloss has no other words (issue #658).
func FrequencyWords(text string) []string {
	if words := splitWords(parenthesised.ReplaceAllString(text, " ")); len(words) > 0 {
		return words
	}
	return splitWords(text)
}

func splitWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && r != '\''
	})
}

// RebuildGlossRank replaces the contents of gloss_rank with the rank of every
// en/de sense found in cedict_entries. The split and rank logic mirrors
// db.splitSenses and db.computeTranslationRank (both use FrequencyWords).
func RebuildGlossRank(db *sql.DB) error {
	freq, err := loadFrequencyRanks(db)
	if err != nil {
		return err
	}

	type gloss struct{ lang, text string }
	glosses := map[gloss]struct{}{}
	rows, err := db.Query(`SELECT lang, definition FROM cedict_entries WHERE lang IN ('en', 'de')`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var lang, def string
		if err := rows.Scan(&lang, &def); err != nil {
			rows.Close()
			return err
		}
		for _, sense := range splitCedictSenses(def) {
			glosses[gloss{lang, sense}] = struct{}{}
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
	if _, err := tx.Exec(`DELETE FROM gloss_rank`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO gloss_rank (lang, gloss, rank) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for g := range glosses {
		if _, err := stmt.Exec(g.lang, g.text, glossRank(freq[g.lang], g.text)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// loadFrequencyRanks reads the en/de rows of word_frequency_lang as
// lang -> word -> rank.
func loadFrequencyRanks(db *sql.DB) (map[string]map[string]int64, error) {
	freq := map[string]map[string]int64{"en": {}, "de": {}}
	rows, err := db.Query(`SELECT word, lang, rank FROM word_frequency_lang WHERE lang IN ('en', 'de')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var word, lang string
		var rank int64
		if err := rows.Scan(&word, &lang, &rank); err != nil {
			return nil, err
		}
		freq[lang][word] = rank
	}
	return freq, rows.Err()
}

// glossRank is the rank of the rarest word of text found in freq, or invalid
// when none is found (mirrors db.computeTranslationRank).
func glossRank(freq map[string]int64, text string) sql.NullInt64 {
	var rank sql.NullInt64
	for _, word := range FrequencyWords(text) {
		if r, ok := freq[word]; ok && (!rank.Valid || r > rank.Int64) {
			rank = sql.NullInt64{Int64: r, Valid: true}
		}
	}
	return rank
}
