package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
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
	})
}
