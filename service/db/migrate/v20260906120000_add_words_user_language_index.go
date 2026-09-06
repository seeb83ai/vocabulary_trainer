package migrate

// v20260906120000 adds a plain index on words(user_id, language). The only
// existing index touching those columns is idx_words_text_lang (text,
// language, user_id), which leads with text and can't be used by the very
// common "WHERE w.language = ? AND w.user_id = ?" lookup with no text filter
// (GetNextCard, componentWordSets, DetectConfusion, etc.). Without this
// index those queries fall back to a full scan of the words table.
func init() {
	register(migration{
		version: 20260906120000,
		sql:     `CREATE INDEX IF NOT EXISTS idx_words_user_language ON words(user_id, language);`,
	})
}
