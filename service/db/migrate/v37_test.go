package migrate

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDB creates an in-memory SQLite DB with only the component_progress
// table — the minimum needed to exercise spreadComponentDueDates.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(OFF)")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE component_progress (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id  INTEGER NOT NULL,
		character TEXT NOT NULL,
		due_date  TEXT NOT NULL,
		UNIQUE(user_id, character)
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// insertComp adds a single component_progress row and returns its id.
func insertComp(t *testing.T, db *sql.DB, userID int64, char, dueDate string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
		userID, char, dueDate,
	)
	if err != nil {
		t.Fatalf("insertComp %q: %v", char, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// dueDatesForUser returns all due_date strings for a user, ordered by id.
func dueDatesForUser(t *testing.T, db *sql.DB, userID int64) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT due_date FROM component_progress WHERE user_id = ? ORDER BY id ASC`,
		userID,
	)
	if err != nil {
		t.Fatalf("query due dates: %v", err)
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			t.Fatalf("scan due_date: %v", err)
		}
		dates = append(dates, d)
	}
	return dates
}

func TestSpreadComponentDueDates_EmptyTable(t *testing.T) {
	db := openTestDB(t)
	if err := spreadComponentDueDates(db); err != nil {
		t.Fatalf("spreadComponentDueDates on empty table: %v", err)
	}
}

func TestSpreadComponentDueDates_NoOverflow(t *testing.T) {
	db := openTestDB(t)
	// Exactly 5 components on the same day — none should move.
	chars := []string{"a", "b", "c", "d", "e"}
	for _, ch := range chars {
		insertComp(t, db, 1, ch, "2026-04-01 00:00:00")
	}

	if err := spreadComponentDueDates(db); err != nil {
		t.Fatalf("spreadComponentDueDates: %v", err)
	}

	dates := dueDatesForUser(t, db, 1)
	for i, d := range dates {
		if d != "2026-04-01 00:00:00" {
			t.Errorf("row %d: want 2026-04-01 00:00:00, got %q", i, d)
		}
	}
}

func TestSpreadComponentDueDates_Overflow(t *testing.T) {
	db := openTestDB(t)
	// 7 components on the same day → first 5 stay, last 2 move to next day.
	chars := []string{"a", "b", "c", "d", "e", "f", "g"}
	for _, ch := range chars {
		insertComp(t, db, 1, ch, "2026-04-01 00:00:00")
	}

	if err := spreadComponentDueDates(db); err != nil {
		t.Fatalf("spreadComponentDueDates: %v", err)
	}

	dates := dueDatesForUser(t, db, 1)
	wantDay1 := 0
	wantDay2 := 0
	for _, d := range dates {
		switch d[:10] {
		case "2026-04-01":
			wantDay1++
		case "2026-04-02":
			wantDay2++
		default:
			t.Errorf("unexpected date %q", d)
		}
	}
	if wantDay1 != 5 {
		t.Errorf("want 5 on 2026-04-01, got %d", wantDay1)
	}
	if wantDay2 != 2 {
		t.Errorf("want 2 on 2026-04-02, got %d", wantDay2)
	}
}

func TestSpreadComponentDueDates_CascadeOverflow(t *testing.T) {
	db := openTestDB(t)
	// 11 on one day → 5 on day 1, 5 on day 2, 1 on day 3.
	for i := 0; i < 11; i++ {
		insertComp(t, db, 1, string(rune('A'+i)), "2026-04-01 00:00:00")
	}

	if err := spreadComponentDueDates(db); err != nil {
		t.Fatalf("spreadComponentDueDates: %v", err)
	}

	counts := map[string]int{}
	for _, d := range dueDatesForUser(t, db, 1) {
		counts[d[:10]]++
	}
	if counts["2026-04-01"] != 5 {
		t.Errorf("want 5 on 2026-04-01, got %d", counts["2026-04-01"])
	}
	if counts["2026-04-02"] != 5 {
		t.Errorf("want 5 on 2026-04-02, got %d", counts["2026-04-02"])
	}
	if counts["2026-04-03"] != 1 {
		t.Errorf("want 1 on 2026-04-03, got %d", counts["2026-04-03"])
	}
}

func TestSpreadComponentDueDates_MultipleUsers(t *testing.T) {
	db := openTestDB(t)
	// User 1: 7 on 2026-04-01 → overflow 2 to 2026-04-02.
	// User 2: 3 on 2026-04-01 → no overflow (slots are per-user).
	for i := 0; i < 7; i++ {
		insertComp(t, db, 1, string(rune('a'+i)), "2026-04-01 00:00:00")
	}
	for i := 0; i < 3; i++ {
		insertComp(t, db, 2, string(rune('A'+i)), "2026-04-01 00:00:00")
	}

	if err := spreadComponentDueDates(db); err != nil {
		t.Fatalf("spreadComponentDueDates: %v", err)
	}

	u1 := map[string]int{}
	for _, d := range dueDatesForUser(t, db, 1) {
		u1[d[:10]]++
	}
	if u1["2026-04-01"] != 5 {
		t.Errorf("user1: want 5 on 2026-04-01, got %d", u1["2026-04-01"])
	}
	if u1["2026-04-02"] != 2 {
		t.Errorf("user1: want 2 on 2026-04-02, got %d", u1["2026-04-02"])
	}

	u2 := map[string]int{}
	for _, d := range dueDatesForUser(t, db, 2) {
		u2[d[:10]]++
	}
	if u2["2026-04-01"] != 3 {
		t.Errorf("user2: want 3 on 2026-04-01 (no overflow), got %d", u2["2026-04-01"])
	}
	if u2["2026-04-02"] != 0 {
		t.Errorf("user2: want 0 on 2026-04-02, got %d", u2["2026-04-02"])
	}
}

func TestSpreadComponentDueDates_OrderPreserved(t *testing.T) {
	db := openTestDB(t)
	// Earlier-dated component should keep its original date; later-dated overflow
	// should move forward rather than displacing the earlier one.
	// 5 on 2026-04-01, then 1 on 2026-04-02 that was already scheduled there.
	// After spread: the 2026-04-02 component should stay on 2026-04-02.
	for i := 0; i < 5; i++ {
		insertComp(t, db, 1, string(rune('a'+i)), "2026-04-01 00:00:00")
	}
	insertComp(t, db, 1, "f", "2026-04-02 00:00:00")

	if err := spreadComponentDueDates(db); err != nil {
		t.Fatalf("spreadComponentDueDates: %v", err)
	}

	counts := map[string]int{}
	for _, d := range dueDatesForUser(t, db, 1) {
		counts[d[:10]]++
	}
	if counts["2026-04-01"] != 5 {
		t.Errorf("want 5 on 2026-04-01, got %d", counts["2026-04-01"])
	}
	if counts["2026-04-02"] != 1 {
		t.Errorf("want 1 on 2026-04-02, got %d", counts["2026-04-02"])
	}
}
