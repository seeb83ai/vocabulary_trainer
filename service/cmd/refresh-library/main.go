// cmd/refresh-library/main.go — Refresh the shared library from the dictionaries.
//
// Brings every library (user 1) zh word's glosses in line with cedict_entries
// (CC-CEDICT for en, HanDeDict for de). Run it after cmd/import-cedict loads a
// new dictionary version. Learners with library references see the change at
// once; learners who edited a changed word are asked on the import screen
// whether to keep their version (see ADR-0005).
//
// Re-running is safe: unchanged glosses keep their ids and nothing is marked.
//
// Usage:
//
//	go run ./cmd/refresh-library [-db data/vocab.db]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	vocabdb "vocabulary_trainer/db"
)

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	flag.Parse()

	store, err := vocabdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	report, err := store.RefreshLibrary(context.Background())
	if err != nil {
		log.Fatalf("refresh library: %v", err)
	}
	fmt.Printf("Done. words=%d  changed=%d  added=%d  dropped=%d  missing=%d\n",
		report.Words, report.Changed, report.Added, report.Dropped, report.Missing)
}
