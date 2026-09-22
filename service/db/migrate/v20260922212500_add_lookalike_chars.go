package migrate

// v20260922212500 adds lookalike_chars: pairs of characters that look the same
// (e.g. 囗 wéi / 口 kǒu). Quiz cards show a "≠ 口" hint for them so the glyph can
// be told apart when pinyin is hidden (issue #466). Each pair is stored in both
// directions; add more rows directly in the DB.
func init() {
	register(migration{
		version: 20260922212500,
		sql: `
CREATE TABLE IF NOT EXISTS lookalike_chars (
	character TEXT NOT NULL,
	lookalike TEXT NOT NULL,
	PRIMARY KEY (character, lookalike)
);
INSERT OR IGNORE INTO lookalike_chars (character, lookalike) VALUES
	('囗', '口'), ('口', '囗'),
	('土', '士'), ('士', '土'),
	('未', '末'), ('末', '未'),
	('已', '巳'), ('巳', '已'),
	('日', '曰'), ('曰', '日');
`,
	})
}
