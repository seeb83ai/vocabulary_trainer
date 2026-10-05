package migrate

// v20261005090000 adds zh_char_translations: a shared cache of translations
// for single characters (e.g. radicals) that came from DeepL, read by the
// mnemonic builder. It replaces storing them as zh words, which wrote into
// learners' vocabularies.
func init() {
	register(migration{
		version: 20261005090000,
		sql: `CREATE TABLE IF NOT EXISTS zh_char_translations (
  text        TEXT NOT NULL,
  lang        TEXT NOT NULL,
  translation TEXT NOT NULL,
  PRIMARY KEY (text, lang)
)`,
	})
}
