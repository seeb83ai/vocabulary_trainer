package migrate

func init() {
	register(migration{
		version: 5,
		sql: `ALTER TABLE sm2_progress ADD COLUMN learning_new_word INTEGER NOT NULL DEFAULT 1;
UPDATE sm2_progress SET learning_new_word = 0 WHERE total_correct >= 3;`,
	})
}
