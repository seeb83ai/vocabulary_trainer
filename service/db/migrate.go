package db

import (
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// migration describes a single schema migration step.
type migration struct {
	version int
	sql     string              // executed first (may be empty)
	fn      func(*sql.DB) error // executed after sql (may be nil)
}

// migrations is the ordered list of all schema migrations.
// Version 1 corresponds to the full initial schema (works on both fresh and
// existing databases thanks to IF NOT EXISTS / duplicate-column guards).
// Append new migrations at the end with incrementing version numbers.
var migrations = []migration{
	{
		version: 1,
		sql: `
CREATE TABLE IF NOT EXISTS words (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  text       TEXT    NOT NULL,
  language   TEXT    NOT NULL CHECK(language IN ('en', 'zh')),
  pinyin     TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(text, language)
);

CREATE TABLE IF NOT EXISTS translations (
  en_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  zh_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  PRIMARY KEY (en_word_id, zh_word_id)
);

CREATE TABLE IF NOT EXISTS sm2_progress (
  word_id          INTEGER PRIMARY KEY REFERENCES words(id) ON DELETE CASCADE,
  repetitions      INTEGER NOT NULL DEFAULT 0,
  easiness         REAL    NOT NULL DEFAULT 2.5,
  interval_days    INTEGER NOT NULL DEFAULT 1,
  due_date         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  total_correct    INTEGER NOT NULL DEFAULT 0,
  total_attempts   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS confusion_pairs (
  zh_word_id       INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  confused_with_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  mode             TEXT    NOT NULL,
  count            INTEGER NOT NULL DEFAULT 1,
  last_seen        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (zh_word_id, confused_with_id, mode)
);

CREATE TABLE IF NOT EXISTS tags (
  id   INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS word_tags (
  word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (word_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_sm2_due ON sm2_progress(due_date);
CREATE INDEX IF NOT EXISTS idx_words_text_lang ON words(text, language);
CREATE INDEX IF NOT EXISTS idx_translations_zh ON translations(zh_word_id);
CREATE INDEX IF NOT EXISTS idx_word_tags_word ON word_tags(word_id);
`,
		fn: func(db *sql.DB) error {
			// Add first_seen_date to sm2_progress for existing databases
			// that pre-date this column. Fresh databases get it from the
			// CREATE TABLE above only starting from migration v2+; for v1
			// we always attempt the ALTER so existing production DBs are
			// covered.
			if _, err := db.Exec(`ALTER TABLE sm2_progress ADD COLUMN first_seen_date TEXT DEFAULT NULL`); err != nil {
				if !strings.Contains(err.Error(), "duplicate column name") {
					return fmt.Errorf("add first_seen_date column: %w", err)
				}
			} else {
				// Column was just added — backfill: mark already-tested
				// words as seen yesterday so they are not treated as "new"
				// by the daily new-word cap.
				if _, err := db.Exec(`UPDATE sm2_progress SET first_seen_date = DATE('now', '-1 day') WHERE total_attempts > 0`); err != nil {
					return fmt.Errorf("backfill first_seen_date: %w", err)
				}
			}
			if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sm2_first_seen ON sm2_progress(first_seen_date)`); err != nil {
				return fmt.Errorf("create first_seen index: %w", err)
			}
			return nil
		},
	},
	{
		version: 2,
		sql: `
CREATE TABLE IF NOT EXISTS daily_stats (
  date            TEXT PRIMARY KEY,
  attempts        INTEGER NOT NULL DEFAULT 0,
  mistakes        INTEGER NOT NULL DEFAULT 0,
  words_known     INTEGER NOT NULL DEFAULT 0,
  new_words       INTEGER NOT NULL DEFAULT 0,
  correct_streak  INTEGER NOT NULL DEFAULT 0,
  current_streak  INTEGER NOT NULL DEFAULT 0
);
`,
	},
	{
		version: 3,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('words') WHERE name = 'needs_review'`).Scan(&count); err != nil {
				return fmt.Errorf("check needs_review column: %w", err)
			}
			if count > 0 {
				return nil // column already exists
			}
			if _, err := db.Exec(`ALTER TABLE words ADD COLUMN needs_review INTEGER DEFAULT 0`); err != nil {
				return fmt.Errorf("add needs_review column: %w", err)
			}
			return nil
		},
	},
	{
		version: 4,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('daily_stats') WHERE name = 'words_seen'`).Scan(&count); err != nil {
				return fmt.Errorf("check words_seen column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE daily_stats ADD COLUMN words_seen INTEGER NOT NULL DEFAULT 0`); err != nil {
					return fmt.Errorf("add words_seen column: %w", err)
				}
			}
			return nil
		},
	},
	{
		version: 5,
		sql: `ALTER TABLE sm2_progress ADD COLUMN learning_new_word INTEGER NOT NULL DEFAULT 1;
UPDATE sm2_progress SET learning_new_word = 0 WHERE total_correct >= 3;`,
	},
	{
		version: 6,
		fn: func(db *sql.DB) error {
			cols := []struct{ name, def string }{
				{"bucket_new", "INTEGER NOT NULL DEFAULT 0"},
				{"bucket_struggling", "INTEGER NOT NULL DEFAULT 0"},
				{"bucket_learning", "INTEGER NOT NULL DEFAULT 0"},
				{"bucket_practicing", "INTEGER NOT NULL DEFAULT 0"},
				{"bucket_mastered", "INTEGER NOT NULL DEFAULT 0"},
			}
			for _, c := range cols {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('daily_stats') WHERE name = ?`, c.name).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", c.name, err)
				}
				if count == 0 {
					if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE daily_stats ADD COLUMN %s %s`, c.name, c.def)); err != nil {
						return fmt.Errorf("add %s column: %w", c.name, err)
					}
				}
			}
			return nil
		},
	},
	{
		version: 7,
		fn: func(db *sql.DB) error {
			for _, col := range []string{"words_known", "new_words"} {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('daily_stats') WHERE name = ?`, col).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", col, err)
				}
				if count > 0 {
					if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE daily_stats DROP COLUMN %s`, col)); err != nil {
						return fmt.Errorf("drop %s column: %w", col, err)
					}
				}
			}
			return nil
		},
	},
	{
		version: 8,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sm2_progress') WHERE name = 'streak_bonus'`).Scan(&count); err != nil {
				return fmt.Errorf("check streak_bonus column: %w", err)
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE sm2_progress ADD COLUMN streak_bonus INTEGER NOT NULL DEFAULT 0`); err != nil {
					return fmt.Errorf("add streak_bonus column: %w", err)
				}
			}
			return nil
		},
	},
	{
		// Clean up words that were stamped by the now-removed GetNextCard stamp()
		// but never acknowledged by the user. These words have first_seen_date set
		// despite total_attempts = 0, which made them count in CountLearningNewWords
		// and block new word introductions. Resetting first_seen_date to NULL returns
		// them to the unseen pool so they can be properly introduced.
		version: 9,
		sql: `UPDATE sm2_progress
		      SET first_seen_date = NULL
		      WHERE total_attempts = 0
		        AND first_seen_date IS NOT NULL;`,
	},
	{
		version: 10,
		sql: `CREATE TABLE IF NOT EXISTS hanzi_decomposition (
			character     TEXT PRIMARY KEY,
			definition    TEXT,
			radical       TEXT,
			decomposition TEXT,
			etymology     TEXT
		);`,
	},
	{
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
	},
	{
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
	},
	{
		version: 13,
		sql: `
CREATE TABLE IF NOT EXISTS hmm_actors (
  initial    TEXT PRIMARY KEY,
  category   TEXT NOT NULL,
  actor_name TEXT NOT NULL DEFAULT '',
  hint       TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS hmm_locations (
  final_key     TEXT PRIMARY KEY,
  location_name TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS hmm_tone_rooms (
  tone      INTEGER PRIMARY KEY,
  room_name TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS hmm_props (
  radical   TEXT PRIMARY KEY,
  prop_name TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS hmm_scenes (
  word_id    INTEGER PRIMARY KEY REFERENCES words(id) ON DELETE CASCADE,
  scene_text TEXT NOT NULL DEFAULT ''
);
`,
		fn: func(db *sql.DB) error {
			// Seed actors by category with naming hints
			type actorSeed struct{ initial, category, hint string }
			actors := []actorSeed{
				// Male (18)
				{"null", "male", "Null initial (vowel-only syllables)"},
				{"b", "male", "Name starts with 'B'"}, {"p", "male", "Name starts with 'P'"},
				{"m", "male", "Name starts with 'M'"}, {"f", "male", "Name starts with 'F'"},
				{"d", "male", "Name starts with 'D'"}, {"t", "male", "Name starts with 'T'"},
				{"n", "male", "Name starts with 'N'"}, {"l", "male", "Name starts with 'L'"},
				{"g", "male", "Name starts with 'G'"}, {"k", "male", "Name starts with 'K'"},
				{"h", "male", "Name starts with 'H'"}, {"zh", "male", "Name sounds like 'Zh'"},
				{"ch", "male", "Name sounds like 'Ch'"}, {"sh", "male", "Name starts with 'Sh'"},
				{"r", "male", "Name starts with 'R'"}, {"z", "male", "Name starts with 'Z'"},
				{"c", "male", "Name starts with 'C'"}, {"s", "male", "Name starts with 'S'"},
				// Female (7)
				{"yi", "female", "Name starts with 'Yi'"},
				{"bi", "female", "Name starts with 'B'"}, {"pi", "female", "Name starts with 'P'"},
				{"mi", "female", "Name starts with 'M'"}, {"di", "female", "Name starts with 'D'"},
				{"ti", "female", "Name starts with 'T'"}, {"ni", "female", "Name starts with 'N'"},
				{"li", "female", "Name starts with 'L'"}, {"ji", "female", "Name starts with 'J'"},
				{"qi", "female", "Name starts with 'Q'"}, {"xi", "female", "Name starts with 'X'"},
				// Fictional (3)
				{"wu", "fictional", "Fictional, name starts with 'Wu'"},
				{"bu", "fictional", "Fictional, name starts with 'Bu'"},
				{"pu", "fictional", "Fictional, name starts with 'Pu'"},
				{"mu", "fictional", "Fictional, name starts with 'Mu'"},
				{"fu", "fictional", "Fictional, name starts with 'Fu'"},
				{"du", "fictional", "Fictional, name starts with 'Du'"},
				{"tu", "fictional", "Fictional, name starts with 'Tu'"},
				{"nu", "fictional", "Fictional, name starts with 'Nu'"},
				{"lu", "fictional", "Fictional, name starts with 'Lu'"},
				{"gu", "fictional", "Fictional, name starts with 'Gu'"},
				{"ku", "fictional", "Fictional, name starts with 'Ku'"},
				{"hu", "fictional", "Fictional, name starts with 'Hu'"},
				{"zhu", "fictional", "Fictional, name starts with 'Zhu'"},
				{"chu", "fictional", "Fictional, name starts with 'Chu'"},
				{"shu", "fictional", "Fictional, name starts with 'Shu'"},
				{"ru", "fictional", "Fictional, name starts with 'Ru'"},
				{"zu", "fictional", "Fictional, name starts with 'Zu'"},
				{"cu", "fictional", "Fictional, name starts with 'Cu'"},
				{"su", "fictional", "Fictional, name starts with 'Su'"},
				// Wildcard (3)
				{"yu", "wildcard", "God or world leader, name starts with 'Yu'"},
				{"nü", "wildcard", "God or world leader, name starts with 'Nü'"},
				{"lü", "wildcard", "God or world leader, name starts with 'Lü'"},
				{"ju", "wildcard", "God or world leader, name starts with 'Ju'"},
				{"qu", "wildcard", "God or world leader, name starts with 'Qu'"},
				{"xu", "wildcard", "God or world leader, name starts with 'Xu'"},
			}
			for _, a := range actors {
				name := ""
				if a.initial == "null" {
					name = "Jackie Chan"
				}
				if _, err := db.Exec(
					`INSERT OR IGNORE INTO hmm_actors (initial, category, actor_name, hint) VALUES (?, ?, ?, ?)`,
					a.initial, a.category, name, a.hint); err != nil {
					return fmt.Errorf("seed actor %s: %w", a.initial, err)
				}
			}

			// Seed tone rooms with method defaults
			toneRooms := []struct {
				tone int
				name string
			}{
				{1, "Outside the entrance"},
				{2, "Hallway / kitchen"},
				{3, "Bedroom / living room"},
				{4, "Bathroom / backyard"},
				{5, "On the roof"},
			}
			for _, tr := range toneRooms {
				if _, err := db.Exec(
					`INSERT OR IGNORE INTO hmm_tone_rooms (tone, room_name) VALUES (?, ?)`,
					tr.tone, tr.name); err != nil {
					return fmt.Errorf("seed tone room %d: %w", tr.tone, err)
				}
			}

			// Seed 13 location finals (empty names)
			finals := []string{"a", "o", "e", "ai", "ei", "ao", "ou", "an", "ang", "en", "eng", "ong", "null"}
			for _, f := range finals {
				if _, err := db.Exec(
					`INSERT OR IGNORE INTO hmm_locations (final_key, location_name) VALUES (?, '')`, f); err != nil {
					return fmt.Errorf("seed location %s: %w", f, err)
				}
			}

			// Seed default radical→prop mappings
			type propSeed struct{ radical, prop string }
			props := []propSeed{
				{"一", "razor blade"},
				{"二", "twins"},
			}
			for _, p := range props {
				if _, err := db.Exec(
					`INSERT OR IGNORE INTO hmm_props (radical, prop_name) VALUES (?, ?)`,
					p.radical, p.prop); err != nil {
					return fmt.Errorf("seed prop %s: %w", p.radical, err)
				}
			}

			return nil
		},
	},
	{
		// v14: shuffle due_date of all unseen pinyin_progress rows so training
		// order is random instead of alphabetical (insertion) order.
		version: 14,
		fn: func(db *sql.DB) error {
			rows, err := db.Query(`SELECT sound_id FROM pinyin_progress WHERE first_seen_date IS NULL`)
			if err != nil {
				return fmt.Errorf("query unseen pinyin progress: %w", err)
			}
			var ids []int64
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return fmt.Errorf("scan sound_id: %w", err)
				}
				ids = append(ids, id)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterate unseen pinyin progress: %w", err)
			}

			rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })

			now := time.Now().UTC()
			for i, id := range ids {
				// Assign unique past timestamps in shuffled order (1 second apart).
				dueDate := now.Add(-time.Duration(len(ids)-i) * time.Second).Format("2006-01-02 15:04:05")
				if _, err := db.Exec(`UPDATE pinyin_progress SET due_date = ? WHERE sound_id = ?`, dueDate, id); err != nil {
					return fmt.Errorf("update due_date for sound_id %d: %w", id, err)
				}
			}
			return nil
		},
	},
	{
		// v15: add hmm_progress table for SM-2 spaced repetition on mnemonic
		// library entries (actors, locations, tone rooms, props).
		// Also seeds progress rows for any already-named library entries,
		// with shuffled due_dates so review order is random.
		version: 15,
		sql: `
CREATE TABLE IF NOT EXISTS hmm_progress (
  entity_type     TEXT    NOT NULL,
  entity_key      TEXT    NOT NULL,
  repetitions     INTEGER NOT NULL DEFAULT 0,
  easiness        REAL    NOT NULL DEFAULT 2.5,
  interval_days   INTEGER NOT NULL DEFAULT 1,
  due_date        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  total_correct   INTEGER NOT NULL DEFAULT 0,
  total_attempts  INTEGER NOT NULL DEFAULT 0,
  learning        INTEGER NOT NULL DEFAULT 1,
  streak_bonus    INTEGER NOT NULL DEFAULT 0,
  first_seen_date TEXT DEFAULT NULL,
  PRIMARY KEY (entity_type, entity_key)
);
CREATE INDEX IF NOT EXISTS idx_hmm_progress_due ON hmm_progress(due_date);
`,
		fn: func(db *sql.DB) error {
			type seed struct {
				typ   string
				query string
			}
			seeds := []seed{
				{"actor", `SELECT initial FROM hmm_actors WHERE actor_name != ''`},
				{"location", `SELECT final_key FROM hmm_locations WHERE location_name != ''`},
				{"tone_room", `SELECT CAST(tone AS TEXT) FROM hmm_tone_rooms WHERE room_name != ''`},
				{"prop", `SELECT radical FROM hmm_props WHERE prop_name != ''`},
			}
			now := time.Now().UTC()
			for _, s := range seeds {
				rows, err := db.Query(s.query)
				if err != nil {
					return fmt.Errorf("seed hmm_progress %s: %w", s.typ, err)
				}
				var keys []string
				for rows.Next() {
					var k string
					if err := rows.Scan(&k); err != nil {
						rows.Close()
						return fmt.Errorf("scan hmm %s key: %w", s.typ, err)
					}
					keys = append(keys, k)
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					return fmt.Errorf("iterate hmm %s keys: %w", s.typ, err)
				}
				rand.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
				for i, key := range keys {
					dueDate := now.Add(-time.Duration(len(keys)-i) * time.Second).Format("2006-01-02 15:04:05")
					if _, err := db.Exec(
						`INSERT OR IGNORE INTO hmm_progress (entity_type, entity_key, due_date) VALUES (?, ?, ?)`,
						s.typ, key, dueDate); err != nil {
						return fmt.Errorf("insert hmm_progress %s/%s: %w", s.typ, key, err)
					}
				}
			}
			return nil
		},
	},
	{
		// v16: add pinyin_daily_stats table to track per-day pinyin training stats.
		version: 16,
		sql: `
CREATE TABLE IF NOT EXISTS pinyin_daily_stats (
  date        TEXT PRIMARY KEY,
  attempts    INTEGER NOT NULL DEFAULT 0,
  mistakes    INTEGER NOT NULL DEFAULT 0,
  sounds_seen INTEGER NOT NULL DEFAULT 0
);
`,
	},
	{
		// v17: add per-tone correct/wrong columns to pinyin_daily_stats.
		version: 17,
		fn: func(db *sql.DB) error {
			cols := []struct{ name, def string }{
				{"tone1_correct", "INTEGER NOT NULL DEFAULT 0"},
				{"tone1_wrong", "INTEGER NOT NULL DEFAULT 0"},
				{"tone2_correct", "INTEGER NOT NULL DEFAULT 0"},
				{"tone2_wrong", "INTEGER NOT NULL DEFAULT 0"},
				{"tone3_correct", "INTEGER NOT NULL DEFAULT 0"},
				{"tone3_wrong", "INTEGER NOT NULL DEFAULT 0"},
				{"tone4_correct", "INTEGER NOT NULL DEFAULT 0"},
				{"tone4_wrong", "INTEGER NOT NULL DEFAULT 0"},
				{"tone5_correct", "INTEGER NOT NULL DEFAULT 0"},
				{"tone5_wrong", "INTEGER NOT NULL DEFAULT 0"},
			}
			for _, c := range cols {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('pinyin_daily_stats') WHERE name = ?`, c.name).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", c.name, err)
				}
				if count == 0 {
					if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE pinyin_daily_stats ADD COLUMN %s %s`, c.name, c.def)); err != nil {
						return fmt.Errorf("add %s column: %w", c.name, err)
					}
				}
			}
			return nil
		},
	},
	{
		// v18: expand words.language CHECK to allow 'de' and add de_translations table.
		// SQLite does not support ALTER CONSTRAINT, so the words table is recreated
		// (same pattern as v12 for pinyin_sounds).
		version: 18,
		fn: func(db *sql.DB) error {
			// Check if 'de' is already in the CHECK — skip if so (idempotent).
			var wordsSql string
			if err := db.QueryRow(
				`SELECT sql FROM sqlite_master WHERE type='table' AND name='words'`,
			).Scan(&wordsSql); err != nil {
				return fmt.Errorf("read words schema: %w", err)
			}
			if !strings.Contains(wordsSql, "'de'") {
				// Disable FK enforcement so DROP TABLE words doesn't cascade-delete
				// translations, sm2_progress, word_tags, confusion_pairs, hmm_scenes.
				if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
					return fmt.Errorf("disable foreign keys: %w", err)
				}
				defer db.Exec(`PRAGMA foreign_keys = ON`)
				for _, stmt := range []string{
					`CREATE TABLE words_new (
					  id           INTEGER PRIMARY KEY AUTOINCREMENT,
					  text         TEXT    NOT NULL,
					  language     TEXT    NOT NULL CHECK(language IN ('en', 'zh', 'de')),
					  pinyin       TEXT,
					  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
					  needs_review INTEGER NOT NULL DEFAULT 0,
					  UNIQUE(text, language)
					)`,
					`INSERT OR IGNORE INTO words_new (id, text, language, pinyin, created_at, needs_review)
					 SELECT id, text, language, pinyin, created_at, COALESCE(needs_review, 0) FROM words`,
					`DROP TABLE words`,
					`ALTER TABLE words_new RENAME TO words`,
					`CREATE INDEX IF NOT EXISTS idx_words_text_lang ON words(text, language)`,
				} {
					if _, err := db.Exec(stmt); err != nil {
						return fmt.Errorf("expand words language check: %w", err)
					}
				}
			}
			return nil
		},
	},
	{
		// v19: create missing hmm_progress rows for any named library entries that
		// were added after migration v15 (which only ran once). Sets first_seen_date
		// to today so they are immediately eligible for training.
		version: 19,
		fn: func(db *sql.DB) error {
			seeds := []struct{ typ, query string }{
				{"actor", `SELECT initial FROM hmm_actors WHERE actor_name != ''`},
				{"location", `SELECT final_key FROM hmm_locations WHERE location_name != ''`},
				{"tone_room", `SELECT CAST(tone AS TEXT) FROM hmm_tone_rooms WHERE room_name != ''`},
				{"prop", `SELECT radical FROM hmm_props WHERE prop_name != ''`},
			}
			for _, s := range seeds {
				rows, err := db.Query(s.query)
				if err != nil {
					return fmt.Errorf("seed hmm_progress %s: %w", s.typ, err)
				}
				var keys []string
				for rows.Next() {
					var k string
					if err := rows.Scan(&k); err != nil {
						rows.Close()
						return fmt.Errorf("scan hmm %s key: %w", s.typ, err)
					}
					keys = append(keys, k)
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					return fmt.Errorf("iterate hmm %s keys: %w", s.typ, err)
				}
				for _, key := range keys {
					if _, err := db.Exec(
						`INSERT OR IGNORE INTO hmm_progress (entity_type, entity_key, first_seen_date) VALUES (?, ?, date('now'))`,
						s.typ, key); err != nil {
						return fmt.Errorf("insert hmm_progress %s/%s: %w", s.typ, key, err)
					}
				}
			}
			return nil
		},
	},
}

// Migrate runs all pending migrations on the given database.
// Exported so cmd/import and cmd/import-hsk can call it directly on a *sql.DB.
func Migrate(database *sql.DB) error {
	// Ensure the version-tracking table exists.
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL DEFAULT 0)`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	// Seed with version 0 if the table is empty (fresh DB or first run of
	// the migration system on an existing DB).
	if _, err := database.Exec(`INSERT INTO schema_version (version) SELECT 0 WHERE NOT EXISTS (SELECT 1 FROM schema_version)`); err != nil {
		return fmt.Errorf("seed schema_version: %w", err)
	}

	var current int
	if err := database.QueryRow(`SELECT version FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if m.sql != "" {
			if _, err := database.Exec(m.sql); err != nil {
				return fmt.Errorf("migration %d sql: %w", m.version, err)
			}
		}
		if m.fn != nil {
			if err := m.fn(database); err != nil {
				return fmt.Errorf("migration %d fn: %w", m.version, err)
			}
		}
		if _, err := database.Exec(`UPDATE schema_version SET version = ?`, m.version); err != nil {
			return fmt.Errorf("update schema version to %d: %w", m.version, err)
		}
	}
	return nil
}
