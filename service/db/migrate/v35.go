package migrate

func init() {
	register(migration{
		// v35: rename translations.en_word_id → translation_word_id.
		// The column stored any target-language word (en or de), not only English.
		version: 35,
		sql: `ALTER TABLE translations RENAME COLUMN en_word_id TO translation_word_id`,
	})
}
