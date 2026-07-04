package migrate

import (
	"database/sql"
	"fmt"
	"sort"
)

// migration describes a single schema migration step.
type migration struct {
	version int64
	sql     string              // executed first (may be empty)
	fn      func(*sql.DB) error // executed after sql (may be nil)
}

var registry []migration

func register(m migration) {
	registry = append(registry, m)
}

// Migrate runs all pending migrations on the given database.
// Exported so cmd/import and cmd/import-hsk can call it directly on a *sql.DB.
func Migrate(database *sql.DB) error {
	sort.Slice(registry, func(i, j int) bool {
		return registry[i].version < registry[j].version
	})

	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Bootstrap: convert old single-row schema_version table to per-migration tracking.
	var oldExists int
	database.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'`).Scan(&oldExists)
	if oldExists > 0 {
		var hwm int64
		database.QueryRow(`SELECT version FROM schema_version`).Scan(&hwm)
		for _, m := range registry {
			if m.version <= hwm {
				database.Exec(`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, m.version)
			}
		}
		database.Exec(`DROP TABLE schema_version`)
	}

	// Load the set of already-applied versions. rows.Close() before any follow-up query.
	rows, err := database.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	applied := make(map[int64]bool)
	for rows.Next() {
		var v int64
		rows.Scan(&v)
		applied[v] = true
	}
	rows.Close()

	for _, m := range registry {
		if applied[m.version] {
			continue
		}
		if m.sql != "" {
			if _, err := database.Exec(m.sql); err != nil {
				return fmt.Errorf("migration %d sql: %w", m.version, err)
			}
		}
		if m.fn != nil {
			if err := m.fn(database); err != nil {
				return fmt.Errorf("migration %d fn: %w", m.version, err)
			}
		}
		if _, err := database.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.version); err != nil {
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
	}
	return nil
}
