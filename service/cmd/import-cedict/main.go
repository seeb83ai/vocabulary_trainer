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
// A run replaces the previous version of that dictionary (entries with
// source 'user', kept from the old curated library, stay), then refreshes
// the shared library so learners see the new translations (ADR-0005). Use
// -append to add a partial file instead.
//
// Usage:
//
//	go run ./cmd/import-cedict -db data/vocab.db -file cedict_ts.u8 -lang en [-dry-run]
//	go run ./cmd/import-cedict -db data/vocab.db -file handedict.u8 -lang de [-dry-run]
//	go run ./cmd/import-cedict -db data/vocab.db -file extra.u8 -lang en -append
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	vocabdb "vocabulary_trainer/db"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

var cedictLineRE = regexp.MustCompile(`^(\S+)\s+(\S+)\s+\[([^\]]*)\]\s+/(.+)/$`)

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	filePath := flag.String("file", "", "path to a CEDICT-format dictionary file (required)")
	lang := flag.String("lang", "", `dictionary language: "en" (CC-CEDICT) or "de" (HanDeDict) (required)`)
	dryRun := flag.Bool("dry-run", false, "parse and validate but do not insert")
	appendOnly := flag.Bool("append", false, "add the entries instead of replacing the previous dictionary version")
	force := flag.Bool("force", false, "replace even when the file is much smaller than the stored dictionary")
	flag.Parse()

	if *filePath == "" {
		log.Fatal("flag -file is required")
	}
	*lang = strings.ToLower(*lang)
	if *lang != "en" && *lang != "de" {
		log.Fatal(`flag -lang is required and must be "en" or "de"`)
	}

	f, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("open file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	var entries []models.DictionaryEntry
	var skipped int
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
		e := models.DictionaryEntry{
			Simplified: m[2],
			Pinyin:     toneMarkPinyin(m[3]),
			Definition: strings.Join(splitDefs(m[4]), "; "),
		}
		if e.Simplified == "" || e.Definition == "" {
			skipped++
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scan error: %v", err)
	}
	if *dryRun {
		log.Printf("Done: would insert %d, skipped %d (lang=%s)", len(entries), skipped, *lang)
		return
	}

	store, err := vocabdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	if !*appendOnly {
		existing, err := store.CountDictionaryEntries(ctx, *lang)
		if err != nil {
			log.Fatalf("count entries: %v", err)
		}
		if err := checkReplaceSize(existing, len(entries), *force); err != nil {
			log.Fatal(err)
		}
	}
	report, err := store.ImportDictionaryEntries(ctx, *lang, entries, !*appendOnly)
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	if err := store.RebuildGlossRank(ctx); err != nil {
		log.Fatalf("rebuild gloss_rank: %v", err)
	}
	log.Printf("Done: inserted %d, removed %d old, skipped %d (lang=%s)", report.Inserted, report.Removed, skipped, *lang)

	// Learners see the new translations through the shared library.
	lib, err := store.RefreshLibrary(ctx)
	if err != nil {
		log.Fatalf("refresh library: %v", err)
	}
	log.Printf("Library: %d words, %d changed, %d glosses added, %d dropped, %d without dictionary entry",
		lib.Words, lib.Changed, lib.Added, lib.Dropped, lib.Missing)
}

// checkReplaceSize refuses to replace a stored dictionary of existing
// entries with a file of incoming entries that is less than half its size:
// that is a partial file, which would wipe the dictionary. force skips the
// check; -append adds a partial file without replacing.
func checkReplaceSize(existing, incoming int, force bool) error {
	if force || existing == 0 || incoming*2 >= existing {
		return nil
	}
	return fmt.Errorf("the file has %d entries, the database has %d: use -append to add a partial file, or -force to replace anyway", incoming, existing)
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
