package migrate

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openFixMatchGameFirstSeenAtTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE sm2_progress (
		word_id          INTEGER PRIMARY KEY,
		total_attempts   INTEGER NOT NULL DEFAULT 0,
		first_seen_at    TEXT,
		last_attempt_at  TEXT
	)`); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

// Word answered only via match-game: first_seen_at IS NULL, total_attempts > 0,
// last_attempt_at set. Should be fixed to first_seen_at = last_attempt_at.
func TestFixMatchGameFirstSeenAt_SetsFromLastAttempt(t *testing.T) {
	db := openFixMatchGameFirstSeenAtTestDB(t)
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, total_attempts, first_seen_at, last_attempt_at) VALUES
		(1, 7, NULL, '2026-09-03 18:09:06'),
		(2, 11, NULL, '2026-09-07 10:25:21')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := fixMatchGameFirstSeenAtV20260907120000(db); err != nil {
		t.Fatalf("migration: %v", err)
	}

	for _, tc := range []struct{ id int; want string }{
		{1, "2026-09-03 18:09:06"},
		{2, "2026-09-07 10:25:21"},
	} {
		var got sql.NullString
		if err := db.QueryRow(`SELECT first_seen_at FROM sm2_progress WHERE word_id = ?`, tc.id).Scan(&got); err != nil {
			t.Fatalf("query word %d: %v", tc.id, err)
		}
		if !got.Valid || got.String != tc.want {
			t.Errorf("word %d: expected first_seen_at=%q, got %q (valid=%v)", tc.id, tc.want, got.String, got.Valid)
		}
	}
}

// Word answered only via match-game but last_attempt_at also NULL (old data
// predating the timestamps column): fall back to CURRENT_TIMESTAMP.
func TestFixMatchGameFirstSeenAt_FallsBackToNow(t *testing.T) {
	db := openFixMatchGameFirstSeenAtTestDB(t)
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, total_attempts, first_seen_at, last_attempt_at) VALUES
		(1, 11, NULL, NULL)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := fixMatchGameFirstSeenAtV20260907120000(db); err != nil {
		t.Fatalf("migration: %v", err)
	}

	var got sql.NullString
	if err := db.QueryRow(`SELECT first_seen_at FROM sm2_progress WHERE word_id = 1`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Valid || got.String == "" {
		t.Errorf("expected first_seen_at to be set to a non-empty timestamp, got %q (valid=%v)", got.String, got.Valid)
	}
}

// Words with total_attempts=0 must not be touched (they are genuinely unseen).
func TestFixMatchGameFirstSeenAt_LeavesUnseenWordsAlone(t *testing.T) {
	db := openFixMatchGameFirstSeenAtTestDB(t)
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, total_attempts, first_seen_at, last_attempt_at) VALUES
		(1, 0, NULL, NULL),
		(2, 0, NULL, '2026-09-01 10:00:00')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := fixMatchGameFirstSeenAtV20260907120000(db); err != nil {
		t.Fatalf("migration: %v", err)
	}

	for _, id := range []int{1, 2} {
		var got sql.NullString
		if err := db.QueryRow(`SELECT first_seen_at FROM sm2_progress WHERE word_id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("query word %d: %v", id, err)
		}
		if got.Valid {
			t.Errorf("word %d: expected first_seen_at to remain NULL, got %q", id, got.String)
		}
	}
}

// Words already having first_seen_at set must not be overwritten.
func TestFixMatchGameFirstSeenAt_LeavesAlreadySetAlone(t *testing.T) {
	db := openFixMatchGameFirstSeenAtTestDB(t)
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, total_attempts, first_seen_at, last_attempt_at) VALUES
		(1, 5, '2026-08-01 10:00:00', '2026-09-01 10:00:00')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := fixMatchGameFirstSeenAtV20260907120000(db); err != nil {
		t.Fatalf("migration: %v", err)
	}

	var got sql.NullString
	if err := db.QueryRow(`SELECT first_seen_at FROM sm2_progress WHERE word_id = 1`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Valid || got.String != "2026-08-01 10:00:00" {
		t.Errorf("expected first_seen_at unchanged, got %q (valid=%v)", got.String, got.Valid)
	}
}

func TestFixMatchGameFirstSeenAt_Idempotent(t *testing.T) {
	db := openFixMatchGameFirstSeenAtTestDB(t)
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, total_attempts, first_seen_at, last_attempt_at) VALUES
		(1, 7, NULL, '2026-09-03 18:09:06')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := fixMatchGameFirstSeenAtV20260907120000(db); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var got sql.NullString
	if err := db.QueryRow(`SELECT first_seen_at FROM sm2_progress WHERE word_id = 1`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Valid || got.String != "2026-09-03 18:09:06" {
		t.Errorf("expected first_seen_at=2026-09-03 18:09:06 after repeated runs, got %q", got.String)
	}
}
