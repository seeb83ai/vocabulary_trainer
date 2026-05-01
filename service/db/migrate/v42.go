package migrate

func init() {
	register(migration{
		version: 42,
		sql: `INSERT OR IGNORE INTO hanzi_decomposition_translation (character, lang, definition)
SELECT character, 'EN', definition
FROM hanzi_decomposition
WHERE definition IS NOT NULL AND definition != ''`,
	})
}
