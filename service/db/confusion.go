package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// DetectConfusion checks if the user's wrong answer matches a different known word.
// For zh_to_en / zh_pinyin_to_en: looks for a translation word (restricted to langs)
// whose text matches the answer, then returns the zh word it belongs to (if different
// from zhWordID).
// For en_to_zh: looks for a ZH word whose text matches the answer (if different from zhWordID).
// Returns (confusedWithID, true, nil) if a confusion is found, (0, false, nil) if not.
func (s *Store) DetectConfusion(ctx context.Context, userID, zhWordID int64, answer, mode string, langs []string) (int64, bool, error) {
	normalized := sm2.NormalizeAnswer(answer)
	if normalized == "" {
		return 0, false, nil
	}

	switch mode {
	case models.ModeZhToTransl, models.ModeZhPinyinToTransl, models.ModeZhToTranslNoSound, models.ModeVoiceToTransl:
		// Fetch all translation words for the user across ALL languages (excluding
		// translations of the word being quizzed), then match in Go using ExpandVariants.
		// Language-agnostic: when transl_to_zh has no translations in the selected lang
		// it falls back to zh_to_transl, so the typed answer may be in any language.
		// Go-level matching also handles SQLite LOWER() not folding non-ASCII (umlauts)
		// and slash-separated alternatives (e.g. "dog / hound").
		rows, qErr := s.db.QueryContext(ctx, `
			SELECT t.zh_word_id, w.text FROM words w
			JOIN translations t ON t.translation_word_id = w.id
			JOIN words wz ON wz.id = t.zh_word_id
			WHERE t.zh_word_id != ?
			  AND wz.user_id = ?`, zhWordID, userID)
		if qErr != nil {
			return 0, false, fmt.Errorf("lookup confusion: %w", qErr)
		}
		defer rows.Close()
		for rows.Next() {
			var zhID int64
			var text string
			if sErr := rows.Scan(&zhID, &text); sErr != nil {
				return 0, false, fmt.Errorf("scan confusion: %w", sErr)
			}
			for _, v := range sm2.ExpandVariants(text) {
				if v == normalized {
					_ = rows.Close()
					return zhID, true, nil
				}
			}
		}
		return 0, false, rows.Err()

	case models.ModeTranslToZh:
		// First: find a ZH word whose text matches the answer (user typed Chinese).
		var confusedWithID int64
		err := s.db.QueryRowContext(ctx, `
			SELECT id FROM words
			WHERE language = 'zh' AND LOWER(TRIM(text)) = ?
			  AND id != ? AND user_id = ?
			LIMIT 1`, normalized, zhWordID, userID).Scan(&confusedWithID)
		if err != nil && err != sql.ErrNoRows {
			return 0, false, fmt.Errorf("lookup confusion: %w", err)
		}
		if err == nil {
			return confusedWithID, true, nil
		}

		// Second: user may have typed a translation of a different zh word.
		// Fetch all translation words for the user (excluding current word's
		// translations) and match in Go using ExpandVariants so that umlauts
		// and slash-separated alternatives are handled correctly.
		if len(langs) == 0 {
			langs = []string{"en"}
		}
		placeholders := make([]string, len(langs))
		args := make([]any, 0, len(langs)+2)
		for i, l := range langs {
			placeholders[i] = "?"
			args = append(args, l)
		}
		args = append(args, zhWordID, userID)
		rows, qErr := s.db.QueryContext(ctx, `
			SELECT t.zh_word_id, w.text FROM words w
			JOIN translations t ON t.translation_word_id = w.id
			JOIN words wz ON wz.id = t.zh_word_id
			WHERE w.language IN (`+strings.Join(placeholders, ",")+`)
			  AND t.zh_word_id != ?
			  AND wz.user_id = ?`, args...)
		if qErr != nil {
			return 0, false, fmt.Errorf("lookup confusion translations: %w", qErr)
		}
		defer rows.Close()
		for rows.Next() {
			var zhID int64
			var text string
			if sErr := rows.Scan(&zhID, &text); sErr != nil {
				return 0, false, fmt.Errorf("scan confusion: %w", sErr)
			}
			for _, v := range sm2.ExpandVariants(text) {
				if v == normalized {
					_ = rows.Close()
					return zhID, true, nil
				}
			}
		}
		return 0, false, rows.Err()

	default:
		return 0, false, nil
	}
}

// upsertConfusion records or increments a confusion pair. Exactly one of
// zhWordID/zhComponent and one of confusedWithID/confusedWithComponent should
// be set (the other left at its zero value) on each side, per the caller's
// detection result. userID is required directly (rather than inferred by
// joining through words) because component characters, unlike word ids, are
// not inherently scoped to one user.
func (s *Store) upsertConfusion(ctx context.Context, userID, zhWordID int64, zhComponent string, confusedWithID int64, confusedWithComponent, mode string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO confusion_pairs (user_id, zh_word_id, zh_component, confused_with_id, confused_with_component, mode, count, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, zh_word_id, zh_component, confused_with_id, confused_with_component, mode)
		DO UPDATE SET count = count + 1, last_seen = CURRENT_TIMESTAMP`,
		userID, zhWordID, zhComponent, confusedWithID, confusedWithComponent, mode)
	if err != nil {
		return fmt.Errorf("upsert confusion: %w", err)
	}
	return nil
}

// UpsertConfusion records or increments a word-vs-word confusion pair.
func (s *Store) UpsertConfusion(ctx context.Context, userID, zhWordID, confusedWithID int64, mode string) error {
	return s.upsertConfusion(ctx, userID, zhWordID, "", confusedWithID, "", mode)
}

// GetConfusionDetail returns a single word-vs-word ConfusionDetail for use in
// the vocabulary-word answer response.
func (s *Store) GetConfusionDetail(ctx context.Context, userID, zhWordID, confusedWithID int64, mode string, langs []string) (*models.ConfusionDetail, error) {
	var d models.ConfusionDetail
	var lastSeen string
	err := s.db.QueryRowContext(ctx, `
		SELECT cp.zh_word_id, wz.text, wz.pinyin,
		       cp.confused_with_id, wc.text, wc.pinyin,
		       cp.mode, cp.count, cp.last_seen
		FROM confusion_pairs cp
		JOIN words wz ON wz.id = cp.zh_word_id
		JOIN words wc ON wc.id = cp.confused_with_id
		WHERE cp.user_id = ? AND cp.zh_word_id = ? AND cp.confused_with_id = ? AND cp.mode = ?`,
		userID, zhWordID, confusedWithID, mode).Scan(
		&d.ZhWordID, &d.ZhText, &d.ZhPinyin,
		&d.ConfusedWithID, &d.ConfusedWithText, &d.ConfusedWithPinyin,
		&d.Mode, &d.Count, &lastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get confusion detail: %w", err)
	}
	d.ZhKind = models.ConfusionKindWord
	d.ConfusedWithKind = models.ConfusionKindWord
	d.LastSeen = parseDateTime(lastSeen)
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	d.ZhTranslations = map[string][]string{}
	d.ConfusedWithTranslations = map[string][]string{}
	for _, lang := range langs {
		texts, ferr := s.getTranslationTextsForZhWord(ctx, zhWordID, lang)
		if ferr != nil {
			return nil, ferr
		}
		if len(texts) > 0 {
			d.ZhTranslations[lang] = texts
		}
		texts, ferr = s.getTranslationTextsForZhWord(ctx, confusedWithID, lang)
		if ferr != nil {
			return nil, ferr
		}
		if len(texts) > 0 {
			d.ConfusedWithTranslations[lang] = texts
		}
	}
	return &d, nil
}

// rawConfusionRow is the shape of a confusion_pairs row before its word/component
// sides have been resolved to display data.
type rawConfusionRow struct {
	zhWordID, confusedWithID           int64
	zhComponent, confusedWithComponent string
	mode                               string
	count                              int
	lastSeen                           string
}

// resolveConfusionEntity resolves one side of a confusion pair (word if wordID
// != 0, otherwise component) into display data. ok=false means the referenced
// word no longer exists (e.g. a pre-cleanup dangling row) and the row should
// be skipped, matching the old behaviour of silently dropping such rows via
// an INNER JOIN.
func (s *Store) resolveConfusionEntity(ctx context.Context, userID, wordID int64, component string, langs []string) (kind, text string, pinyin *string, translations map[string][]string, ok bool, err error) {
	translations = map[string][]string{}
	if wordID != 0 {
		var t string
		var p sql.NullString
		err = s.db.QueryRowContext(ctx, `SELECT text, pinyin FROM words WHERE id = ? AND user_id = ?`, wordID, userID).Scan(&t, &p)
		if err == sql.ErrNoRows {
			return "", "", nil, nil, false, nil
		}
		if err != nil {
			return "", "", nil, nil, false, fmt.Errorf("resolve confusion word %d: %w", wordID, err)
		}
		if p.Valid {
			pinyin = &p.String
		}
		for _, lang := range langs {
			texts, ferr := s.getTranslationTextsForZhWord(ctx, wordID, lang)
			if ferr != nil {
				return "", "", nil, nil, false, ferr
			}
			if len(texts) > 0 {
				translations[lang] = texts
			}
		}
		return models.ConfusionKindWord, t, pinyin, translations, true, nil
	}

	// Component side.
	if py := s.GetComponentPinyin(ctx, component); py != "" {
		pinyin = &py
	}
	defs, dErr := s.GetComponentDefinitions(ctx, userID, component, langs)
	if dErr != nil {
		return "", "", nil, nil, false, dErr
	}
	for lang, def := range defs {
		translations[lang] = []string{def}
	}
	return models.ConfusionKindComponent, component, pinyin, translations, true, nil
}

func (s *Store) hydrateConfusionRows(ctx context.Context, userID int64, raws []rawConfusionRow) ([]models.ConfusionDetail, error) {
	allLangs, err := s.GetTranslationLanguages(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]models.ConfusionDetail, 0, len(raws))
	for _, r := range raws {
		d := models.ConfusionDetail{Mode: r.mode, Count: r.count, LastSeen: parseDateTime(r.lastSeen)}
		var zhOK, cwOK bool
		d.ZhKind, d.ZhText, d.ZhPinyin, d.ZhTranslations, zhOK, err = s.resolveConfusionEntity(ctx, userID, r.zhWordID, r.zhComponent, allLangs)
		if err != nil {
			return nil, err
		}
		d.ConfusedWithKind, d.ConfusedWithText, d.ConfusedWithPinyin, d.ConfusedWithTranslations, cwOK, err =
			s.resolveConfusionEntity(ctx, userID, r.confusedWithID, r.confusedWithComponent, allLangs)
		if err != nil {
			return nil, err
		}
		if !zhOK || !cwOK {
			continue
		}
		d.ZhWordID, d.ZhComponent = r.zhWordID, r.zhComponent
		d.ConfusedWithID, d.ConfusedWithComponent = r.confusedWithID, r.confusedWithComponent
		items = append(items, d)
	}
	return items, nil
}

// GetConfusions returns all confusion pairs for the given user, ordered by last_seen DESC.
func (s *Store) GetConfusions(ctx context.Context, userID int64) ([]models.ConfusionDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT zh_word_id, zh_component, confused_with_id, confused_with_component, mode, count, last_seen
		FROM confusion_pairs
		WHERE user_id = ?
		ORDER BY last_seen DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get confusions: %w", err)
	}
	var raws []rawConfusionRow
	for rows.Next() {
		var r rawConfusionRow
		if err := rows.Scan(&r.zhWordID, &r.zhComponent, &r.confusedWithID, &r.confusedWithComponent, &r.mode, &r.count, &r.lastSeen); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan confusion: %w", err)
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	items, err := s.hydrateConfusionRows(ctx, userID, raws)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []models.ConfusionDetail{}
	}
	return items, nil
}

// GetRecentMismatches returns confusion pairs with last_seen >= since that have
// not yet been shown in a game (or have been re-confused since last shown), up to limit rows.
func (s *Store) GetRecentMismatches(ctx context.Context, userID int64, since time.Time, limit int) ([]models.ConfusionDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT zh_word_id, zh_component, confused_with_id, confused_with_component, mode, count, last_seen
		FROM confusion_pairs
		WHERE user_id = ?
		  AND last_seen >= ?
		  AND (last_shown_in_game IS NULL OR last_seen > last_shown_in_game)
		ORDER BY last_seen DESC
		LIMIT ?`,
		userID, since.UTC().Format("2006-01-02 15:04:05"), limit)
	if err != nil {
		return nil, fmt.Errorf("get recent mismatches: %w", err)
	}
	var raws []rawConfusionRow
	for rows.Next() {
		var r rawConfusionRow
		if err := rows.Scan(&r.zhWordID, &r.zhComponent, &r.confusedWithID, &r.confusedWithComponent, &r.mode, &r.count, &r.lastSeen); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan recent mismatch: %w", err)
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	items, err := s.hydrateConfusionRows(ctx, userID, raws)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []models.ConfusionDetail{}
	}
	return items, nil
}

// ConfusionPairKey identifies one side-pair of a confusion_pairs row (each
// side being either a word id or a component character). UserID is required
// directly (rather than inferred by joining through words) because component
// characters, unlike word ids, are not inherently scoped to one user — the
// same character can be trained, and independently confused, by many users.
type ConfusionPairKey struct {
	UserID                int64
	ZhWordID              int64
	ZhComponent           string
	ConfusedWithID        int64
	ConfusedWithComponent string
	Mode                  string
}

// MarkConfusionsShownInGame stamps the given pairs with the current time so
// they are excluded from future match-game sessions unless the user confuses
// them again after this timestamp.
func (s *Store) MarkConfusionsShownInGame(ctx context.Context, pairs []ConfusionPairKey) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for _, p := range pairs {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE confusion_pairs SET last_shown_in_game = ?
			 WHERE user_id = ? AND zh_word_id = ? AND zh_component = ? AND confused_with_id = ? AND confused_with_component = ? AND mode = ?`,
			now, p.UserID, p.ZhWordID, p.ZhComponent, p.ConfusedWithID, p.ConfusedWithComponent, p.Mode,
		); err != nil {
			return fmt.Errorf("mark confusion shown: %w", err)
		}
	}
	return nil
}
