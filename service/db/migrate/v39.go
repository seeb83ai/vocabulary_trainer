package migrate

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

func init() {
	register(migration{
		// v39: add hanzi_decomposition.pinyin column for per-character pinyin
		// (JSON-encoded string array), wipe component_progress, and re-backfill
		// with an etymology-aware filter that skips phonetic-only components.
		// Due dates are re-spread so no user sees more than 5 new components
		// per day.
		version: 39,
		fn:      rebackfillComponentsV39,
	})
}

func rebackfillComponentsV39(db *sql.DB) error {
	if err := addPinyinColumnV39(db); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM component_progress`); err != nil {
		return fmt.Errorf("v39 clear component_progress: %w", err)
	}
	if err := backfillComponentsV39(db); err != nil {
		return err
	}
	return spreadComponentDueDates(db)
}

// addPinyinColumnV39 adds the pinyin column to hanzi_decomposition if missing.
// Existence is checked via PRAGMA table_info so the migration is idempotent
// on re-runs (ALTER TABLE ADD COLUMN is not IF NOT EXISTS-aware on SQLite).
func addPinyinColumnV39(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(hanzi_decomposition)`)
	if err != nil {
		return fmt.Errorf("v39 pragma table_info: %w", err)
	}
	hasPinyin := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("v39 scan table_info: %w", err)
		}
		if name == "pinyin" {
			hasPinyin = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("v39 iterate table_info: %w", err)
	}
	if hasPinyin {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE hanzi_decomposition ADD COLUMN pinyin TEXT`); err != nil {
		return fmt.Errorf("v39 add pinyin column: %w", err)
	}
	return nil
}

// backfillComponentsV39 iterates every (user, zh word) with first_seen_date set
// and inserts filtered component_progress rows. Mirrors v36's backfill but
// calls the v39 etymology-aware filter instead of the old "has definition"
// check.
func backfillComponentsV39(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT w.user_id, w.text, p.due_date
		FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		WHERE w.language = 'zh'
		  AND p.first_seen_date IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("v39 backfill query: %w", err)
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
			return fmt.Errorf("v39 backfill scan: %w", err)
		}
		entries = append(entries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("v39 backfill iterate: %w", err)
	}

	for _, e := range entries {
		if err := insertComponentsForEntryV39(db, e.userID, e.text, e.dueDate); err != nil {
			return err
		}
	}
	return nil
}

func insertComponentsForEntryV39(db *sql.DB, userID int64, zhText, dueDate string) error {
	for _, r := range []rune(zhText) {
		if !unicode.Is(unicode.Han, r) {
			continue
		}
		var decomp, etymology, radical, parentPinyin sql.NullString
		err := db.QueryRow(
			`SELECT decomposition, etymology, radical, pinyin FROM hanzi_decomposition WHERE character = ?`,
			string(r),
		).Scan(&decomp, &etymology, &radical, &parentPinyin)
		if err == sql.ErrNoRows || !decomp.Valid || decomp.String == "" {
			continue
		}
		if err != nil {
			return fmt.Errorf("v39 decomp lookup %q: %w", string(r), err)
		}
		parentPy := parsePinyinV39(parentPinyin.String)
		for _, comp := range extractComponentsV36(decomp.String) {
			var def, compPinyin sql.NullString
			err := db.QueryRow(
				`SELECT definition, pinyin FROM hanzi_decomposition WHERE character = ?`,
				string(comp),
			).Scan(&def, &compPinyin)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return fmt.Errorf("v39 component lookup %q: %w", string(comp), err)
			}
			if !def.Valid || def.String == "" {
				continue
			}
			if !shouldKeepComponentV39(r, comp, etymology.String, radical.String, parentPy, parsePinyinV39(compPinyin.String)) {
				continue
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
				userID, string(comp), dueDate,
			); err != nil {
				return fmt.Errorf("v39 component insert %q: %w", string(comp), err)
			}
		}
	}
	return nil
}

// shouldKeepComponentV39 is a frozen copy of the runtime shouldKeepComponent.
// Duplicated here intentionally so future tweaks to the runtime helper do not
// alter this historical migration's behaviour.
func shouldKeepComponentV39(parent, comp rune, etymologyJSON, radical string, parentPinyin, compPinyin []string) bool {
	if comp == parent {
		return false
	}
	if etymologyJSON != "" {
		var ety struct {
			Type     string `json:"type"`
			Phonetic string `json:"phonetic"`
			Semantic string `json:"semantic"`
		}
		if err := json.Unmarshal([]byte(etymologyJSON), &ety); err == nil && ety.Type != "" {
			if ety.Type == "pictophonetic" &&
				ety.Phonetic != "" &&
				string(comp) == ety.Phonetic &&
				string(comp) != ety.Semantic &&
				string(comp) != radical {
				return false
			}
			return true
		}
	}
	if pinyinSimilarV39(parentPinyin, compPinyin) {
		return false
	}
	return true
}

func pinyinSimilarV39(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	finalsA := make(map[string]struct{}, len(a))
	for _, r := range a {
		if f := pinyinFinalV39(r); f != "" {
			finalsA[f] = struct{}{}
		}
	}
	if len(finalsA) == 0 {
		return false
	}
	for _, r := range b {
		if f := pinyinFinalV39(r); f != "" {
			if _, ok := finalsA[f]; ok {
				return true
			}
		}
	}
	return false
}

var pinyinInitialsV39 = []string{
	"zh", "ch", "sh",
	"b", "p", "m", "f", "d", "t", "n", "l",
	"g", "k", "h", "j", "q", "x",
	"r", "z", "c", "s", "y", "w",
}

func pinyinFinalV39(syllable string) string {
	s := stripPinyinTonesV39(syllable)
	if s == "" {
		return ""
	}
	for _, init := range pinyinInitialsV39 {
		if strings.HasPrefix(s, init) {
			return s[len(init):]
		}
	}
	return s
}

func stripPinyinTonesV39(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	for len(s) > 0 {
		last := s[len(s)-1]
		if last >= '0' && last <= '5' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'ā', 'á', 'ǎ', 'à':
			b.WriteRune('a')
		case 'ē', 'é', 'ě', 'è':
			b.WriteRune('e')
		case 'ī', 'í', 'ǐ', 'ì':
			b.WriteRune('i')
		case 'ō', 'ó', 'ǒ', 'ò':
			b.WriteRune('o')
		case 'ū', 'ú', 'ǔ', 'ù':
			b.WriteRune('u')
		case 'ǖ', 'ǘ', 'ǚ', 'ǜ', 'ü':
			b.WriteRune('v')
		case 'ń', 'ň', 'ǹ':
			b.WriteRune('n')
		case 'ḿ':
			b.WriteRune('m')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func parsePinyinV39(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}
