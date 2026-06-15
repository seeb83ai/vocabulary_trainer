package migrate

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMain sets the seed-user env vars so the v20 admin/template-user migration
// runs non-interactively (matching the db package's test setup).
func TestMain(m *testing.M) {
	os.Setenv("ADMIN_EMAIL", "admin@example.de")
	os.Setenv("ADMIN_PASSWORD", "I am the admin")
	os.Setenv("USER_EMAIL", "me@example.de")
	os.Setenv("USER_PASSWORD", "I learn zh")
	os.Setenv("BCRYPT_COST", "min") // speed up bcrypt in tests
	os.Exit(m.Run())
}

// openRawDB opens a fresh in-memory SQLite database configured like production
// (foreign keys on, single connection so the shared in-memory DB is stable).
func openRawDB(t *testing.T) *sql.DB {
	t.Helper()
	// Unique per-test in-memory DB name so tests don't share state via the shared cache.
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
func maxRegisteredVersion() int {
	max := 0
	for _, m := range registry {
		if m.version > max {
			max = m.version
		}
	}
	return max
}

// schemaVersion reads the current schema_version.
func schemaVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow(`SELECT version FROM schema_version`).Scan(&v); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	return v
}

// migrateUpTo applies the registered migrations up to and including target,
// mirroring Migrate but stopping early, to simulate a DB frozen at a past
// schema version (a "mid-history snapshot").
func migrateUpTo(t *testing.T, db *sql.DB, target int) {
	t.Helper()
	sorted := append([]migration(nil), registry...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].version < sorted[j].version })

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version (version) SELECT 0 WHERE NOT EXISTS (SELECT 1 FROM schema_version)`); err != nil {
		t.Fatalf("seed schema_version: %v", err)
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
		if _, err := db.Exec(`UPDATE schema_version SET version = ?`, m.version); err != nil {
			t.Fatalf("update schema_version to %d: %v", m.version, err)
		}
	}
}

// TestMigrate_FullChainFromEmpty runs the entire migration chain on a fresh DB
// and verifies it lands at the latest version. This exercises every migration,
// including the table-recreation migrations (v18, v21).
func TestMigrate_FullChainFromEmpty(t *testing.T) {
	db := openRawDB(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate from empty: %v", err)
	}
	if got, want := schemaVersion(t, db), maxRegisteredVersion(); got != want {
		t.Fatalf("schema_version after full chain = %d, want %d", got, want)
	}
	// Core tables created by the chain must exist and be queryable.
	for _, table := range []string{"words", "sm2_progress", "user_settings", "schema_version"} {
		if _, err := db.Exec("SELECT 1 FROM " + table + " LIMIT 1"); err != nil {
			t.Errorf("expected table %q to exist after migration: %v", table, err)
		}
	}
}

// TestMigrate_FromMidHistorySnapshot freezes a DB at a version just before the
// risky table-recreation migrations (v18, v21), seeds a row, then runs the rest
// of the chain — verifying the table-recreation migrations upgrade an existing,
// populated schema without error and preserve data.
func TestMigrate_FromMidHistorySnapshot(t *testing.T) {
	db := openRawDB(t)
	// Freeze at v17 (immediately before v18's table recreation).
	migrateUpTo(t, db, 17)
	if got := schemaVersion(t, db); got != 17 {
		t.Fatalf("snapshot schema_version = %d, want 17", got)
	}

	// Seed a word so the v18/v21 table recreations must carry data forward.
	if _, err := db.Exec(`INSERT INTO words (text, language) VALUES ('__migrate_test_marker__', 'zh')`); err != nil {
		t.Fatalf("seed word at snapshot: %v", err)
	}

	// Apply the remainder of the chain.
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate from mid-history snapshot: %v", err)
	}
	if got, want := schemaVersion(t, db), maxRegisteredVersion(); got != want {
		t.Fatalf("schema_version after upgrade = %d, want %d", got, want)
	}
	// The marker word must survive the v18/v21 table recreations. A later
	// multi-tenant migration may copy template words per seeded user, so assert
	// survival (>= 1) rather than an exact count.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM words WHERE text = '__migrate_test_marker__'`).Scan(&count); err != nil {
		t.Fatalf("count words after upgrade: %v", err)
	}
	if count < 1 {
		t.Errorf("seeded word did not survive the table-recreation migrations: count=%d", count)
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
	if got, want := schemaVersion(t, db), maxRegisteredVersion(); got != want {
		t.Fatalf("schema_version after re-run = %d, want %d", got, want)
	}
}
