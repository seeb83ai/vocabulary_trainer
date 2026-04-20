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
		if err := insertComponentsForEntry(db, e.userID, e.text, e.dueDate); err != nil {
			return err
		}
	}
	return nil
}

// insertComponentsForEntry inserts component_progress rows for all components
// extracted from the decompositions of each Han rune in zhText.
func insertComponentsForEntry(db *sql.DB, userID int64, zhText, dueDate string) error {
	for _, r := range []rune(zhText) {
		if !unicode.Is(unicode.Han, r) {
			continue
		}
		var decomp sql.NullString
		err := db.QueryRow(
			`SELECT decomposition FROM hanzi_decomposition WHERE character = ?`,
			string(r),
		).Scan(&decomp)
		if err == sql.ErrNoRows || !decomp.Valid || decomp.String == "" {
			continue
		}
		if err != nil {
			return fmt.Errorf("backfill decomp lookup %q: %w", string(r), err)
		}
		for _, comp := range extractComponentsV36(decomp.String) {
			var def string
			err := db.QueryRow(
				`SELECT COALESCE(definition, '') FROM hanzi_decomposition WHERE character = ?`,
				string(comp),
			).Scan(&def)
			if err == sql.ErrNoRows || def == "" {
				continue
			}
			if err != nil {
				return fmt.Errorf("backfill component def lookup %q: %w", string(comp), err)
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
				userID, string(comp), dueDate,
			); err != nil {
				return fmt.Errorf("backfill component insert %q: %w", string(comp), err)
			}
		}
	}
	return nil
}

func extractComponentsV36(decomposition string) []rune {
	var out []rune
	for _, r := range decomposition {
		if !idsOperatorRune(r) && r != '？' && r != '?' {
			out = append(out, r)
		}
	}
	return out
}
