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
func loadTranslationsForCard(ctx context.Context, store translationCandidateStore, wordID int64, lang string, settings *models.UserSettings, extraSlots int) (shown []string, extra []string, err error) {
	if settings == nil || !settings.TranslationRankingEnabled {
		words, err := store.GetTranslationsForWord(ctx, wordID, lang)
		if err != nil {
			return nil, nil, err
		}
		texts := make([]string, len(words))
		for i, w := range words {
			texts[i] = w.Text
		}
		return texts, nil, nil
	}

	rows, err := store.GetTranslationCandidatesForWord(ctx, wordID, lang)
	if err != nil {
		return nil, nil, err
	}
	candidates := make([]translationCandidate, len(rows))
	for i, row := range rows {
		c := translationCandidate{Text: row.Text, Source: row.Source}
		if row.Rank != nil {
			c.Rank = sql.NullInt64{Int64: *row.Rank, Valid: true}
		}
		candidates[i] = c
	}
	shown, extra = filterTranslationsForDisplay(candidates, settings.MaxTranslationsShown+extraSlots, settings.TranslationHideUnranked)
	return shown, extra, nil
}

// loadTranslationsForResult builds the capped/extra translation maps shown
// on the answer result screen, covering every language the word has a
// translation in (mirrors zhWord.Translations' language coverage, unlike
// loadTranslationsForCard which is called per-lang for a specific card).
func loadTranslationsForResult(ctx context.Context, store translationCandidateStore, wordID int64, allTranslations map[string][]string, settings *models.UserSettings) (shown map[string][]string, extra map[string][]string, err error) {
	shown = map[string][]string{}
	extra = map[string][]string{}
	for lang := range allTranslations {
		texts, extraTexts, err := loadTranslationsForCard(ctx, store, wordID, lang, settings, 0)
		if err != nil {
			return nil, nil, err
		}
		if len(texts) > 0 {
			shown[lang] = texts
		}
		if len(extraTexts) > 0 {
			extra[lang] = extraTexts
		}
	}
	return shown, extra, nil
}

// translationCandidate is one linked translation word with its stored
// importance data (see db.computeTranslationRank / translations.source).
type translationCandidate struct {
	Text   string
	Source string // "user" or "cedict"
	Rank   sql.NullInt64
}

// filterTranslationsForDisplay picks which translations to show on a quiz
// card when translation ranking is enabled, and which to collapse into
// "extra" for the frontend to reveal on demand. User-added translations and
// (unless hideUnranked is set) unranked CEDICT/HanDeDict glosses are
// prioritized into the visible set first; the remaining ranked glosses are
// sorted rarest-last after them. All of it — prioritized and ranked alike —
// still counts against maxShown: nothing bypasses the cap, it's just ordered
// so the most important entries are the ones kept visible (issue
// #431/#432/#433). Relative order within each group is preserved (stable)
// so results don't jitter across calls with identical rank/source data.
func filterTranslationsForDisplay(candidates []translationCandidate, maxShown int, hideUnranked bool) (shown []string, extra []string) {
	var prioritized, ranked []translationCandidate
	for _, c := range candidates {
		if c.Source != "cedict" {
			prioritized = append(prioritized, c)
			continue
		}
		if !c.Rank.Valid && !hideUnranked {
			prioritized = append(prioritized, c)
			continue
		}
		ranked = append(ranked, c)
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return rankValue(ranked[i]) < rankValue(ranked[j])
	})

	ordered := make([]translationCandidate, 0, len(prioritized)+len(ranked))
	ordered = append(ordered, prioritized...)
	ordered = append(ordered, ranked...)

	if maxShown < 0 {
		maxShown = 0
	}
	if maxShown > len(ordered) {
		maxShown = len(ordered)
	}

	shown = candidateTexts(ordered[:maxShown])
	extra = candidateTexts(ordered[maxShown:])
	return shown, extra
}

func candidateTexts(candidates []translationCandidate) []string {
	if len(candidates) == 0 {
		return nil
	}
	texts := make([]string, len(candidates))
	for i, c := range candidates {
		texts[i] = c.Text
	}
	return texts
}

func rankValue(c translationCandidate) int64 {
	if !c.Rank.Valid {
		return math.MaxInt64
	}
	return c.Rank.Int64
}
