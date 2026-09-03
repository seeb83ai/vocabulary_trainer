// cmd/import-cedict/main.go — Import a CEDICT-format dictionary into the
// trainer DB. Run once for each dictionary: CC-CEDICT (-lang en) and
// HanDeDict (-lang de). Both are distributed in the same line format, since
// HanDeDict began life as a translation of CC-CEDICT:
//
//	traditional simplified [pin1 yin1] /definition 1/definition 2/.../
//
// We store simplified text only (this app is simplified-only), pinyin
// converted from CEDICT's numbered form to tone-mark form (matching the
// convention used everywhere else in the app), and all /-delimited
// definitions joined into one display string.
//
// Usage:
//
//	go run ./cmd/import-cedict -db data/vocab.db -file cedict_ts.u8 -lang en [-dry-run]
//	go run ./cmd/import-cedict -db data/vocab.db -file handedict.u8 -lang de [-dry-run]
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	vocabdb "vocabulary_trainer/db"
	"vocabulary_trainer/sm2"

	_ "modernc.org/sqlite"
)

var cedictLineRE = regexp.MustCompile(`^(\S+)\s+(\S+)\s+\[([^\]]*)\]\s+/(.+)/$`)

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	filePath := flag.String("file", "", "path to a CEDICT-format dictionary file (required)")
	lang := flag.String("lang", "", `dictionary language: "en" (CC-CEDICT) or "de" (HanDeDict) (required)`)
	dryRun := flag.Bool("dry-run", false, "parse and validate but do not insert")
	flag.Parse()

	if *filePath == "" {
		log.Fatal("flag -file is required")
	}
	*lang = strings.ToLower(*lang)
	if *lang != "en" && *lang != "de" {
		log.Fatal(`flag -lang is required and must be "en" or "de"`)
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

	stmt, err := tx.Prepare(`INSERT INTO cedict_entries (simplified, lang, pinyin, definition) VALUES (?, ?, ?, ?)`)
	if err != nil {
		log.Fatalf("prepare: %v", err)
	}
	defer stmt.Close()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		m := cedictLineRE.FindStringSubmatch(line)
		if m == nil {
			skipped++
			continue
		}
		simplified := m[2]
		pinyin := toneMarkPinyin(m[3])
		definition := strings.Join(splitDefs(m[4]), "; ")

		if simplified == "" || definition == "" {
			skipped++
			continue
		}

		if *dryRun {
			inserted++
			continue
		}

		if _, err := stmt.Exec(simplified, *lang, pinyin, definition); err != nil {
			log.Printf("WARN: insert %q: %v", simplified, err)
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
	log.Printf("Done: %s %d, skipped %d, failed %d (lang=%s)", action, inserted, skipped, failed, *lang)
}

// splitDefs splits a CEDICT "/def1/def2/.../" body (already stripped of the
// surrounding slashes) on "/", dropping empty entries.
func splitDefs(body string) []string {
	parts := strings.Split(body, "/")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// toneMarkPinyin converts a CEDICT bracketed pinyin field (space-separated
// numbered syllables, e.g. "ti1 zu2 qiu2") to tone-mark form ("tī zú qiú")
// using the same conversion the rest of the app uses. CEDICT spells the ü
// vowel as "u:" (e.g. "nu:3"); NumberedToToneMark expects "v" for ü, so that
// substitution happens first.
func toneMarkPinyin(raw string) string {
	syllables := strings.Fields(raw)
	out := make([]string, 0, len(syllables))
	for _, syl := range syllables {
		syl = strings.ReplaceAll(syl, "u:", "v")
		n := len(syl)
		if n == 0 {
			continue
		}
		toneDigit := syl[n-1]
		if toneDigit < '1' || toneDigit > '5' {
			out = append(out, syl)
			continue
		}
		tone, err := strconv.Atoi(string(toneDigit))
		if err != nil {
			out = append(out, syl)
			continue
		}
		out = append(out, sm2.NumberedToToneMark(syl[:n-1], tone))
	}
	return strings.Join(out, " ")
}
