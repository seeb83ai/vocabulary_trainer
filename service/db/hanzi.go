package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"vocabulary_trainer/models"
)

// idsOperator returns true for IDS operator runes (U+2FF0–U+2FFB).
func idsOperator(r rune) bool {
	return r >= 0x2FF0 && r <= 0x2FFB
}

// extractComponents returns the non-operator, non-placeholder runes from an IDS string.
func extractComponents(decomposition string) []rune {
	var out []rune
	for _, r := range decomposition {
		if !idsOperator(r) && r != '？' && r != '?' {
			out = append(out, r)
		}
	}
	return out
}

func (s *Store) GetHanziDecomposition(ctx context.Context, chars []rune) ([]models.HanziDecomposition, error) {
	if len(chars) == 0 {
		return nil, nil
	}

	// Build placeholders for the IN clause.
	ph := make([]string, len(chars))
	args := make([]any, len(chars))
	for i, c := range chars {
		ph[i] = "?"
		args[i] = string(c)
	}

	query := `SELECT character, definition, radical, decomposition, etymology
		FROM hanzi_decomposition WHERE character IN (` + strings.Join(ph, ",") + `)`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query hanzi decomposition: %w", err)
	}

	type rawRow struct {
		character     string
		definition    sql.NullString
		radical       sql.NullString
		decomposition sql.NullString
		etymology     sql.NullString
	}
	var rawRows []rawRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.character, &r.definition, &r.radical, &r.decomposition, &r.etymology); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hanzi decomposition: %w", err)
		}
		rawRows = append(rawRows, r)
	}
	rows.Close()

	// Collect all component characters for a second-level lookup.
	compSet := map[rune]bool{}
	for _, r := range rawRows {
		if r.decomposition.Valid {
			for _, c := range extractComponents(r.decomposition.String) {
				compSet[c] = true
			}
		}
	}
	// Remove characters we already have from the top-level query.
	for _, c := range chars {
		delete(compSet, c)
	}

	// Second query for component definitions.
	compMap := map[string]*models.HanziDecomposition{}
	if len(compSet) > 0 {
		ph2 := make([]string, 0, len(compSet))
		args2 := make([]any, 0, len(compSet))
		for c := range compSet {
			ph2 = append(ph2, "?")
			args2 = append(args2, string(c))
		}
		rows2, err := s.db.QueryContext(ctx, `SELECT character, definition, radical, decomposition, etymology
			FROM hanzi_decomposition WHERE character IN (`+strings.Join(ph2, ",")+`)`, args2...)
		if err != nil {
			return nil, fmt.Errorf("query component decomposition: %w", err)
		}
		for rows2.Next() {
			var r rawRow
			if err := rows2.Scan(&r.character, &r.definition, &r.radical, &r.decomposition, &r.etymology); err != nil {
				rows2.Close()
				return nil, fmt.Errorf("scan component decomposition: %w", err)
			}
			d := buildDecomposition(r.character, r.definition, r.radical, r.decomposition, r.etymology)
			compMap[r.character] = &d
		}
		rows2.Close()
	}

	// Also index top-level results so components can reference siblings.
	for _, r := range rawRows {
		if _, ok := compMap[r.character]; !ok {
			d := buildDecomposition(r.character, r.definition, r.radical, r.decomposition, r.etymology)
			compMap[r.character] = &d
		}
	}

	// Build result with components attached.
	results := make([]models.HanziDecomposition, 0, len(rawRows))
	for _, r := range rawRows {
		d := buildDecomposition(r.character, r.definition, r.radical, r.decomposition, r.etymology)
		if r.decomposition.Valid {
			for _, c := range extractComponents(r.decomposition.String) {
				if comp, ok := compMap[string(c)]; ok && string(c) != r.character {
					appendComponent(&d, *comp)
				}
			}
		}
		results = append(results, d)
	}

	// Return in the same order as the input chars.
	ordered := make([]models.HanziDecomposition, 0, len(chars))
	idx := map[string]models.HanziDecomposition{}
	for _, d := range results {
		idx[d.Character] = d
	}
	for _, c := range chars {
		if d, ok := idx[string(c)]; ok {
			ordered = append(ordered, d)
		}
	}
	return ordered, nil
}

// GetHanziDecompositionString returns the raw decomposition string for a single character,
// or an empty string if none exists.
func (s *Store) GetHanziDecompositionString(ctx context.Context, char string) (string, error) {
	var decomp sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT decomposition FROM hanzi_decomposition WHERE character = ?`, char,
	).Scan(&decomp)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get hanzi decomposition string: %w", err)
	}
	if decomp.Valid {
		return decomp.String, nil
	}
	return "", nil
}

// UpsertHanziDecomposition inserts or updates the decomposition string for a character.
func (s *Store) UpsertHanziDecomposition(ctx context.Context, char, decomp string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, decomposition)
		 VALUES (?, ?)
		 ON CONFLICT(character) DO UPDATE SET decomposition = excluded.decomposition`,
		char, decomp)
	if err != nil {
		return fmt.Errorf("upsert hanzi decomposition: %w", err)
	}
	return nil
}

func appendComponent(parent *models.HanziDecomposition, comp models.HanziDecomposition) {
	comp.Components = nil
	comp.Decomposition = ""
	parent.Components = append(parent.Components, comp)
}

func buildDecomposition(character string, definition, radical, decomposition, etymology sql.NullString) models.HanziDecomposition {
	d := models.HanziDecomposition{Character: character}
	if definition.Valid {
		d.Definition = definition.String
	}
	if radical.Valid {
		d.Radical = radical.String
	}
	if decomposition.Valid {
		d.Decomposition = decomposition.String
	}
	if etymology.Valid && etymology.String != "" {
		var ety models.HanziEtymology
		if err := json.Unmarshal([]byte(etymology.String), &ety); err == nil && ety.Type != "" {
			d.Etymology = &ety
		}
	}
	return d
}

// getTranslationsByZhTexts returns the first translation in the given language for each
// zh_text in the supplied slice, keyed by zh_text. Both EN and DE translations share
// the translations table; language is distinguished via words.language.
func (s *Store) getTranslationsByZhTexts(ctx context.Context, zhTexts []string, lang string) (map[string]string, error) {
	if len(zhTexts) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(zhTexts))
	args := make([]any, len(zhTexts)+1)
	args[0] = lang
	for i, t := range zhTexts {
		placeholders[i] = "?"
		args[i+1] = t
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.text, MIN(e.text)
		 FROM words w
		 JOIN translations t ON t.zh_word_id = w.id
		 JOIN words e ON e.id = t.translation_word_id AND e.language = ?
		 WHERE w.language = 'zh' AND w.text IN (`+strings.Join(placeholders, ",")+`)
		 GROUP BY w.text`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("get %s translations by zh texts: %w", lang, err)
	}
	result := make(map[string]string)
	for rows.Next() {
		var zh, trans string
		if err := rows.Scan(&zh, &trans); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan %s translation: %w", lang, err)
		}
		result[zh] = trans
	}
	rows.Close()
	return result, rows.Err()
}

// GetTranslationsByZhTexts returns the first translation in the given language for each
// zh_text in the supplied slice, keyed by zh_text.
func (s *Store) GetTranslationsByZhTexts(ctx context.Context, zhTexts []string, lang string) (map[string]string, error) {
	return s.getTranslationsByZhTexts(ctx, zhTexts, lang)
}

// StoreTranslationForZhChar stores an EN or DE translation for a Chinese character.
// Both languages use the translations table; words.language distinguishes them.
// If the zh character does not yet exist in the words table it is created with the
// supplied pinyin (which may be empty). No SM-2 progress row is initialised because
// the character is stored only as a reference, not as a quiz word.
func (s *Store) StoreTranslationForZhChar(ctx context.Context, zhText, pinyin, transText, lang string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var zhID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM words WHERE text = ? AND language = 'zh'`, zhText,
	).Scan(&zhID); err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("find zh word: %w", err)
		}
		// zh word doesn't exist yet — create it so the translation can be linked.
		var py *string
		if pinyin != "" {
			py = &pinyin
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO words (text, language, pinyin) VALUES (?, 'zh', ?)`, zhText, py,
		); err != nil {
			return fmt.Errorf("insert zh word: %w", err)
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM words WHERE text = ? AND language = 'zh'`, zhText,
		).Scan(&zhID); err != nil {
			return fmt.Errorf("get new zh word id: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO words (text, language) VALUES (?, ?)`, transText, lang,
	); err != nil {
		return fmt.Errorf("upsert %s word: %w", lang, err)
	}
	var transID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM words WHERE text = ? AND language = ?`, transText, lang,
	).Scan(&transID); err != nil {
		return fmt.Errorf("get %s word id: %w", lang, err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO translations (translation_word_id, zh_word_id) VALUES (?, ?)`, transID, zhID,
	); err != nil {
		return fmt.Errorf("link %s translation: %w", lang, err)
	}

	return tx.Commit()
}
