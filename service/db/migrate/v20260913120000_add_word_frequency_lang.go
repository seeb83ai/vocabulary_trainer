package migrate

import (
	"bufio"
	"database/sql"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

// frequencyDataEN/DE are 8,000-entry English/German word-frequency lists
// (word<TAB>rank per line, rank 1 = most frequent), derived from
// hermitdave/FrequencyWords (MIT license,
// https://github.com/hermitdave/FrequencyWords), content/2018/en/en_50k.txt
// and content/2018/de/de_50k.txt (OpenSubtitles-2018-based corpora) —
// filtered to alphabetic entries, deduplicated, and truncated to the top
// 8,000 by frequency. Same shape as the existing zh frequencyData in
// v20260826120000_add_word_frequency.go.
//
//go:embed frequency_data_en.txt
var frequencyDataEN string

//go:embed frequency_data_de.txt
var frequencyDataDE string

func init() {
	register(migration{
		version: 20260913120000,
		sql: `
CREATE TABLE IF NOT EXISTS word_frequency_lang (
  word TEXT    NOT NULL,
  lang TEXT    NOT NULL,
  rank INTEGER NOT NULL,
  PRIMARY KEY (word, lang)
);
`,
		fn: importWordFrequencyLang,
	})
}

// importWordFrequencyLang populates word_frequency_lang from the existing
// zh word_frequency table plus the bundled EN/DE lists. Runs once (tracked
// in schema_migrations); the legacy word_frequency table is left in place
// (never dropped, per CLAUDE.md) but is no longer read by application code.
func importWordFrequencyLang(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin word_frequency_lang import: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT word, rank FROM word_frequency`)
	if err != nil {
		return fmt.Errorf("read word_frequency: %w", err)
	}
	type entry struct {
		word string
		rank int
	}
	var zhEntries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.word, &e.rank); err != nil {
			rows.Close()
			return fmt.Errorf("scan word_frequency row: %w", err)
		}
		zhEntries = append(zhEntries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate word_frequency: %w", err)
	}
	for _, e := range zhEntries {
		if err := upsertWordFrequencyLang(tx, e.word, "zh", e.rank); err != nil {
			return err
		}
	}

	if err := importFrequencyLangData(tx, frequencyDataEN, "en"); err != nil {
		return err
	}
	if err := importFrequencyLangData(tx, frequencyDataDE, "de"); err != nil {
		return err
	}

	return tx.Commit()
}

func importFrequencyLangData(tx *sql.Tx, data, lang string) error {
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		word := strings.TrimSpace(parts[0])
		rank, convErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if word == "" || convErr != nil {
			continue
		}
		if err := upsertWordFrequencyLang(tx, word, lang, rank); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func upsertWordFrequencyLang(tx *sql.Tx, word, lang string, rank int) error {
	if _, err := tx.Exec(
		`INSERT INTO word_frequency_lang (word, lang, rank) VALUES (?, ?, ?)
		 ON CONFLICT(word, lang) DO UPDATE SET rank = excluded.rank`,
		word, lang, rank,
	); err != nil {
		return fmt.Errorf("upsert word_frequency_lang %q/%s: %w", word, lang, err)
	}
	return nil
}
