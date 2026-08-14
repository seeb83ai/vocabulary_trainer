package migrate

// v20260814120000 adds cedict_entries, a bilingual (EN/DE) CEDICT-format
// dictionary table used to segment multi-character zh words into their
// constituent sub-words (issue #293). Populated offline by
// cmd/import-cedict from CC-CEDICT (lang='en') and HanDeDict (lang='de').
func init() {
	register(migration{
		version: 20260814120000,
		sql: `
CREATE TABLE IF NOT EXISTS cedict_entries (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  simplified TEXT NOT NULL,
  lang       TEXT NOT NULL,
  pinyin     TEXT,
  definition TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cedict_simplified ON cedict_entries(simplified);
CREATE INDEX IF NOT EXISTS idx_cedict_simplified_lang ON cedict_entries(simplified, lang);
`,
	})
}
