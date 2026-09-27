// cmd/import-hsk/main.go — Import an HSK word list into the shared library (user 1).
//
// Reads complete.json from https://github.com/drkameleon/complete-hsk-vocabulary
// (MIT licence) and adds the words of one HSK version to the library user:
//
//	-version 2  HSK 2.0 (levels 1-6)                      → tags hsk2-1 … hsk2-6
//	-version 3  HSK 3.0, 2025 syllabus (levels 1-6, 7-9)  → tags hsk3-1 … hsk3-7
//
// The library stores only zh words, pinyin and tags; translations come from
// cedict_entries when a user imports a list. A zh word that is already in the
// library is not added again; it only gets the new tag. Every tag is marked
// importable with a description such as "HSK 3.0 – Level 1".
//
// Usage:
//
//	go run ./cmd/import-hsk [-db data/vocab.db] [-version 3] [-file complete.json] [-dry-run]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	vocabdb "vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

const defaultURL = "https://raw.githubusercontent.com/drkameleon/complete-hsk-vocabulary/main/complete.json"

const libraryUserID int64 = 1

// levelPrefix maps an HSK version to its level label prefix in complete.json.
// "newest" is the 2025 HSK 3.0 syllabus; "new" (2021) is not used.
var levelPrefix = map[int]string{2: "old-", 3: "newest-"}

type entry struct {
	hanzi  string
	pinyin string
	level  int
}

type result struct {
	created int
	tagged  int
}

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	version := flag.Int("version", 3, "HSK version to import: 2 (HSK 2.0) or 3 (HSK 3.0, 2025 syllabus)")
	file := flag.String("file", "", "read complete.json from this file instead of downloading it")
	url := flag.String("url", defaultURL, "download URL of complete.json")
	dryRun := flag.Bool("dry-run", false, "show what would change but do not write")
	flag.Parse()

	if _, ok := levelPrefix[*version]; !ok {
		log.Fatalf("invalid -version %d: must be 2 or 3", *version)
	}

	var data []byte
	var err error
	if *file != "" {
		data, err = os.ReadFile(*file)
	} else {
		fmt.Printf("Downloading %s\n", *url)
		data, err = download(*url)
	}
	if err != nil {
		log.Fatalf("read word list: %v", err)
	}

	entries, err := parseComplete(data, *version)
	if err != nil {
		log.Fatalf("parse word list: %v", err)
	}
	fmt.Printf("Parsed %d HSK %d words\n", len(entries), *version)

	store, err := vocabdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	res, err := importLibrary(context.Background(), store, entries, *version, *dryRun)
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	fmt.Printf("\nDone. created=%d  tagged=%d\n", res.created, res.tagged)
	if *dryRun {
		fmt.Println("(dry-run: no changes were written)")
	}
}

func download(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// parseComplete returns the words of complete.json that belong to the given
// HSK version, in file order. A word with more than one level label for the
// version gets the lowest level.
func parseComplete(data []byte, version int) ([]entry, error) {
	var raw []struct {
		Simplified string   `json:"simplified"`
		Level      []string `json:"level"`
		Forms      []struct {
			Transcriptions struct {
				Pinyin string `json:"pinyin"`
			} `json:"transcriptions"`
		} `json:"forms"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	prefix := levelPrefix[version]
	var out []entry
	for _, r := range raw {
		level := 0
		for _, l := range r.Level {
			if !strings.HasPrefix(l, prefix) {
				continue
			}
			n, err := strconv.Atoi(strings.TrimPrefix(l, prefix))
			if err != nil {
				continue
			}
			if level == 0 || n < level {
				level = n
			}
		}
		if level == 0 || strings.TrimSpace(r.Simplified) == "" {
			continue
		}
		e := entry{hanzi: strings.TrimSpace(r.Simplified), level: level}
		if len(r.Forms) > 0 {
			e.pinyin = r.Forms[0].Transcriptions.Pinyin
		}
		out = append(out, e)
	}
	return out, nil
}

// tagName returns the library tag for an HSK level, e.g. "hsk3-1". Level 7
// of HSK 3.0 stands for the combined band 7-9.
func tagName(version, level int) string {
	return fmt.Sprintf("hsk%d-%d", version, level)
}

func tagDescription(version, level int) string {
	if version == 3 && level == 7 {
		return "HSK 3.0 – Levels 7–9"
	}
	return fmt.Sprintf("HSK %d.0 – Level %d", version, level)
}

// importLibrary adds the entries to the library user. Existing zh words only
// get the level tag; missing ones are created with pinyin and the tag.
func importLibrary(ctx context.Context, store *vocabdb.Store, entries []entry, version int, dryRun bool) (result, error) {
	var res result
	existing, _, err := store.GetWords(ctx, libraryUserID, "", 1, 0, "", "", nil, false, false, "", "", "")
	if err != nil {
		return res, fmt.Errorf("load library words: %w", err)
	}
	existingIDs := make(map[string]int64, len(existing))
	for _, w := range existing {
		existingIDs[w.ZhText] = w.ID
	}

	levels := map[int]bool{}
	for _, e := range entries {
		tag := tagName(version, e.level)
		levels[e.level] = true
		if id, ok := existingIDs[e.hanzi]; ok {
			res.tagged++
			if dryRun {
				continue
			}
			if err := store.AddWordTags(ctx, libraryUserID, id, []string{tag}); err != nil {
				return res, fmt.Errorf("tag %q: %w", e.hanzi, err)
			}
			continue
		}
		res.created++
		if dryRun {
			fmt.Printf("  [DRY]   would create: %s (%s) %s\n", e.hanzi, e.pinyin, tag)
			continue
		}
		id, err := store.CreateWord(ctx, libraryUserID, models.CreateWordRequest{
			ZhText: e.hanzi,
			Pinyin: e.pinyin,
			Tags:   []string{tag},
		})
		if err != nil {
			return res, fmt.Errorf("create %q: %w", e.hanzi, err)
		}
		existingIDs[e.hanzi] = id
	}

	if dryRun {
		return res, nil
	}
	for level := range levels {
		if err := store.UpsertTagMeta(ctx, libraryUserID, tagName(version, level), tagDescription(version, level), true); err != nil {
			return res, fmt.Errorf("tag meta: %w", err)
		}
	}
	return res, nil
}
