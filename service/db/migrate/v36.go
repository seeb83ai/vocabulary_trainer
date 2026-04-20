package migrate

import (
	"database/sql"
	"fmt"
	"unicode"
)

func init() {
	register(migration{
		// v36: add component_progress and component_stats tables for hanzi component learning.
		version: 36,
		sql: `
CREATE TABLE IF NOT EXISTS component_progress (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  character       TEXT    NOT NULL,
  repetitions     INTEGER NOT NULL DEFAULT 0,
  easiness        REAL    NOT NULL DEFAULT 2.5,
  interval_days   INTEGER NOT NULL DEFAULT 1,
  due_date        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  total_correct   INTEGER NOT NULL DEFAULT 0,
  total_attempts  INTEGER NOT NULL DEFAULT 0,
  first_seen_date TEXT,
  UNIQUE(user_id, character)
);

CREATE INDEX IF NOT EXISTS idx_comp_progress_due ON component_progress(user_id, due_date);

CREATE TABLE IF NOT EXISTS component_stats (
  user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date     TEXT    NOT NULL,
  correct  INTEGER NOT NULL DEFAULT 0,
  wrong    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, date)
);
`,
		fn: backfillComponents,
	})
}

// idsOperatorRune returns true for IDS operator runes (U+2FF0–U+2FFB).
func idsOperatorRune(r rune) bool {
	return r >= 0x2FF0 && r <= 0x2FFB
}

func backfillComponents(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT w.user_id, w.text, p.due_date
		FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		WHERE w.language = 'zh'
		  AND p.first_seen_date IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("backfill components query: %w", err)
	}

	type entry struct {
		userID  int64
		text    string
		dueDate string
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.userID, &e.text, &e.dueDate); err != nil {
			rows.Close()
			return fmt.Errorf("backfill components scan: %w", err)
		}
		entries = append(entries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backfill components iterate: %w", err)
	}

	for _, e := range entries {
		for _, r := range []rune(e.text) {
			if !unicode.Is(unicode.Han, r) {
				continue
			}
			var def string
			err := db.QueryRow(
				`SELECT COALESCE(definition, '') FROM hanzi_decomposition WHERE character = ?`,
				string(r),
			).Scan(&def)
			if err == sql.ErrNoRows || def == "" {
				continue
			}
			if err != nil {
				return fmt.Errorf("backfill components lookup %q: %w", string(r), err)
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
				e.userID, string(r), e.dueDate,
			); err != nil {
				return fmt.Errorf("backfill components insert %q: %w", string(r), err)
			}
		}
	}
	return nil
}
