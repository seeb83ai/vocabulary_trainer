package migrate

func init() {
	register(migration{
		version: 44,
		sql: `CREATE TABLE IF NOT EXISTS component_hmm_scenes (
			character TEXT NOT NULL,
			user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			scene_text TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (character, user_id)
		)`,
	})
}
