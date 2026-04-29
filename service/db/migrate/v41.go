package migrate

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
)

func init() {
	register(migration{
		version: 41,
		fn:      migrateV41,
	})
}

func migrateV41(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS user_settings (
		user_id              INTEGER PRIMARY KEY REFERENCES users(id),
		primary_lang         TEXT NOT NULL DEFAULT 'en',
		secondary_lang       TEXT NOT NULL DEFAULT 'de',
		prog_new             TEXT NOT NULL DEFAULT 'transl_to_zh',
		prog_tier_struggling TEXT NOT NULL DEFAULT 'transl_to_zh',
		prog_tier_learning   TEXT NOT NULL DEFAULT 'zh_pinyin_to_transl',
		prog_tier_practicing TEXT NOT NULL DEFAULT 'zh_to_transl',
		prog_tier_mastered   TEXT NOT NULL DEFAULT 'random',
		new_word_mode_0      TEXT NOT NULL DEFAULT 'transl_to_zh',
		new_word_mode_1      TEXT NOT NULL DEFAULT 'transl_to_zh',
		new_word_mode_2      TEXT NOT NULL DEFAULT 'zh_to_transl',
		api_key_salt         TEXT NOT NULL DEFAULT '',
		deepl_key_enc        TEXT NOT NULL DEFAULT '',
		llm_provider         TEXT NOT NULL DEFAULT '',
		llm_key_enc          TEXT NOT NULL DEFAULT '',
		llm_local_url        TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return err
	}

	rows, err := db.Query(`SELECT id FROM users`)
	if err != nil {
		return err
	}
	var userIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		userIDs = append(userIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, uid := range userIDs {
		saltBytes := make([]byte, 16)
		if _, err := rand.Read(saltBytes); err != nil {
			return err
		}
		salt := hex.EncodeToString(saltBytes)
		_, err := db.Exec(`INSERT OR IGNORE INTO user_settings (user_id, api_key_salt) VALUES (?, ?)`, uid, salt)
		if err != nil {
			return err
		}
	}
	return nil
}
