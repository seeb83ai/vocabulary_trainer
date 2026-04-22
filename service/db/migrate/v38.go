package migrate

func init() {
	register(migration{
		// v38: add hanzi_decomposition_translation for storing translated definitions
		// (e.g. German) keyed by (character, lang).
		version: 38,
		sql: `
CREATE TABLE IF NOT EXISTS hanzi_decomposition_translation (
    character  TEXT NOT NULL REFERENCES hanzi_decomposition(character) ON DELETE CASCADE,
    lang       TEXT NOT NULL,
    definition TEXT NOT NULL,
    PRIMARY KEY (character, lang)
);
CREATE INDEX IF NOT EXISTS idx_hanzi_trans_lang ON hanzi_decomposition_translation(lang);
`,
	})
}
