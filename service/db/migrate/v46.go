package migrate

func init() {
	register(migration{
		version: 46,
		sql:     `ALTER TABLE user_settings ADD COLUMN accept_correct_mode TEXT NOT NULL DEFAULT 'typo'`,
	})
}
