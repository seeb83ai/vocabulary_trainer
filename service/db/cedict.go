package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
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
		for _, part := range strings.Split(d, ";") {
			if s := strings.TrimSpace(part); s != "" {
				defs = append(defs, s)
			}
		}
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
	if err != nil {
		return err
	}
	if !ok {
		// No cedict data: add each Han character that has a hanzi definition
		// as a component so it gets trained even without segmentation.
		dueDate := time.Now().UTC().Format("2006-01-02 15:04:05")
		for _, r := range []rune(zhText) {
			ch := string(r)
			var def string
			if err := s.db.QueryRowContext(ctx,
				`SELECT COALESCE(definition, '') FROM hanzi_decomposition WHERE character = ?`, ch,
			).Scan(&def); err != nil || def == "" {
				continue
			}
			if _, err := s.db.ExecContext(ctx,
				`INSERT OR IGNORE INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
				userID, ch, dueDate,
			); err != nil {
				log.Printf("CreateSubwordsForWord: add component %q (no cedict): %v", ch, err)
			}
		}
		return nil
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
		if tok.Text == zhText {
			// The whole parent word matched as one token (e.g. 炒饭→炒饭).
			// Fall through to the character-level pass below instead.
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

	// Single-char component pass: for each IsSingle token (no cedict entry),
	// add it to component_progress if it has a hanzi definition. This ensures
	// that un-segmented top-level characters get trained as components.
	dueDate := time.Now().UTC().Format("2006-01-02 15:04:05")
	for _, tok := range tokens {
		if !tok.IsSingle {
			continue
		}
		var def string
		err := s.db.QueryRowContext(ctx,
			`SELECT COALESCE(definition, '') FROM hanzi_decomposition WHERE character = ?`, tok.Text,
		).Scan(&def)
		if err != nil || def == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
			userID, tok.Text, dueDate,
		); err != nil {
			log.Printf("CreateSubwordsForWord: add component %q: %v", tok.Text, err)
		}
	}

	// Character-level pass: for each rune in the parent word, create a
	// sub-word if it has a cedict definition and isn't already in the user's
	// vocabulary. This handles the case where the whole parent matched as one
	// token (e.g. 炒饭 → 炒饭), leaving the constituent chars unprocessed.
	for _, r := range []rune(zhText) {
		ch := string(r)
		en, de, pinyin, found, err := lookupCedictToken(ctx, s.db, ch)
		if err != nil {
			return fmt.Errorf("cedict char lookup %q: %w", ch, err)
		}
		if !found || (en == "" && de == "") {
			continue
		}
		exists, err := s.IsZhWordForUser(ctx, userID, ch)
		if err != nil {
			return fmt.Errorf("check existing char subword %q: %w", ch, err)
		}
		if exists {
			continue
		}
		tok := SegmentToken{Text: ch, IsSingle: false, Pinyin: pinyin, DefinitionEN: en, DefinitionDE: de}
		if err := s.createSubword(ctx, userID, tok, subTags); err != nil {
			return fmt.Errorf("create char subword %q: %w", ch, err)
		}
	}
	return nil
}

// deriveSubwordTags returns the same tags as the parent word so subwords are
// findable under the same tag filter. An untagged parent produces no tags.
func deriveSubwordTags(parentTags []string) []string {
	return parentTags
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
		for _, sense := range strings.Split(pair.def, ";") {
			sense = strings.TrimSpace(sense)
			if sense == "" {
				continue
			}
			transID, err := upsertWord(ctx, tx, sense, pair.lang, nil, userID)
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
	}

	if err := setWordTags(ctx, tx, zhID, tags); err != nil {
		return err
	}

	return tx.Commit()
}
