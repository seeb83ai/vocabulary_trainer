// cmd/import-topics/main.go — Import the topic word lists into the shared library.
//
// Reads every <dir>/<topic>.csv written by cmd/classify-topics (columns zh,en;
// _none.csv is skipped) and tags each word in the shared library (user 1) with
// "topic-<topic>". Words the library does not have yet are created with their
// CC-CEDICT pinyin, like cmd/import-hsk does. Every topic tag is marked
// importable, so users see the lists in the existing import pickers;
// translations come from CEDICT/HanDeDict when a user imports a list.
//
// Re-running is safe: it only adds missing tags and words.
//
// Usage:
//
//	go run ./cmd/import-topics [-db data/vocab.db] [-dir data/topics] [-dry-run]
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	vocabdb "vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

const libraryUserID int64 = 1

type result struct {
	topics  int // topic files read
	tagged  int // (word, topic) rows tagged
	created int // words added to the library
}

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	dir := flag.String("dir", "data/topics", "directory with the <topic>.csv files from classify-topics")
	dryRun := flag.Bool("dry-run", false, "show what would change without writing")
	flag.Parse()

	store, err := vocabdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	res, err := importTopics(context.Background(), store, *dir, *dryRun)
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	fmt.Printf("\nDone. topics=%d  tagged=%d  created=%d\n", res.topics, res.tagged, res.created)
	if *dryRun {
		fmt.Println("(dry-run: no changes were written)")
	}
}

func importTopics(ctx context.Context, store *vocabdb.Store, dir string, dryRun bool) (result, error) {
	var res result
	paths, err := filepath.Glob(filepath.Join(dir, "*.csv"))
	if err != nil {
		return res, err
	}
	existing, _, err := store.GetWords(ctx, libraryUserID, "", 1, 0, "", "", nil, false, false, "", "", "")
	if err != nil {
		return res, fmt.Errorf("load library words: %w", err)
	}
	existingIDs := make(map[string]int64, len(existing))
	for _, w := range existing {
		existingIDs[w.ZhText] = w.ID
	}

	for _, path := range paths {
		slug := strings.TrimSuffix(filepath.Base(path), ".csv")
		if strings.HasPrefix(slug, "_") {
			continue
		}
		words, err := readWords(path)
		if err != nil {
			return res, err
		}
		tag := "topic-" + slug
		res.topics++
		for _, zh := range words {
			res.tagged++
			if id, ok := existingIDs[zh]; ok {
				if dryRun {
					continue
				}
				if err := store.AddWordTags(ctx, libraryUserID, id, []string{tag}); err != nil {
					return res, fmt.Errorf("tag %q: %w", zh, err)
				}
				continue
			}
			res.created++
			pinyin, err := store.LookupPinyin(ctx, zh)
			if err != nil {
				return res, err
			}
			// Library pinyin is written without spaces between syllables.
			pinyin = strings.ReplaceAll(pinyin, " ", "")
			if dryRun {
				fmt.Printf("  [DRY]   would create: %s (%s) %s\n", zh, pinyin, tag)
				existingIDs[zh] = 0
				continue
			}
			id, err := store.CreateWord(ctx, libraryUserID, models.CreateWordRequest{ZhText: zh, Pinyin: pinyin, Tags: []string{tag}})
			if err != nil {
				return res, fmt.Errorf("create %q: %w", zh, err)
			}
			existingIDs[zh] = id
		}
		if !dryRun {
			if err := store.UpsertTagMeta(ctx, libraryUserID, tag, "Topic: "+slug, true); err != nil {
				return res, fmt.Errorf("tag meta %s: %w", tag, err)
			}
		}
		fmt.Printf("  %-12s %d words\n", slug, len(words))
	}
	return res, nil
}

// readWords returns the zh column of a classify-topics CSV, without the header.
func readWords(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var words []string
	for i, r := range recs {
		if i > 0 && len(r) > 0 && r[0] != "" {
			words = append(words, r[0])
		}
	}
	return words, nil
}
