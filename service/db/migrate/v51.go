package migrate

func init() {
	register(migration{
		version: 51,
		sql:     `ALTER TABLE sm2_progress ADD COLUMN prev_state TEXT`,
	})
}
