package migrate

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

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
	db, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(OFF)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	// Simulate an existing DB using the old high-watermark format at version 51.
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
