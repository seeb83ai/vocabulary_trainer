package db

import (
	"database/sql"

	dbmigrate "vocabulary_trainer/db/migrate"
)

// Migrate runs all pending migrations on the given database.
// Exported so cmd/import and cmd/import-hsk can call it directly on a *sql.DB.
func Migrate(database *sql.DB) error {
	return dbmigrate.Migrate(database)
}
