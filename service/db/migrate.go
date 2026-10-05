package db

import (
	"context"
	"database/sql"

	dbmigrate "vocabulary_trainer/db/migrate"
)

// Migrate runs all pending migrations on the given database.
// Exported so cmd/import and cmd/import-hsk can call it directly on a *sql.DB.
func Migrate(database *sql.DB) error {
	return dbmigrate.Migrate(database)
}

// RebuildGlossRank recomputes the gloss_rank cache from cedict_entries and
// word_frequency_lang. Call it after either table changes.
func RebuildGlossRank(database *sql.DB) error {
	return dbmigrate.RebuildGlossRank(database)
}

// RebuildGlossRank rebuilds the gloss_rank cache (see migrate.RebuildGlossRank).
func (s *Store) RebuildGlossRank(ctx context.Context) error {
	return RebuildGlossRank(s.db)
}
