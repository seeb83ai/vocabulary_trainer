package migrate

// v20261012120000: attempts and errors per word and answer bucket (the help
// the user had on the card: zh_only, pinyin, voice, voice_only, to_zh).
// Daily rows are kept for 30 days, monthly rows are kept.
func init() {
	register(migration{
		version: 20261012120000,
		sql: `
CREATE TABLE IF NOT EXISTS answer_stats_daily (
  user_id  INTEGER NOT NULL,
  day      TEXT    NOT NULL,
  word_id  INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  bucket   TEXT    NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  errors   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, day, word_id, bucket)
);
CREATE TABLE IF NOT EXISTS answer_stats_monthly (
  user_id  INTEGER NOT NULL,
  month    TEXT    NOT NULL,
  word_id  INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
  bucket   TEXT    NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  errors   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, month, word_id, bucket)
);
CREATE INDEX IF NOT EXISTS idx_answer_stats_daily_word ON answer_stats_daily(word_id);
CREATE INDEX IF NOT EXISTS idx_answer_stats_monthly_word ON answer_stats_monthly(word_id);
`,
	})
}
