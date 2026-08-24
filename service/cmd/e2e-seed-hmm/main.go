// cmd/e2e-seed-hmm/main.go — test-only helper for the README screenshot
// generator (make screenshots-readme).
//
// A freshly registered user has no hmm_actors/hmm_locations/hmm_tone_rooms
// rows at all (that skeleton was only ever seeded, historically, for the
// single legacy account migrations v27-v29 hardcode as user_id=2) — the
// PUT /api/hmm/{actors,locations,tone-rooms} endpoints only UPDATE existing
// rows, so they 404 for a new user. This tool inserts a few rows directly
// via the exported Store.ExecForTest escape hatch so /mnemonics renders a
// populated library instead of an empty one.
//
// Usage:
//
//	go run ./cmd/e2e-seed-hmm -db <path> -email user@example.com
package main

import (
	"context"
	"flag"
	"log"
	vocabdb "vocabulary_trainer/db"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "path to SQLite database")
	email := flag.String("email", "", "email of the user to seed the HMM library for")
	flag.Parse()

	if *dbPath == "" || *email == "" {
		log.Fatal("both -db and -email are required")
	}

	store, err := vocabdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	user, err := store.GetUserByEmail(context.Background(), *email)
	if err != nil {
		log.Fatalf("get user: %v", err)
	}
	if user == nil {
		log.Fatalf("user %s not found", *email)
	}

	actors := []struct{ initial, category, name, hint string }{
		{"b", "male", "Bruce Lee", "Name starts with 'B'"},
		{"m", "male", "Michael Jordan", "Name starts with 'M'"},
	}
	for _, a := range actors {
		if _, err := store.ExecForTest(
			`INSERT OR REPLACE INTO hmm_actors (user_id, initial, category, actor_name, hint) VALUES (?, ?, ?, ?, ?)`,
			user.ID, a.initial, a.category, a.name, a.hint); err != nil {
			log.Fatalf("seed actor %s: %v", a.initial, err)
		}
	}

	locations := []struct{ final, name string }{
		{"a", "Airport"},
		{"an", "Ancient temple"},
	}
	for _, l := range locations {
		if _, err := store.ExecForTest(
			`INSERT OR REPLACE INTO hmm_locations (user_id, final_key, location_name) VALUES (?, ?, ?)`,
			user.ID, l.final, l.name); err != nil {
			log.Fatalf("seed location %s: %v", l.final, err)
		}
	}

	if _, err := store.ExecForTest(
		`INSERT OR REPLACE INTO hmm_tone_rooms (user_id, tone, room_name) VALUES (?, ?, ?)`,
		user.ID, 1, "Sky-high tower"); err != nil {
		log.Fatalf("seed tone room: %v", err)
	}

	log.Printf("Seeded HMM library entries for user %s", *email)
}
