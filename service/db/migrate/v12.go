package migrate

func init() {
	register(migration{
		// v12: widen pinyin tone CHECK from 1-4 to 1-5 (neutral tone).
		// SQLite doesn't support ALTER CONSTRAINT, so rebuild the table.
		version: 12,
		sql: `
PRAGMA foreign_keys = OFF;
CREATE TABLE IF NOT EXISTS pinyin_sounds_new (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	initial   TEXT NOT NULL DEFAULT '',
	final     TEXT NOT NULL,
	tone      INTEGER NOT NULL CHECK(tone BETWEEN 1 AND 5),
	syllable  TEXT NOT NULL,
	filename  TEXT NOT NULL UNIQUE,
	tag       TEXT NOT NULL DEFAULT '',
	UNIQUE(syllable, tone)
);
INSERT OR IGNORE INTO pinyin_sounds_new (id, initial, final, tone, syllable, filename, tag)
	SELECT id, initial, final, tone, syllable, filename, tag FROM pinyin_sounds;
DROP TABLE IF EXISTS pinyin_sounds;
ALTER TABLE pinyin_sounds_new RENAME TO pinyin_sounds;
CREATE INDEX IF NOT EXISTS idx_pinyin_sounds_tag ON pinyin_sounds(tag);
CREATE INDEX IF NOT EXISTS idx_pinyin_sounds_syllable ON pinyin_sounds(syllable);
PRAGMA foreign_keys = ON;
`,
	})
}
