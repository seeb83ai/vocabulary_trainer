package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// DetectComponentConfusion checks whether a component quiz's wrong answer
// matches a different known entity: either another of the user's trained
// components (by definition, in the given langs) or a translation of one of
// the user's zh words (any language, mirroring the word-side DetectConfusion
// breadth). Returns exactly one of (confusedWordID, confusedComponent) set
// when found=true.
func (s *Store) DetectComponentConfusion(ctx context.Context, userID int64, character, answer string, langs []string) (confusedWordID int64, confusedComponent string, found bool, err error) {
	if strings.TrimSpace(answer) == "" {
		return 0, "", false, nil
	}

	// 1. Scan the user's other trained components' definitions.
	rows, qErr := s.db.QueryContext(ctx,
		`SELECT character FROM component_progress WHERE user_id = ? AND character != ?`,
		userID, character)
	if qErr != nil {
		return 0, "", false, fmt.Errorf("lookup component confusion: %w", qErr)
	}
	var otherChars []string
	for rows.Next() {
		var c string
		if sErr := rows.Scan(&c); sErr != nil {
			rows.Close()
			return 0, "", false, fmt.Errorf("scan component confusion: %w", sErr)
		}
		otherChars = append(otherChars, c)
	}
	if err := rows.Err(); err != nil {
		return 0, "", false, err
	}
	rows.Close()

	for _, c := range otherChars {
		defs, dErr := s.GetComponentDefinitions(ctx, userID, c, langs)
		if dErr != nil {
			return 0, "", false, dErr
		}
		for _, def := range defs {
			if sm2.CheckComponentAnswer(answer, def) {
				return 0, c, true, nil
			}
		}
	}

	// 2. Scan all of the user's words' translations (excluding a word whose zh
	// text is the character itself, in case the component is also a word).
	normalized := sm2.NormalizeAnswer(answer)
	wRows, qErr := s.db.QueryContext(ctx, `
		SELECT t.zh_word_id, w.text FROM words w
		JOIN translations t ON t.translation_word_id = w.id
		JOIN words wz ON wz.id = t.zh_word_id
		WHERE wz.user_id = ? AND wz.text != ?`, userID, character)
	if qErr != nil {
		return 0, "", false, fmt.Errorf("lookup word confusion: %w", qErr)
	}
	defer wRows.Close()
	for wRows.Next() {
		var zhID int64
		var text string
		if sErr := wRows.Scan(&zhID, &text); sErr != nil {
			return 0, "", false, fmt.Errorf("scan word confusion: %w", sErr)
		}
		for _, v := range sm2.ExpandVariants(text) {
			if v == normalized {
				_ = wRows.Close()
				return zhID, "", true, nil
			}
		}
	}
	return 0, "", false, wRows.Err()
}

// UpsertComponentConfusion records or increments a confusion pair whose
// quizzed side is a component. Exactly one of confusedWithID/confusedWithComponent
// should be set, matching DetectComponentConfusion's result.
func (s *Store) UpsertComponentConfusion(ctx context.Context, userID int64, zhComponent string, confusedWithID int64, confusedWithComponent, mode string) error {
	return s.upsertConfusion(ctx, userID, 0, zhComponent, confusedWithID, confusedWithComponent, mode)
}

// GetComponentConfusionDetail returns a single ConfusionDetail for a
// component-originated confusion, for use in the component answer response.
// The confused-with side may itself be either a word or a component.
func (s *Store) GetComponentConfusionDetail(ctx context.Context, userID int64, zhComponent string, confusedWithID int64, confusedWithComponent, mode string, langs []string) (*models.ConfusionDetail, error) {
	var count int
	var lastSeen string
	err := s.db.QueryRowContext(ctx, `
		SELECT count, last_seen FROM confusion_pairs
		WHERE user_id = ? AND zh_word_id = 0 AND zh_component = ?
		  AND confused_with_id = ? AND confused_with_component = ? AND mode = ?`,
		userID, zhComponent, confusedWithID, confusedWithComponent, mode,
	).Scan(&count, &lastSeen)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get component confusion detail: %w", err)
	}

	d := &models.ConfusionDetail{Mode: mode, Count: count, LastSeen: parseDateTime(lastSeen)}
	var zhOK, cwOK bool
	d.ZhKind, d.ZhText, d.ZhPinyin, d.ZhTranslations, zhOK, err = s.resolveConfusionEntity(ctx, userID, 0, zhComponent, langs)
	if err != nil {
		return nil, err
	}
	d.ZhComponent = zhComponent
	d.ConfusedWithKind, d.ConfusedWithText, d.ConfusedWithPinyin, d.ConfusedWithTranslations, cwOK, err =
		s.resolveConfusionEntity(ctx, userID, confusedWithID, confusedWithComponent, langs)
	if err != nil {
		return nil, err
	}
	if !zhOK || !cwOK {
		return nil, nil
	}
	d.ConfusedWithID = confusedWithID
	d.ConfusedWithComponent = confusedWithComponent
	return d, nil
}
