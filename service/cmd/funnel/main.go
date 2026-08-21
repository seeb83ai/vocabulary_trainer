// cmd/funnel/main.go — Print the signup → activation → retention funnel.
//
// Reads the users and daily_stats tables and prints how many users reached
// each stage, with conversion percentages. The shared library user (id=1)
// is excluded.
//
// Usage:
//
//	go run ./cmd/funnel [-db data/vocab.db] [-min-attempts 20]
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
	minAttempts := flag.Int("min-attempts", 20, "total attempts required to count as engaged")
	flag.Parse()

	store, err := vocabdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	r, err := store.GetFunnelReport(context.Background(), *minAttempts)
	if err != nil {
		log.Fatalf("funnel report: %v", err)
	}

	pct := func(n, of int) string {
		if of == 0 {
			return "    —"
		}
		return fmt.Sprintf("%4.0f%%", float64(n)/float64(of)*100)
	}

	fmt.Printf("Funnel (library user excluded, engaged = ≥%d attempts)\n\n", *minAttempts)
	fmt.Printf("  %-32s %6s %8s %8s\n", "stage", "users", "of prev", "of reg")
	fmt.Printf("  %-32s %6d %8s %8s\n", "registered", r.Registered, "", "")
	fmt.Printf("  %-32s %6d %8s %8s\n", "verified email", r.Verified, pct(r.Verified, r.Registered), pct(r.Verified, r.Registered))
	fmt.Printf("  %-32s %6d %8s %8s\n", "activated (≥1 training day)", r.Activated, pct(r.Activated, r.Verified), pct(r.Activated, r.Registered))
	fmt.Printf("  %-32s %6d %8s %8s\n", fmt.Sprintf("engaged (≥%d attempts)", *minAttempts), r.Engaged, pct(r.Engaged, r.Activated), pct(r.Engaged, r.Registered))
	fmt.Printf("  %-32s %6d %8s %8s\n", "returned (≥2 training days)", r.Returned, pct(r.Returned, r.Activated), pct(r.Returned, r.Registered))
	fmt.Printf("  %-32s %6d %8s %8s\n", "returned next day (D1)", r.ReturnedDay1, pct(r.ReturnedDay1, r.Activated), pct(r.ReturnedDay1, r.Registered))
}
