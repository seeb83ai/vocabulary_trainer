package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// SegmentToken is one piece of a segmented zh string: either a single Han
// character with no dictionary match (a component candidate) or a substring
// that matched a cedict_entries row (a sub-word candidate).
type SegmentToken struct {
	Text         string
	IsSingle     bool
	Pinyin       string
	DefinitionEN string
	DefinitionDE string
}

const (
	// maxSegmentableRunes guards segmentation against sentence-length input.
	// Chengyu are exactly 4 characters and most CEDICT compounds are 2-3
	// characters; doubling to 8 covers slightly longer fixed phrases while
	// staying well short of anything that reads as a sentence.
	maxSegmentableRunes = 8
	maxCedictMatchRunes = 8
)

// sentenceGuardPunct are sentence-terminating punctuation marks. Their
// presence anywhere in a short string is treated as a signal that it isn't a
// flat compound word, independent of the length guard.
const sentenceGuardPunct = "。.!！?？"

// segmentZhText splits zhText into SegmentTokens using forward maximum-match
// against cedict_entries. Returns ok=false (no tokens) when zhText looks like
// a sentence rather than a short vocabulary entry: longer than
// maxSegmentableRunes characters, or containing sentence-terminating
// punctuation. An empty cedict_entries table degrades gracefully: every rune
// becomes its own IsSingle token, never an error.
func segmentZhText(ctx context.Context, q querier, zhText string) ([]SegmentToken, bool, error) {
	runes := []rune(zhText)
	if len(runes) == 0 || len(runes) > maxSegmentableRunes {
		return nil, false, nil
	}
	if strings.ContainsAny(zhText, sentenceGuardPunct) {
		return nil, false, nil
	}

	var tokens []SegmentToken
	for i := 0; i < len(runes); {
		maxLen := maxCedictMatchRunes
		if rem := len(runes) - i; rem < maxLen {
			maxLen = rem
		}
		matched := false
		for l := maxLen; l >= 2; l-- {
			cand := string(runes[i : i+l])
			en, de, pinyin, found, err := lookupCedictToken(ctx, q, cand)
			if err != nil {
				return nil, false, err
			}
			if found {
				tokens = append(tokens, SegmentToken{Text: cand, Pinyin: pinyin, DefinitionEN: en, DefinitionDE: de})
				i += l
				matched = true
				break
			}
		}
		if !matched {
			tokens = append(tokens, SegmentToken{Text: string(runes[i]), IsSingle: true})
			i++
		}
	}
	return tokens, true, nil
}

// lookupCedictToken checks whether simplified exists in cedict_entries at
// all (in either language), and if so fetches its EN and DE definitions.
// Pinyin prefers the EN (CC-CEDICT) row, falling back to the DE row.
func lookupCedictToken(ctx context.Context, q querier, simplified string) (en, de, pinyin string, found bool, err error) {
	var exists int
	err = q.QueryRowContext(ctx, `SELECT 1 FROM cedict_entries WHERE simplified = ? LIMIT 1`, simplified).Scan(&exists)
	if err == sql.ErrNoRows {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, fmt.Errorf("cedict lookup %q: %w", simplified, err)
	}

	var enDef, enPinyin sql.NullString
	err = q.QueryRowContext(ctx,
		`SELECT definition, pinyin FROM cedict_entries WHERE simplified = ? AND lang = 'en' ORDER BY id LIMIT 1`,
		simplified,
	).Scan(&enDef, &enPinyin)
	if err != nil && err != sql.ErrNoRows {
		return "", "", "", false, fmt.Errorf("cedict en lookup %q: %w", simplified, err)
	}

	var deDef, dePinyin sql.NullString
	err = q.QueryRowContext(ctx,
		`SELECT definition, pinyin FROM cedict_entries WHERE simplified = ? AND lang = 'de' ORDER BY id LIMIT 1`,
		simplified,
	).Scan(&deDef, &dePinyin)
	if err != nil && err != sql.ErrNoRows {
		return "", "", "", false, fmt.Errorf("cedict de lookup %q: %w", simplified, err)
	}

	pinyin = enPinyin.String
	if pinyin == "" {
		pinyin = dePinyin.String
	}
	return enDef.String, deDef.String, pinyin, true, nil
}

// LookupDictionary returns every cedict_entries definition for an exact
// simplified-text match in the given language ("en" or "de"), used by the
// free dictionary-lookup step in the word add/edit "Translate" flow. Returns
// an empty slice (not an error) when nothing matches.
func (s *Store) LookupDictionary(ctx context.Context, simplified, lang string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT definition FROM cedict_entries WHERE simplified = ? AND lang = ? ORDER BY id`,
		simplified, strings.ToLower(lang))
	if err != nil {
		return nil, fmt.Errorf("lookup dictionary %q/%s: %w", simplified, lang, err)
	}
	defer rows.Close()
	var defs []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan dictionary row: %w", err)
		}
		defs = append(defs, d)
	}
	return defs, rows.Err()
}

// SeedCedictEntryForTest inserts a cedict_entries row. Intended for use in tests only.
func (s *Store) SeedCedictEntryForTest(ctx context.Context, simplified, lang, pinyin, definition string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cedict_entries (simplified, lang, pinyin, definition) VALUES (?, ?, ?, ?)`,
		simplified, lang, pinyin, definition)
	return err
}

// CreateSubwordsForWord segments zhText and, for every multi-character token
// found, creates it as a new inert vocabulary word for userID — never
// acknowledged, so it queues into the same MAX_NEW_WORDS-gated pipeline as
// any manually-added word rather than bypassing the daily new-word cap.
// Tokens already tracked as one of the user's own zh words are skipped
// (existing tags left untouched). Tags are derived from zhWordID's own tags,
// each suffixed "-sub" (e.g. HSK1 -> HSK1-sub), or the single tag "sub" when
// the word has no tags. Intended to be called once, right after
// InitComponentsForWord, at the same points a word enters active training;
// callers treat it as best-effort (log on error, don't fail the caller).
func (s *Store) CreateSubwordsForWord(ctx context.Context, userID, zhWordID int64, zhText string) error {
	if len([]rune(zhText)) <= 1 {
		return nil
	}
	tokens, ok, err := segmentZhText(ctx, s.db, zhText)
	if err != nil || !ok {
		return err
	}

	parentTags, err := s.getTagsForWord(ctx, zhWordID)
	if err != nil {
		return fmt.Errorf("get parent tags for subword creation: %w", err)
	}
	subTags := deriveSubwordTags(parentTags)

	for _, tok := range tokens {
		if tok.IsSingle || (tok.DefinitionEN == "" && tok.DefinitionDE == "") {
			continue
		}
		exists, err := s.IsZhWordForUser(ctx, userID, tok.Text)
		if err != nil {
			return fmt.Errorf("check existing subword %q: %w", tok.Text, err)
		}
		if exists {
			continue
		}
		if err := s.createSubword(ctx, userID, tok, subTags); err != nil {
			return fmt.Errorf("create subword %q: %w", tok.Text, err)
		}
	}
	return nil
}

// deriveSubwordTags suffixes every parent tag with "-sub" (e.g. HSK1 ->
// HSK1-sub); an untagged parent produces the single generic tag "sub".
func deriveSubwordTags(parentTags []string) []string {
	if len(parentTags) == 0 {
		return []string{"sub"}
	}
	tags := make([]string, len(parentTags))
	for i, tag := range parentTags {
		tags[i] = tag + "-sub"
	}
	return tags
}

// createSubword inserts one auto-created sub-word (zh word + EN/DE
// translations + tags) in its own transaction, matching the shape of
// CreateWord but always inert (never acknowledged/started training).
func (s *Store) createSubword(ctx context.Context, userID int64, tok SegmentToken, tags []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin subword tx: %w", err)
	}
	defer tx.Rollback()

	var pinyin *string
	if tok.Pinyin != "" {
		pinyin = &tok.Pinyin
	}
	zhID, err := upsertWord(ctx, tx, tok.Text, "zh", pinyin, userID)
	if err != nil {
		return err
	}
	if err := initSM2(ctx, tx, zhID); err != nil {
		return err
	}

	for _, pair := range []struct{ lang, def string }{{"en", tok.DefinitionEN}, {"de", tok.DefinitionDE}} {
		if pair.def == "" {
			continue
		}
		transID, err := upsertWord(ctx, tx, pair.def, pair.lang, nil, userID)
		if err != nil {
			return err
		}
		if err := initSM2(ctx, tx, transID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO translations (translation_word_id, zh_word_id) VALUES (?, ?)`,
			transID, zhID); err != nil {
			return fmt.Errorf("link %s subword translation: %w", pair.lang, err)
		}
	}

	if err := setWordTags(ctx, tx, zhID, tags); err != nil {
		return err
	}

	return tx.Commit()
}
