// cmd/import-frequency/main.go — Import a Chinese word-frequency list into the
// standalone word_frequency reference table.
//
// The table is keyed by the hanzi string (word_frequency(word, rank)) and is
// independent of any user's vocabulary — it only annotates GetNextCard's new-word
// ordering (see issue #340: frequent words are introduced before rare ones).
// Importing does not create or modify any words; a frequency entry for a word not
// in a user's vocabulary is simply unused until that word is added.
//
// The bundled frequency_data.txt (rank 1 = most frequent) is derived from
// hermitdave/FrequencyWords (MIT license, https://github.com/hermitdave/FrequencyWords),
// content/2018/zh_cn/zh_cn_50k.txt, based on an OpenSubtitles 2018 corpus — filtered
// to entries consisting solely of CJK ideographs, deduplicated, top 8000 by frequency.
//
// Usage:
//
//	go run ./cmd/import-frequency [-db data/vocab.db] [-file frequency_data.txt] [-dry-run]
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	vocabdb "vocabulary_trainer/db"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	filePath := flag.String("file", "frequency_data.txt", "path to word-frequency list (word<TAB>rank per line)")
	dryRun := flag.Bool("dry-run", false, "parse and validate but do not write to the database")
	flag.Parse()

	f, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("open file: %v", err)
	}
	defer f.Close()

	entries, skipped, err := parse(f)
	if err != nil {
		log.Fatalf("parse %s: %v", *filePath, err)
	}
	fmt.Printf("Parsed %d entries (%d skipped/malformed lines)\n", len(entries), skipped)

	if *dryRun {
		fmt.Println("(dry-run: no changes were written)")
		return
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", *dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := vocabdb.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	inserted, err := importEntries(db, entries)
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	fmt.Printf("Done. wrote %d word_frequency rows\n", inserted)
}

type freqEntry struct {
	word string
	rank int
}

// parse reads "word<TAB>rank" lines, skipping blank lines and lines starting
// with '#' (comments, used for provenance notes in the bundled data file).
func parse(f *os.File) (entries []freqEntry, skipped int, err error) {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			skipped++
			continue
		}
		word := strings.TrimSpace(parts[0])
		rank, convErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if word == "" || convErr != nil {
			skipped++
			continue
		}
		entries = append(entries, freqEntry{word: word, rank: rank})
	}
	if err := scanner.Err(); err != nil {
		return nil, skipped, err
	}
	return entries, skipped, nil
}

// importEntries upserts every entry into word_frequency and returns the count written.
func importEntries(db *sql.DB, entries []freqEntry) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	for _, e := range entries {
		if _, err := tx.Exec(
			`INSERT INTO word_frequency (word, rank) VALUES (?, ?)
			 ON CONFLICT(word) DO UPDATE SET rank = excluded.rank`,
			e.word, e.rank); err != nil {
			return 0, fmt.Errorf("upsert %q: %w", e.word, err)
		}
	}
	return len(entries), tx.Commit()
}
