// cmd/e2e-seed-history/main.go — test-only helper for the landing-page and
// README screenshot generators (make screenshots-landing / screenshots-readme).
//
// The stats page needs months of training history, answer counts spread over
// every proficiency level and a due-date forecast. None of that can be created
// through the REST API (answers are always recorded for "today"), so this tool
// writes it directly via the exported Store.ExecForTest escape hatch.
//
// Usage:
//
//	go run ./cmd/e2e-seed-history -db <path> -email user@example.com [-focus 超市,号]
//
// With -focus, only the listed zh words stay due today (so the training page
// serves one of them next); every other word is due later. Without it, about a
// quarter of the words are due today and the rest are spread over 30 days.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"
	vocabdb "vocabulary_trainer/db"

	_ "modernc.org/sqlite"
)

// lcg is a tiny deterministic generator so every run produces the same history.
type lcg uint32

func (r *lcg) next(n int) int {
	*r = *r*1664525 + 1013904223
	return int((uint32(*r) >> 8) % uint32(n))
}

func main() {
	dbPath := flag.String("db", "", "path to SQLite database")
	email := flag.String("email", "", "email of the user to seed history for")
	focus := flag.String("focus", "", "comma-separated zh words that stay due today (all others are due later)")
	days := flag.Int("days", 190, "days of training history")
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
	if err != nil || user == nil {
		log.Fatalf("user %s not found: %v", *email, err)
	}

	focusSet := map[string]bool{}
	for _, w := range strings.Split(*focus, ",") {
		if w = strings.TrimSpace(w); w != "" {
			focusSet[w] = true
		}
	}

	list, _, err := store.GetWords(context.Background(), user.ID, "", 1, 1000, "zh", "asc", nil, false, false, "", "", "")
	if err != nil {
		log.Fatalf("list words: %v", err)
	}
	type zhWord struct {
		id int64
		zh string
	}
	words := make([]zhWord, 0, len(list))
	for _, w := range list {
		words = append(words, zhWord{w.ID, w.ZhText})
	}

	r := lcg(7)
	today := time.Now().UTC()
	nBuckets := [5]int{}
	for i, w := range words {
		// Spread accuracy so all five levels are populated.
		attempts := 3 + r.next(60)
		acc := 40 + r.next(58)
		switch (i + 1) % 5 {
		case 0:
			attempts, acc = 2, 50 // New
		case 1:
			attempts, acc = 5+r.next(5), 30+r.next(15) // Struggling
		case 2:
			attempts, acc = 12+r.next(20), 55+r.next(14) // Learning
		case 3:
			attempts, acc = 25+r.next(60), 72+r.next(12) // Practicing
		case 4:
			attempts, acc = 40+r.next(70), 88+r.next(10) // Mastered
		}
		correct := attempts * acc / 100
		nBuckets[(i+1)%5]++

		due := today.AddDate(0, 0, 1+r.next(30))
		if len(focusSet) > 0 {
			if focusSet[w.zh] {
				due = today.Add(-time.Hour)
			}
		} else if r.next(4) == 0 {
			due = today.Add(-time.Hour)
		}
		first := today.AddDate(0, 0, -(10 + r.next(*days-10)))
		newWord := 0
		if (i+1)%5 == 0 {
			newWord = 1
		}
		if _, err := store.ExecForTest(`
			UPDATE sm2_progress SET total_attempts = ?, total_correct = ?, repetitions = ?,
			  interval_days = ?, due_date = ?, first_seen_at = ?, learning_new_word = ?
			WHERE word_id = ?`,
			attempts, correct, 1+r.next(6), 1+r.next(20),
			due.Format("2006-01-02 15:04:05"), first.Format("2006-01-02 15:04:05"), newWord, w.id); err != nil {
			log.Fatalf("update progress %s: %v", w.zh, err)
		}
	}

	// Training history: one row per day, words_seen growing to the vocabulary size.
	for d := *days; d >= 1; d-- {
		date := today.AddDate(0, 0, -d).Format("2006-01-02")
		att := 30 + r.next(100)
		mis := att * (12 + r.next(25)) / 100
		f := float64(*days-d) / float64(*days)
		seen := 4 + int(float64(len(words)-4)*f)
		// Levels fill from the top as the history advances: mastered and
		// practicing grow quadratically, new/struggling shrink.
		bMastered := int(float64(nBuckets[4]) * f * f)
		bPracticing := int(float64(nBuckets[3]) * f * f)
		bLearning := int(float64(nBuckets[2]) * f)
		bStruggling := int(float64(nBuckets[1]) * (1.2 - f*0.4))
		bNew := seen - bMastered - bPracticing - bLearning - bStruggling
		if bNew < 0 {
			bNew = 0
		}
		if _, err := store.ExecForTest(`
			INSERT OR REPLACE INTO daily_stats (user_id, date, attempts, mistakes, words_seen,
			  correct_streak, current_streak, bucket_new, bucket_struggling, bucket_learning,
			  bucket_practicing, bucket_mastered, training_seconds)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
			user.ID, date, att, mis, seen, 5+r.next(12),
			bNew, bStruggling, bLearning, bPracticing, bMastered,
			600+r.next(1800)); err != nil {
			log.Fatalf("insert daily_stats %s: %v", date, err)
		}
	}
	// Today's baseline, so a session's answers extend a believable row (and the
	// day streak reaches today).
	if _, err := store.ExecForTest(`
		INSERT OR REPLACE INTO daily_stats (user_id, date, attempts, mistakes, words_seen,
		  correct_streak, current_streak, bucket_new, bucket_struggling, bucket_learning,
		  bucket_practicing, bucket_mastered, training_seconds)
		VALUES (?, date('now'), 48, 11, ?, 9, 0, ?, ?, ?, ?, ?, 900)`,
		user.ID, len(words), nBuckets[0], nBuckets[1], nBuckets[2], nBuckets[3], nBuckets[4]); err != nil {
		log.Fatalf("insert today: %v", err)
	}
	fmt.Printf("Seeded history for %s: %d words, %d days\n", *email, len(words), *days)
}
