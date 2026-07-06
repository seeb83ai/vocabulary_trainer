// cmd/e2e-seed-pinyin/main.go — test-only helper for the README screenshot
// generator (make screenshots).
//
// There is no REST endpoint for creating pinyin_sounds rows (only the
// cmd/import-pinyin CLI, which requires real MP3 files). This tool seeds a
// handful of sounds directly via the exported Store.InsertPinyinSound method
// so /pinyin renders a real quiz card instead of the empty state.
//
// Usage:
//
//	go run ./cmd/e2e-seed-pinyin -db <path> -email user@example.com
package main

import (
	"context"
	"flag"
	"log"
	vocabdb "vocabulary_trainer/db"
	"vocabulary_trainer/models"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "path to SQLite database")
	email := flag.String("email", "", "email of the user to seed pinyin progress for")
	flag.Parse()

	if *dbPath == "" || *email == "" {
		log.Fatal("both -db and -email are required")
	}

	store, err := vocabdb.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	user, err := store.GetUserByEmail(ctx, *email)
	if err != nil {
		log.Fatalf("get user: %v", err)
	}
	if user == nil {
		log.Fatalf("user %s not found", *email)
	}

	sounds := []models.PinyinSound{
		{Initial: "b", Final: "a", Tone: 1, Syllable: "ba", Filename: "ba1.mp3", Tag: "b_p_m_f"},
		{Initial: "b", Final: "a", Tone: 3, Syllable: "ba", Filename: "ba3.mp3", Tag: "b_p_m_f"},
		{Initial: "m", Final: "a", Tone: 1, Syllable: "ma", Filename: "ma1.mp3", Tag: "b_p_m_f"},
		{Initial: "m", Final: "a", Tone: 3, Syllable: "ma", Filename: "ma3.mp3", Tag: "b_p_m_f"},
		{Initial: "zh", Final: "ong", Tone: 1, Syllable: "zhong", Filename: "zhong1.mp3", Tag: "zh_ch_sh_r"},
		{Initial: "x", Final: "ie", Tone: 4, Syllable: "xie", Filename: "xie4.mp3", Tag: "j_q_x"},
	}

	for _, s := range sounds {
		if _, err := store.InsertPinyinSound(ctx, user.ID, s); err != nil {
			log.Fatalf("insert sound %s: %v", s.Syllable, err)
		}
	}

	log.Printf("Seeded %d pinyin sounds for user %s", len(sounds), *email)
}
