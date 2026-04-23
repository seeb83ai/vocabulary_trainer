// cmd/import-hanzi/main.go — Import makemeahanzi dictionary.txt into the trainer DB.
//
// Each line of dictionary.txt is a JSON object with fields: character, radical,
// decomposition, definition, etymology, pinyin, matches.
// We store character, radical, decomposition, definition, etymology (as JSON
// string) and pinyin (as JSON-encoded string array).
//
// Usage:
//
//	go run ./cmd/import-hanzi [-db data/vocab.db] -file dictionary.txt [-dry-run]
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	vocabdb "vocabulary_trainer/db"

	_ "modernc.org/sqlite"
)

type dictEntry struct {
	Character     string          `json:"character"`
	Radical       string          `json:"radical"`
	Decomposition string          `json:"decomposition"`
	Definition    string          `json:"definition"`
	Etymology     json.RawMessage `json:"etymology"`
	Pinyin        []string        `json:"pinyin"`
}

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	filePath := flag.String("file", "", "path to makemeahanzi dictionary.txt (required)")
	dryRun := flag.Bool("dry-run", false, "parse and validate but do not insert")
	flag.Parse()

	if *filePath == "" {
		log.Fatal("flag -file is required")
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

	f, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("open file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	var inserted, skipped, failed int

	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("begin tx: %v", err)
	}

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO hanzi_decomposition
		(character, definition, radical, decomposition, etymology, pinyin) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		log.Fatalf("prepare: %v", err)
	}
	defer stmt.Close()

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var e dictEntry
		if err := json.Unmarshal(line, &e); err != nil {
			log.Printf("WARN: skip malformed line: %v", err)
			failed++
			continue
		}

		if e.Character == "" {
			skipped++
			continue
		}

		if *dryRun {
			inserted++
			continue
		}

		var etymStr *string
		if len(e.Etymology) > 0 && string(e.Etymology) != "null" {
			s := string(e.Etymology)
			etymStr = &s
		}

		var pinyinStr *string
		if len(e.Pinyin) > 0 {
			b, err := json.Marshal(e.Pinyin)
			if err != nil {
				log.Printf("WARN: marshal pinyin for %q: %v", e.Character, err)
				failed++
				continue
			}
			s := string(b)
			pinyinStr = &s
		}

		if _, err := stmt.Exec(e.Character, nullStr(e.Definition), nullStr(e.Radical),
			nullStr(e.Decomposition), etymStr, pinyinStr); err != nil {
			log.Printf("WARN: insert %q: %v", e.Character, err)
			failed++
			continue
		}
		inserted++
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("scan error: %v", err)
	}

	if !*dryRun {
		if err := tx.Commit(); err != nil {
			log.Fatalf("commit: %v", err)
		}
	}

	action := "inserted"
	if *dryRun {
		action = "would insert"
	}
	log.Printf("Done: %s %d, skipped %d, failed %d", action, inserted, skipped, failed)
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
