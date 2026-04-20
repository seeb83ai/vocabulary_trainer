package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		// v37: clear component_progress rows that were incorrectly populated by
		// the v36 migration (which inserted word characters instead of their
		// decomposition components) and re-backfill with the correct logic.
		version: 37,
		fn:      rebackfillComponents,
	})
}

func rebackfillComponents(db *sql.DB) error {
	if _, err := db.Exec(`DELETE FROM component_progress`); err != nil {
		return fmt.Errorf("v37 clear component_progress: %w", err)
	}
	return backfillComponents(db)
}
