package handlers

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strings"
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
		c := translationCandidate{Text: row.Text, Source: row.Source, DictPos: row.DictPos}
		if row.Rank != nil {
			c.Rank = sql.NullInt64{Int64: *row.Rank, Valid: true}
		}
		candidates[i] = c
	}
	shown, extra = filterTranslationsForDisplay(candidates, settings.MaxTranslationsShown+extraSlots, settings.TranslationHideUnranked, settings.TranslationUserOrder == "last")
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
	Text    string
	Source  string // "user" or "cedict"
	Rank    sql.NullInt64
	DictPos *models.DictPosition // position in the dictionary entry (issue #474); nil if unknown
}

// filterTranslationsForDisplay picks which translations to show on a quiz
// card when translation ranking is enabled, and which to collapse into
// "extra" for the frontend to reveal on demand. Candidates fall into four
// tiers: user-added translations; CEDICT/HanDeDict glosses found in the
// word's dictionary entry, in dictionary order — the first gloss of every
// sense, then the second ones, and so on, with Bsp.:/CL:/ZEW: annotations
// last (issue #474); unranked glosses without a dictionary position (unless
// hideUnranked is set, which files every unranked gloss at the end of the
// ranked tier); and ranked glosses without a dictionary position, sorted
// rarest-last. By default the tiers are ordered user → dictionary →
// unranked → ranked; userLast gives dictionary → ranked → unranked → user.
// Everything counts against maxShown: nothing bypasses the cap, it's just
// ordered so the most important entries are the ones kept visible (issue
// #431/#432/#433). Within each tier, entries shorter than
// maxPreferredTranslationLength runes are preferred over longer ones — a
// long gloss is unlikely to be the most useful translation to show first
// (issue #450) — but a tier that is entirely long entries still gets shown;
// length only reorders within a tier, it never lets one tier jump ahead of
// another. Relative order within each group is otherwise preserved
// (stable) so results don't jitter across calls with identical rank/source
// data.
func filterTranslationsForDisplay(candidates []translationCandidate, maxShown int, hideUnranked bool, userLast bool) (shown []string, extra []string) {
	var user, dict, unranked, ranked []translationCandidate
	for _, c := range candidates {
		switch {
		case c.Source != "cedict":
			user = append(user, c)
		case c.DictPos != nil && (c.Rank.Valid || !hideUnranked):
			dict = append(dict, c)
		case !c.Rank.Valid && !hideUnranked:
			unranked = append(unranked, c)
		default:
			ranked = append(ranked, c)
		}
	}

	shortFirst := func(group []translationCandidate) {
		sort.SliceStable(group, func(i, j int) bool {
			return !isLong(group[i]) && isLong(group[j])
		})
	}
	shortFirst(user)
	shortFirst(unranked)

	sort.SliceStable(dict, func(i, j int) bool {
		a, b := dict[i], dict[j]
		if isDictionaryAnnotation(a.Text) != isDictionaryAnnotation(b.Text) {
			return !isDictionaryAnnotation(a.Text)
		}
		if isLong(a) != isLong(b) {
			return !isLong(a)
		}
		if a.DictPos.Item != b.DictPos.Item {
			return a.DictPos.Item < b.DictPos.Item
		}
		return a.DictPos.Sense < b.DictPos.Sense
	})

	sort.SliceStable(ranked, func(i, j int) bool {
		if isLong(ranked[i]) != isLong(ranked[j]) {
			return !isLong(ranked[i])
		}
		return rankValue(ranked[i]) < rankValue(ranked[j])
	})

	tiers := [][]translationCandidate{user, dict, unranked, ranked}
	if userLast {
		tiers = [][]translationCandidate{dict, ranked, unranked, user}
	}
	ordered := make([]translationCandidate, 0, len(candidates))
	for _, tier := range tiers {
		ordered = append(ordered, tier...)
	}

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

// maxPreferredTranslationLength is the soft cap (in runes) below which a
// translation is preferred over a longer one within its tier — a long gloss
// is unlikely to be the most useful translation to show first (issue #450).
const maxPreferredTranslationLength = 15

func isLong(c translationCandidate) bool {
	return len([]rune(c.Text)) >= maxPreferredTranslationLength
}

// isDictionaryAnnotation reports whether a dictionary gloss is an example
// sentence or measure-word note (Bsp.:/CL:/ZEW:, see isNoise in
// frontend/train-answer.js) rather than a meaning; such glosses sort after
// the real meanings of the dictionary tier (issue #474).
func isDictionaryAnnotation(text string) bool {
	return strings.HasPrefix(text, "Bsp.:") || strings.HasPrefix(text, "CL:") || strings.HasPrefix(text, "ZEW:")
}
