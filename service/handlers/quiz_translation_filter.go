package handlers

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"vocabulary_trainer/models"
)

// translationCandidateStore is the minimal store surface needed to load
// ranked translations for a quiz card.
type translationCandidateStore interface {
	GetTranslationCandidatesForWord(ctx context.Context, wordID int64, targetLang string) ([]models.TranslationCandidate, error)
	GetTranslationsForWord(ctx context.Context, wordID int64, targetLang string) ([]models.Word, error)
}

// loadTranslationsForCard returns the translation texts for wordID/lang to
// show on a quiz card, applying the user's translation-ranking settings when
// enabled. When ranking is disabled (or settings are nil), every linked
// translation is returned, matching the pre-ranking behavior.
//
// extraSlots raises the effective cap by that many entries. The transl_to_zh
// card pulls its prompt word out of this same list and the frontend then
// excludes it from the displayed hint (train-card.js), so that caller passes
// extraSlots=1 — otherwise "max shown" translations become max-1 visible
// once the prompt is drawn from them. Every other caller passes 0.
func loadTranslationsForCard(ctx context.Context, store translationCandidateStore, wordID int64, lang string, settings *models.UserSettings, extraSlots int) ([]string, error) {
	if settings == nil || !settings.TranslationRankingEnabled {
		words, err := store.GetTranslationsForWord(ctx, wordID, lang)
		if err != nil {
			return nil, err
		}
		texts := make([]string, len(words))
		for i, w := range words {
			texts[i] = w.Text
		}
		return texts, nil
	}

	rows, err := store.GetTranslationCandidatesForWord(ctx, wordID, lang)
	if err != nil {
		return nil, err
	}
	candidates := make([]translationCandidate, len(rows))
	for i, row := range rows {
		c := translationCandidate{Text: row.Text, Source: row.Source}
		if row.Rank != nil {
			c.Rank = sql.NullInt64{Int64: *row.Rank, Valid: true}
		}
		candidates[i] = c
	}
	return filterTranslationsForDisplay(candidates, settings.MaxTranslationsShown+extraSlots, settings.TranslationHideUnranked), nil
}

// translationCandidate is one linked translation word with its stored
// importance data (see db.computeTranslationRank / translations.source).
type translationCandidate struct {
	Text   string
	Source string // "user" or "cedict"
	Rank   sql.NullInt64
}

// filterTranslationsForDisplay picks which translations to show on a quiz
// card when translation ranking is enabled. User-added translations are
// always shown. Among CEDICT/HanDeDict-derived translations, unranked ones
// (no frequency-list match) are always shown too unless hideUnranked is set,
// in which case they're treated as lowest priority like any other ranked
// gloss. The remaining ranked glosses are sorted rarest-last and capped at
// maxShown total (counting only the capped set, not the always-shown ones).
// Relative order within each group is preserved (stable) so results don't
// jitter across calls with identical rank/source data.
func filterTranslationsForDisplay(candidates []translationCandidate, maxShown int, hideUnranked bool) []string {
	var alwaysShown, ranked []translationCandidate
	for _, c := range candidates {
		if c.Source != "cedict" {
			alwaysShown = append(alwaysShown, c)
			continue
		}
		if !c.Rank.Valid && !hideUnranked {
			alwaysShown = append(alwaysShown, c)
			continue
		}
		ranked = append(ranked, c)
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return rankValue(ranked[i]) < rankValue(ranked[j])
	})

	if maxShown < len(ranked) {
		ranked = ranked[:maxShown]
	}

	texts := make([]string, 0, len(alwaysShown)+len(ranked))
	for _, c := range alwaysShown {
		texts = append(texts, c.Text)
	}
	for _, c := range ranked {
		texts = append(texts, c.Text)
	}
	return texts
}

func rankValue(c translationCandidate) int64 {
	if !c.Rank.Valid {
		return math.MaxInt64
	}
	return c.Rank.Int64
}
