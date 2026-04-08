package migrate

func init() {
	register(migration{
		version: 11,
		sql: `
CREATE TABLE IF NOT EXISTS pinyin_sounds (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	initial   TEXT NOT NULL DEFAULT '',
	final     TEXT NOT NULL,
	tone      INTEGER NOT NULL CHECK(tone BETWEEN 1 AND 5),
	syllable  TEXT NOT NULL,
	filename  TEXT NOT NULL UNIQUE,
	tag       TEXT NOT NULL DEFAULT '',
	UNIQUE(syllable, tone)
);

CREATE TABLE IF NOT EXISTS pinyin_progress (
	sound_id         INTEGER PRIMARY KEY REFERENCES pinyin_sounds(id) ON DELETE CASCADE,
	repetitions      INTEGER NOT NULL DEFAULT 0,
	easiness         REAL    NOT NULL DEFAULT 2.5,
	interval_days    INTEGER NOT NULL DEFAULT 1,
	due_date         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	total_correct    INTEGER NOT NULL DEFAULT 0,
	total_attempts   INTEGER NOT NULL DEFAULT 0,
	learning         INTEGER NOT NULL DEFAULT 1,
	streak_bonus     INTEGER NOT NULL DEFAULT 0,
	first_seen_date  TEXT DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS pinyin_confusions (
	sound_id         INTEGER NOT NULL REFERENCES pinyin_sounds(id) ON DELETE CASCADE,
	confused_with_id INTEGER NOT NULL REFERENCES pinyin_sounds(id) ON DELETE CASCADE,
	count            INTEGER NOT NULL DEFAULT 1,
	last_seen        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (sound_id, confused_with_id)
);

CREATE INDEX IF NOT EXISTS idx_pinyin_progress_due ON pinyin_progress(due_date);
CREATE INDEX IF NOT EXISTS idx_pinyin_sounds_tag ON pinyin_sounds(tag);
CREATE INDEX IF NOT EXISTS idx_pinyin_sounds_syllable ON pinyin_sounds(syllable);
`,
	})
}
