package db

import (
	"context"
	"database/sql"
	"strings"
	"unicode"
)

// computeTranslationRank scores an automatically-derived translation gloss by
// looking up each word it contains in word_frequency_lang for lang, and
// returning the rarest (highest/worst) rank among the words that matched.
// Words not found in the frequency list are ignored; the result is invalid
// (unranked) only when none of the gloss's words matched at all.
func computeTranslationRank(ctx context.Context, tx *sql.Tx, lang, text string) (sql.NullInt64, error) {
	var worst sql.NullInt64
	for _, word := range splitFrequencyWords(text) {
		var rank int64
		err := tx.QueryRowContext(ctx,
			`SELECT rank FROM word_frequency_lang WHERE word = ? AND lang = ?`, word, lang,
		).Scan(&rank)
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

// sourceAt returns sources[i] if present, else "user" — the default for a
// translation with no explicit origin (e.g. an older request/client that
// doesn't send TranslationSources at all).
func sourceAt(sources []string, i int) string {
	if i < len(sources) && sources[i] == "cedict" {
		return "cedict"
	}
	return "user"
}

// linkTranslation inserts (or reuses) the translations row connecting transID
// to zhID, computing and storing a frequency rank when source is "cedict"
// (see computeTranslationRank); a "user" translation is always rank 0/exempt.
func linkTranslation(ctx context.Context, tx *sql.Tx, transID, zhID int64, lang, text, source string) error {
	rank := sql.NullInt64{Int64: 0, Valid: true}
	if source == "cedict" {
		var err error
		rank, err = computeTranslationRank(ctx, tx, lang, text)
		if err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO translations (translation_word_id, zh_word_id, source, rank) VALUES (?, ?, ?, ?)`,
		transID, zhID, source, rank)
	return err
}

// splitFrequencyWords lowercases text and splits it into individual words on
// anything that isn't a letter or apostrophe (so "to be able to" -> ["to",
// "be", "able", "to"], "don't" stays one word).
func splitFrequencyWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && r != '\''
	})
}
