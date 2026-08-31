package migrate

import (
	"bufio"
	"database/sql"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

// frequencyData is an 8,000-entry Chinese word-frequency list (word<TAB>rank
// per line, rank 1 = most frequent), derived from hermitdave/FrequencyWords
// (MIT license, https://github.com/hermitdave/FrequencyWords),
// content/2018/zh_cn/zh_cn_50k.txt (an OpenSubtitles-2018-based corpus) —
// filtered to entries made up solely of CJK ideographs, deduplicated, and
// truncated to the top 8,000 by frequency. Kept in sync with the identical
// copy at service/cmd/import-frequency/frequency_data.txt, which remains
// available as a standalone tool for importing an alternative/updated list —
// see TestBundledFrequencyDataMatchesCmdCopy.
//
//go:embed frequency_data.txt
var frequencyData string

func init() {
	register(migration{
		version: 20260826120000,
		sql: `
CREATE TABLE IF NOT EXISTS word_frequency (
  word TEXT    PRIMARY KEY,
  rank INTEGER NOT NULL
);
`,
		fn: importBundledWordFrequency,
	})
}

// importBundledWordFrequency populates word_frequency from the embedded
// frequencyData on first run of this migration (see CLAUDE.md: migrations
// run once, tracked in schema_migrations, so this is a one-time cost — not
// re-parsed on every startup). This is a reference table only: it never
// creates or modifies vocabulary words, so unlike the optional import-hanzi/
// import-cedict tools, requiring a manual step here would leave issue #340's
// new-word-ordering feature silently doing nothing on any deploy where the
// operator forgot to run it.
func importBundledWordFrequency(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin word_frequency import: %w", err)
	}
	defer tx.Rollback()

	scanner := bufio.NewScanner(strings.NewReader(frequencyData))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		word := strings.TrimSpace(parts[0])
		rank, convErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if word == "" || convErr != nil {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO word_frequency (word, rank) VALUES (?, ?)
			 ON CONFLICT(word) DO UPDATE SET rank = excluded.rank`,
			word, rank,
		); err != nil {
			return fmt.Errorf("upsert word_frequency %q: %w", word, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan bundled frequency data: %w", err)
	}
	return tx.Commit()
}
