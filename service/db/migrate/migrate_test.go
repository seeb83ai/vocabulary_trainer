package migrate

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestMain sets env vars so the v20 admin/template-user migration runs non-interactively.
func TestMain(m *testing.M) {
	os.Setenv("ADMIN_EMAIL", "admin@example.de")
	os.Setenv("ADMIN_PASSWORD", "I am the admin")
	os.Setenv("USER_EMAIL", "me@example.de")
	os.Setenv("USER_PASSWORD", "I learn zh")
	os.Setenv("BCRYPT_COST", "min")
	os.Exit(m.Run())
}

// openRawDB opens a fresh per-test in-memory SQLite database.
func openRawDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// maxRegisteredVersion returns the highest registered migration version.
func maxRegisteredVersion() int64 {
	var max int64
	for _, m := range registry {
		if m.version > max {
			max = m.version
		}
	}
	return max
}

// appliedVersion returns the highest version recorded in schema_migrations, or 0.
func appliedVersion(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		t.Fatalf("read schema_migrations max version: %v", err)
	}
	return v
}

// migrateUpTo applies registered migrations up to and including target,
// creating schema_migrations directly (simulates a DB frozen at a past version).
func migrateUpTo(t *testing.T, db *sql.DB, target int64) {
	t.Helper()
	sorted := append([]migration(nil), registry...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].version < sorted[j].version })

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for _, m := range sorted {
		if m.version > target {
			break
		}
		if m.sql != "" {
			if _, err := db.Exec(m.sql); err != nil {
				t.Fatalf("migrate up to %d: migration %d sql: %v", target, m.version, err)
			}
		}
		if m.fn != nil {
			if err := m.fn(db); err != nil {
				t.Fatalf("migrate up to %d: migration %d fn: %v", target, m.version, err)
			}
		}
		if _, err := db.Exec(`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, m.version); err != nil {
			t.Fatalf("record migration %d in migrateUpTo: %v", m.version, err)
		}
	}
}

func TestNoDuplicateMigrationVersions(t *testing.T) {
	if len(registry) == 0 {
		t.Fatal("registry is empty")
	}
	seen := make(map[int64]int)
	for _, m := range registry {
		seen[m.version]++
	}
	for v, count := range seen {
		if count > 1 {
			t.Errorf("migration version %d registered %d times", v, count)
		}
	}
}

func TestMigrateBootstrapsFromOldSchemaVersion(t *testing.T) {
	db := openRawDB(t)
	// Apply the real schema up to v51 using the new schema_migrations helper,
	// then swap it for the old schema_version table to simulate an existing DB
	// that was last written by the pre-bootstrap migration runner.
	migrateUpTo(t, db, 51)
	db.Exec(`DROP TABLE schema_migrations`)
	db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL DEFAULT 0)`)
	db.Exec(`INSERT INTO schema_version (version) VALUES (51)`)

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// schema_version must be gone.
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'`).Scan(&n)
	if n != 0 {
		t.Error("schema_version table still exists after bootstrap")
	}

	// schema_migrations must contain v01 (version 1).
	db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&n)
	if n != 1 {
		t.Error("schema_migrations missing version 1 after bootstrap")
	}
}

// TestMigrate_FullChainFromEmpty runs every migration on a fresh DB and verifies
// the applied set matches the full registry.
func TestMigrate_FullChainFromEmpty(t *testing.T) {
	db := openRawDB(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate from empty: %v", err)
	}
	if got, want := appliedVersion(t, db), maxRegisteredVersion(); got != want {
		t.Fatalf("max applied version after full chain = %d, want %d", got, want)
	}
	for _, table := range []string{"words", "sm2_progress", "user_settings", "schema_migrations"} {
		if _, err := db.Exec("SELECT 1 FROM " + table + " LIMIT 1"); err != nil {
			t.Errorf("expected table %q to exist after migration: %v", table, err)
		}
	}
}

// TestMigrate_FromMidHistorySnapshot freezes a DB at v17, seeds a row, then
// runs the full chain — verifying table-recreation migrations (v18, v21) preserve data.
func TestMigrate_FromMidHistorySnapshot(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 17)

	if got := appliedVersion(t, db); got != 17 {
		t.Fatalf("snapshot max applied version = %d, want 17", got)
	}

	if _, err := db.Exec(`INSERT INTO words (text, language) VALUES ('__migrate_test_marker__', 'zh')`); err != nil {
		t.Fatalf("seed word at snapshot: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate from mid-history snapshot: %v", err)
	}
	if got, want := appliedVersion(t, db), maxRegisteredVersion(); got != want {
		t.Fatalf("max applied version after upgrade = %d, want %d", got, want)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM words WHERE text = '__migrate_test_marker__'`).Scan(&count); err != nil {
		t.Fatalf("count words after upgrade: %v", err)
	}
	if count < 1 {
		t.Errorf("seeded word did not survive table-recreation migrations: count=%d", count)
	}
}

// TestMigrate_ConfusionPairsGeneralizationPreservesData is a regression test
// for v20260802103000_generalize_confusion_pairs.go (PR #281 review round 1,
// finding #2): that migration recreates confusion_pairs (drop FK, rebuild via
// CREATE .../INSERT ... SELECT/DROP/RENAME) to add zh_component/
// confused_with_component/user_id. This freezes a DB at v64 (the last version
// before the generalization), seeds an old-schema confusion_pairs row —
// exactly the shape that existed before this PR — then runs the full chain
// and asserts the row survives the rebuild with its original
// user_id/zh_word_id/confused_with_id/mode/count/last_seen values intact.
func TestMigrate_ConfusionPairsGeneralizationPreservesData(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 64)

	if got := appliedVersion(t, db); got != 64 {
		t.Fatalf("snapshot max applied version = %d, want 64", got)
	}

	// words.user_id (added in v21) is NOT NULL; TestMain's ADMIN_EMAIL/USER_EMAIL
	// env vars make v20's fn seed user_id=2 as the personal user, so seed the
	// pre-migration confusion_pairs row's words under that real user.
	if _, err := db.Exec(`INSERT INTO words (text, language, user_id) VALUES ('你好', 'zh', 2)`); err != nil {
		t.Fatalf("seed zh word: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO words (text, language, user_id) VALUES ('您好', 'zh', 2)`); err != nil {
		t.Fatalf("seed confused-with word: %v", err)
	}
	var zhID, confusedID int64
	if err := db.QueryRow(`SELECT id FROM words WHERE text = '你好'`).Scan(&zhID); err != nil {
		t.Fatalf("lookup zh word id: %v", err)
	}
	if err := db.QueryRow(`SELECT id FROM words WHERE text = '您好'`).Scan(&confusedID); err != nil {
		t.Fatalf("lookup confused-with word id: %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO confusion_pairs (zh_word_id, confused_with_id, mode, count, last_seen)
		 VALUES (?, ?, 'zh_to_transl', 3, '2026-07-01 12:00:00')`,
		zhID, confusedID); err != nil {
		t.Fatalf("seed old-schema confusion_pairs row: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate from v64 snapshot: %v", err)
	}
	if got, want := appliedVersion(t, db), maxRegisteredVersion(); got != want {
		t.Fatalf("max applied version after upgrade = %d, want %d", got, want)
	}

	var userID, count int64
	var mode, lastSeen string
	err := db.QueryRow(
		`SELECT user_id, mode, count, last_seen FROM confusion_pairs
		 WHERE zh_word_id = ? AND zh_component = '' AND confused_with_id = ? AND confused_with_component = ''`,
		zhID, confusedID).Scan(&userID, &mode, &count, &lastSeen)
	if err != nil {
		t.Fatalf("read migrated confusion_pairs row: %v", err)
	}
	if userID != 2 {
		t.Errorf("user_id = %d, want 2", userID)
	}
	if mode != "zh_to_transl" {
		t.Errorf("mode = %q, want zh_to_transl", mode)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
	// The sqlite driver reformats DATETIME-typed columns on scan (e.g. to
	// RFC3339), so compare parsed layouts rather than the raw string — mirrors
	// how the db package itself always reads datetime columns via parseDateTime.
	wantLastSeen, err := time.Parse("2006-01-02 15:04:05", "2026-07-01 12:00:00")
	if err != nil {
		t.Fatalf("parse want last_seen: %v", err)
	}
	gotLastSeen, err := time.Parse(time.RFC3339, lastSeen)
	if err != nil {
		gotLastSeen, err = time.Parse("2006-01-02 15:04:05", lastSeen)
		if err != nil {
			t.Fatalf("parse got last_seen %q: %v", lastSeen, err)
		}
	}
	if !gotLastSeen.Equal(wantLastSeen) {
		t.Errorf("last_seen = %q (parsed %v), want %v", lastSeen, gotLastSeen, wantLastSeen)
	}
}

// TestMigrate_OutOfOrder verifies that a migration with a version lower than one
// already applied is still executed on the next run — the core guarantee of
// per-migration tracking over a high-watermark approach.
//
// Sequence:
//  1. Register migration 9000; run Migrate → only 9000 executes.
//  2. Add migrations 8999 (earlier) and 9001 (later); run Migrate again.
//  3. Both 8999 and 9001 must execute; 9000 must not re-run.
func TestMigrate_OutOfOrder(t *testing.T) {
	// Isolate: swap registry for a synthetic set; restore on cleanup.
	saved := registry
	t.Cleanup(func() { registry = saved })

	execCount := map[int64]int{}
	mkFn := func(v int64) func(*sql.DB) error {
		return func(*sql.DB) error { execCount[v]++; return nil }
	}

	registry = []migration{{version: 9000, fn: mkFn(9000)}}

	db := openRawDB(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if execCount[9000] != 1 {
		t.Fatalf("migration 9000: want 1 execution, got %d", execCount[9000])
	}

	// Add one migration before (8999) and one after (9001) the already-applied 9000.
	registry = []migration{
		{version: 8999, fn: mkFn(8999)},
		{version: 9000, fn: mkFn(9000)}, // already applied — must not re-run
		{version: 9001, fn: mkFn(9001)},
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	// All three versions must be recorded.
	for _, v := range []int64{8999, 9000, 9001} {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, v).Scan(&n)
		if n != 1 {
			t.Errorf("version %d: want 1 row in schema_migrations, got %d", v, n)
		}
	}
	// 8999 and 9001 each ran once; 9000 must not have re-run.
	for v, want := range map[int64]int{8999: 1, 9000: 1, 9001: 1} {
		if got := execCount[v]; got != want {
			t.Errorf("migration %d: want %d execution(s), got %d", v, want, got)
		}
	}
}

// TestMigrate_Idempotent verifies running Migrate twice is a no-op the second time.
func TestMigrate_Idempotent(t *testing.T) {
	db := openRawDB(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate should be a no-op: %v", err)
	}
	if got, want := appliedVersion(t, db), maxRegisteredVersion(); got != want {
		t.Fatalf("max applied version after re-run = %d, want %d", got, want)
	}
}
