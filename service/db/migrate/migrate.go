package migrate

import (
	"database/sql"
	"fmt"
	"sort"
)

// migration describes a single schema migration step.
type migration struct {
	version int
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

	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL DEFAULT 0)`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}
	if _, err := database.Exec(`INSERT INTO schema_version (version) SELECT 0 WHERE NOT EXISTS (SELECT 1 FROM schema_version)`); err != nil {
		return fmt.Errorf("seed schema_version: %w", err)
	}

	var current int
	if err := database.QueryRow(`SELECT version FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range registry {
		if m.version <= current {
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
		if _, err := database.Exec(`UPDATE schema_version SET version = ?`, m.version); err != nil {
			return fmt.Errorf("update schema version to %d: %w", m.version, err)
		}
	}
	return nil
}
