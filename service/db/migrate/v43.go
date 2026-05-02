package migrate

func init() {
	register(migration{
		version: 43,
		sql: `ALTER TABLE component_progress ADD COLUMN needs_review INTEGER NOT NULL DEFAULT 0`,
	})
}
