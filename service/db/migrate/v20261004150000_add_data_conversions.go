package migrate

// v20261004150000 adds data_conversions: one row per one-time data
// conversion the server ran at startup (outside the schema migrations,
// because it needs Go logic and filled tables), e.g. the conversion of
// copied list imports to library references (ADR-0005). It also indexes
// word_game_shown(word_id): deleting a word checks that table, and the
// conversion deletes thousands of copied gloss words.
func init() {
	register(migration{
		version: 20261004150000,
		sql: `CREATE TABLE IF NOT EXISTS data_conversions (
  name    TEXT PRIMARY KEY,
  done_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_word_game_shown_word ON word_game_shown(word_id);`,
	})
}
