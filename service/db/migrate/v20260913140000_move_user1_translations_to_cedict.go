package migrate

import "database/sql"

// v20260913140000 makes cedict_entries the single source of translations for
// the tag-based import feature. The template user (user_id=1) previously
// stored its own EN/DE translation words, hand-curated at the original
// import; import/preview now looks up cedict_entries directly instead. Any
// translation user_id=1 had that isn't already covered by a dictionary entry
// is copied into cedict_entries (flagged source='user') so it isn't lost,
// then user_id=1's translation words, links and progress are removed —
// user_id=1 is a zh-words-and-tags-only template from here on.
func init() {
	register(migration{
		version: 20260913140000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cedict_entries') WHERE name = 'source'`).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				if _, err := db.Exec(`ALTER TABLE cedict_entries ADD COLUMN source TEXT NOT NULL DEFAULT 'dict'`); err != nil {
					return err
				}
			}

			if _, err := db.Exec(`
				INSERT INTO cedict_entries (simplified, lang, pinyin, definition, source)
				SELECT zh.text, tw.language, zh.pinyin, tw.text, 'user'
				FROM translations t
				JOIN words zh ON zh.id = t.zh_word_id AND zh.user_id = 1
				JOIN words tw ON tw.id = t.translation_word_id
				WHERE NOT EXISTS (
					SELECT 1 FROM cedict_entries c
					WHERE c.simplified = zh.text AND c.lang = tw.language AND c.definition = tw.text
				)`); err != nil {
				return err
			}

			if _, err := db.Exec(`
				DELETE FROM sm2_progress WHERE word_id IN (
					SELECT id FROM words WHERE user_id = 1 AND language != 'zh'
				)`); err != nil {
				return err
			}
			if _, err := db.Exec(`
				DELETE FROM translations WHERE translation_word_id IN (
					SELECT id FROM words WHERE user_id = 1 AND language != 'zh'
				)`); err != nil {
				return err
			}
			if _, err := db.Exec(`DELETE FROM words WHERE user_id = 1 AND language != 'zh'`); err != nil {
				return err
			}
			return nil
		},
	})
}
