package migrate

func init() {
	register(migration{
		version: 45,
		sql:     `ALTER TABLE sm2_progress ADD COLUMN prev_state TEXT`,
	})
}
