package migrate

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openFixStuckNewBucketTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE sm2_progress (
		word_id INTEGER PRIMARY KEY,
		learning_new_word INTEGER NOT NULL DEFAULT 1,
		interval_days INTEGER NOT NULL DEFAULT 1
	)`); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestFixStuckNewBucketWords_ClearsFlagOnGraduatedInterval(t *testing.T) {
	db := openFixStuckNewBucketTestDB(t)
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, learning_new_word, interval_days) VALUES
		(1, 1, 6),
		(2, 1, 15)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := fixStuckNewBucketWordsV20260905120000(db); err != nil {
		t.Fatalf("migration: %v", err)
	}

	wantInterval := map[int64]int{1: 6, 2: 15}
	for id, want := range wantInterval {
		var learning, interval int
		if err := db.QueryRow(`SELECT learning_new_word, interval_days FROM sm2_progress WHERE word_id = ?`, id).
			Scan(&learning, &interval); err != nil {
			t.Fatalf("query word %d: %v", id, err)
		}
		if learning != 0 {
			t.Errorf("word %d: expected learning_new_word cleared to 0, got %d", id, learning)
		}
		if interval != want {
			t.Errorf("word %d: expected interval_days untouched, got %d", id, interval)
		}
	}
}

func TestFixStuckNewBucketWords_LeavesGenuineNewWordsAlone(t *testing.T) {
	db := openFixStuckNewBucketTestDB(t)
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, learning_new_word, interval_days) VALUES
		(1, 1, 1),
		(2, 0, 1),
		(3, 0, 15)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := fixStuckNewBucketWordsV20260905120000(db); err != nil {
		t.Fatalf("migration: %v", err)
	}

	want := map[int64]int{1: 1, 2: 0, 3: 0}
	for id, wantLearning := range want {
		var learning int
		if err := db.QueryRow(`SELECT learning_new_word FROM sm2_progress WHERE word_id = ?`, id).Scan(&learning); err != nil {
			t.Fatalf("query word %d: %v", id, err)
		}
		if learning != wantLearning {
			t.Errorf("word %d: expected learning_new_word=%d untouched, got %d", id, wantLearning, learning)
		}
	}
}

func TestFixStuckNewBucketWords_Idempotent(t *testing.T) {
	db := openFixStuckNewBucketTestDB(t)
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, learning_new_word, interval_days) VALUES (1, 1, 6)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := fixStuckNewBucketWordsV20260905120000(db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := fixStuckNewBucketWordsV20260905120000(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
	var learning int
	if err := db.QueryRow(`SELECT learning_new_word FROM sm2_progress WHERE word_id = 1`).Scan(&learning); err != nil {
		t.Fatal(err)
	}
	if learning != 0 {
		t.Errorf("expected learning_new_word=0 after repeated runs, got %d", learning)
	}
}
