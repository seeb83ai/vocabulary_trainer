package migrate

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openFixMatchGameIntervalTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE sm2_progress (
		word_id         INTEGER PRIMARY KEY,
		interval_days   INTEGER NOT NULL DEFAULT 1,
		due_date        TEXT    NOT NULL,
		first_seen_at   TEXT,
		last_attempt_at TEXT
	)`); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

// Word whose interval exceeds its age: must be capped to age_in_days.
func TestFixMatchGameInterval_CapsIntervalToAge(t *testing.T) {
	db := openFixMatchGameIntervalTestDB(t)

	firstSeen := time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02 15:04:05")
	lastAttempt := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02 15:04:05")

	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, interval_days, due_date, first_seen_at, last_attempt_at) VALUES
		(1, 3720, '2036-11-03 08:00:00', ?, ?)`, firstSeen, lastAttempt); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := fixMatchGameIntervalV20260907130000(db); err != nil {
		t.Fatalf("migration: %v", err)
	}

	var interval int
	var dueDate string
	if err := db.QueryRow(`SELECT interval_days, due_date FROM sm2_progress WHERE word_id = 1`).Scan(&interval, &dueDate); err != nil {
		t.Fatal(err)
	}

	// Age is ~10 days, so interval must be capped around 10 (allow ±1 for rounding).
	if interval < 9 || interval > 11 {
		t.Errorf("expected interval_days capped to ~10 (word age), got %d", interval)
	}
	// due_date must be in the future (last_attempt + ~10 days from now ≈ ~9 days from now).
	due, err := time.Parse("2006-01-02 15:04:05", dueDate)
	if err != nil {
		t.Fatalf("parse due_date %q: %v", dueDate, err)
	}
	if !due.After(time.Now()) {
		t.Errorf("expected due_date in the future after migration, got %s", dueDate)
	}
}

// Word whose interval is already within its age: must not be touched.
func TestFixMatchGameInterval_LeavesLegitimateIntervalAlone(t *testing.T) {
	db := openFixMatchGameIntervalTestDB(t)

	firstSeen := time.Now().UTC().AddDate(0, 0, -200).Format("2006-01-02 15:04:05")
	lastAttempt := time.Now().UTC().AddDate(0, 0, -5).Format("2006-01-02 15:04:05")
	const originalInterval = 180
	const originalDue = "2027-01-01 00:00:00"

	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, interval_days, due_date, first_seen_at, last_attempt_at) VALUES
		(1, ?, ?, ?, ?)`, originalInterval, originalDue, firstSeen, lastAttempt); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := fixMatchGameIntervalV20260907130000(db); err != nil {
		t.Fatalf("migration: %v", err)
	}

	var interval int
	var dueDate string
	if err := db.QueryRow(`SELECT interval_days, due_date FROM sm2_progress WHERE word_id = 1`).Scan(&interval, &dueDate); err != nil {
		t.Fatal(err)
	}
	if interval != originalInterval {
		t.Errorf("expected interval_days unchanged at %d, got %d", originalInterval, interval)
	}
	if dueDate != originalDue {
		t.Errorf("expected due_date unchanged at %s, got %s", originalDue, dueDate)
	}
}

// Words with first_seen_at IS NULL must not be touched.
func TestFixMatchGameInterval_LeavesUnseenAlone(t *testing.T) {
	db := openFixMatchGameIntervalTestDB(t)

	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, interval_days, due_date, first_seen_at, last_attempt_at) VALUES
		(1, 3720, '2036-01-01 00:00:00', NULL, NULL)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := fixMatchGameIntervalV20260907130000(db); err != nil {
		t.Fatalf("migration: %v", err)
	}

	var interval int
	if err := db.QueryRow(`SELECT interval_days FROM sm2_progress WHERE word_id = 1`).Scan(&interval); err != nil {
		t.Fatal(err)
	}
	if interval != 3720 {
		t.Errorf("expected interval_days unchanged at 3720 for unseen word, got %d", interval)
	}
}

func TestFixMatchGameInterval_Idempotent(t *testing.T) {
	db := openFixMatchGameIntervalTestDB(t)

	firstSeen := time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02 15:04:05")
	lastAttempt := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02 15:04:05")

	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, interval_days, due_date, first_seen_at, last_attempt_at) VALUES
		(1, 3720, '2036-11-03 08:00:00', ?, ?)`, firstSeen, lastAttempt); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := fixMatchGameIntervalV20260907130000(db); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var interval int
	if err := db.QueryRow(`SELECT interval_days FROM sm2_progress WHERE word_id = 1`).Scan(&interval); err != nil {
		t.Fatal(err)
	}
	if interval > 11 {
		t.Errorf("expected interval_days <= 11 after repeated runs, got %d", interval)
	}
}
