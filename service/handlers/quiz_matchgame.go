package handlers

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// Match-game mode keys (issue #288). gameModeMismatch is the pre-existing
// component-confusion-pair mode; the other three are word-based.
const (
	gameModeMismatch     = "mismatch"
	gameModeNewest       = "newest"
	gameModeHardest      = "hardest"
	gameModeLastMistakes = "last_mistakes"
)

// matchGameMinCandidates is the minimum number of eligible words a mode needs
// to be considered playable — mirrors the pre-existing mismatch-game
// threshold (2 confusion pairs, i.e. at least 2 confusable entities).
const matchGameMinCandidates = 2

// matchGameRoundSize is how many words a word-based mode (newest/hardest/
// last-mistakes) fetches for one round, mirroring the up-to-4-word rounds the
// mismatch mode already produces from 2 confusion pairs.
const matchGameRoundSize = 4

// MatchGame handles GET /api/quiz/match-game. It picks uniformly at random
// among the user's enabled game modes that currently have enough eligible
// words (matchGameMinCandidates), tries another enabled mode if the first
// pick comes up short, marks the returned words/pairs as shown for that mode,
// and returns {words:[]} if no enabled mode has enough candidates.
func (h *QuizHandler) MatchGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserIDFromContext(ctx)

	st, err := h.Store.GetUserSettings(ctx, userID)
	if err != nil {
		internalError(w, err)
		return
	}

	var candidateModes []string
	for _, m := range []struct {
		name    string
		enabled bool
	}{
		{gameModeMismatch, st.GameModeMismatch},
		{gameModeNewest, st.GameModeNewest},
		{gameModeHardest, st.GameModeHardest},
		{gameModeLastMistakes, st.GameModeLastMistakes},
	} {
		if m.enabled {
			candidateModes = append(candidateModes, m.name)
		}
	}
	rand.Shuffle(len(candidateModes), func(i, j int) {
		candidateModes[i], candidateModes[j] = candidateModes[j], candidateModes[i]
	})

	for _, mode := range candidateModes {
		words, markShown, err := h.matchGameCandidates(ctx, userID, mode)
		if err != nil {
			internalError(w, err)
			return
		}
		if len(words) < matchGameMinCandidates {
			continue
		}
		markShown()
		writeJSON(w, http.StatusOK, models.MatchGameResponse{Words: words})
		return
	}

	writeJSON(w, http.StatusOK, models.MatchGameResponse{Words: []models.MatchGameWord{}})
}

// matchGameCandidates fetches the eligible word list for one game mode along
// with a markShown callback that stamps those candidates as shown (using
// whichever mechanism that mode uses) once the caller decides to use them.
func (h *QuizHandler) matchGameCandidates(ctx context.Context, userID int64, mode string) (words []models.MatchGameWord, markShown func(), err error) {
	noop := func() {}
	switch mode {
	case gameModeMismatch:
		since := time.Now().UTC().AddDate(0, 0, -7)
		pairs, err := h.Store.GetRecentMismatches(ctx, userID, since, 2)
		if err != nil {
			return nil, noop, err
		}
		if len(pairs) < 2 {
			return nil, noop, nil
		}
		words := mismatchPairsToMatchGameWords(pairs)
		markShown := func() {
			pairKeys := make([]db.ConfusionPairKey, len(pairs))
			for i, p := range pairs {
				pairKeys[i] = db.ConfusionPairKey{
					UserID:   userID,
					ZhWordID: p.ZhWordID, ZhComponent: p.ZhComponent,
					ConfusedWithID: p.ConfusedWithID, ConfusedWithComponent: p.ConfusedWithComponent,
					Mode: p.Mode,
				}
			}
			_ = h.Store.MarkConfusionsShownInGame(ctx, pairKeys)
		}
		return words, markShown, nil
	case gameModeNewest:
		words, err := h.Store.GetNewestWordsForGame(ctx, userID, matchGameRoundSize)
		if err != nil {
			return nil, noop, err
		}
		return words, h.markWordsShownFunc(ctx, userID, words, gameModeNewest), nil
	case gameModeHardest:
		words, err := h.Store.GetHardestWordsForGame(ctx, userID, matchGameRoundSize)
		if err != nil {
			return nil, noop, err
		}
		return words, h.markWordsShownFunc(ctx, userID, words, gameModeHardest), nil
	case gameModeLastMistakes:
		words, err := h.Store.GetLastMistakesForGame(ctx, userID, matchGameRoundSize)
		if err != nil {
			return nil, noop, err
		}
		return words, h.markWordsShownFunc(ctx, userID, words, gameModeLastMistakes), nil
	default:
		return nil, noop, nil
	}
}

// markWordsShownFunc builds a markShown callback for a word-based mode that
// uses the generic word_game_shown table.
func (h *QuizHandler) markWordsShownFunc(ctx context.Context, userID int64, words []models.MatchGameWord, mode string) func() {
	return func() {
		ids := make([]int64, len(words))
		for i, w := range words {
			ids[i] = w.ZhWordID
		}
		_ = h.Store.MarkWordsShownInGame(ctx, userID, ids, mode)
	}
}

// mismatchPairsToMatchGameWords collects the unique zh/confused-with entities
// across a set of confusion pairs into a flat, deduplicated MatchGameWord list.
func mismatchPairsToMatchGameWords(pairs []models.ConfusionDetail) []models.MatchGameWord {
	ptrStr := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	// Collect unique entities from both zh and confused_with sides of each pair.
	// Keyed by kind+id+character since every component shares the placeholder
	// word id 0 — an id-only key would wrongly collapse them into one entry.
	seen := map[string]bool{}
	var words []models.MatchGameWord
	for _, p := range pairs {
		for _, candidate := range []struct {
			kind, character string
			id              int64
			text, pinyin    string
			translations    map[string][]string
		}{
			{p.ZhKind, p.ZhComponent, p.ZhWordID, p.ZhText, ptrStr(p.ZhPinyin), p.ZhTranslations},
			{p.ConfusedWithKind, p.ConfusedWithComponent, p.ConfusedWithID, p.ConfusedWithText, ptrStr(p.ConfusedWithPinyin), p.ConfusedWithTranslations},
		} {
			key := candidate.kind + ":" + strconv.FormatInt(candidate.id, 10) + ":" + candidate.character
			if !seen[key] {
				seen[key] = true
				words = append(words, models.MatchGameWord{
					Kind:         candidate.kind,
					ZhWordID:     candidate.id,
					Character:    candidate.character,
					ZhText:       candidate.text,
					Pinyin:       candidate.pinyin,
					Translations: candidate.translations,
				})
			}
		}
	}
	return words
}

// MatchAnswer handles POST /api/quiz/match-answer.
// Updates SM-2 (word) or component-progress state after a match-game interaction.
func (h *QuizHandler) MatchAnswer(w http.ResponseWriter, r *http.Request) {
	var req models.MatchAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	userID := UserIDFromContext(r.Context())

	if req.Kind == models.ConfusionKindComponent {
		if req.Character == "" {
			writeError(w, http.StatusBadRequest, "character is required")
			return
		}
		progress, _, err := h.Store.RecordComponentAnswer(r.Context(), userID, req.Character, req.Correct)
		if err != nil {
			internalError(w, err)
			return
		}
		if err := h.Store.RecordComponentStat(r.Context(), userID, req.Correct); err != nil {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, models.AnswerResponse{
			Correct:       req.Correct,
			ZhText:        req.Character,
			TotalCorrect:  progress.TotalCorrect,
			TotalAttempts: progress.TotalAttempts,
			Tier:          componentTier(progress),
		})
		return
	}

	if req.ZhWordID <= 0 {
		writeError(w, http.StatusBadRequest, "zh_word_id is required")
		return
	}

	zhWord, err := h.Store.GetWordByID(r.Context(), userID, req.ZhWordID)
	if err != nil {
		internalError(w, err)
		return
	}
	if zhWord == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}

	progress, err := h.Store.GetSM2Progress(r.Context(), req.ZhWordID)
	if err != nil {
		internalError(w, err)
		return
	}
	if progress == nil {
		writeError(w, http.StatusNotFound, "progress not found")
		return
	}

	quality := sm2.QualityWrong
	if req.Correct {
		quality = sm2.QualityCorrect
	}

	updated := sm2.Update(*progress, quality)
	updated.TotalAttempts++
	if req.Correct {
		updated.TotalCorrect++
	}
	updated.StreakBonus = sm2.CalcStreakBonus(updated.StreakBonus, updated.Repetitions, updated.TotalCorrect, updated.TotalAttempts)

	if err := h.Store.UpdateSM2Progress(r.Context(), updated); err != nil {
		internalError(w, err)
		return
	}
	if err := h.Store.RecordAnswerTimestamps(r.Context(), req.ZhWordID, req.Correct); err != nil {
		log.Printf("match-answer: RecordAnswerTimestamps word %d: %v", req.ZhWordID, err)
	}

	writeJSON(w, http.StatusOK, models.AnswerResponse{
		Correct:       req.Correct,
		ZhText:        zhWord.ZhText,
		Pinyin:        zhWord.Pinyin,
		NextDue:       updated.DueDate,
		IntervalDays:  updated.IntervalDays,
		TotalCorrect:  updated.TotalCorrect,
		TotalAttempts: updated.TotalAttempts,
		StreakBonus:   updated.StreakBonus,
		Repetitions:   updated.Repetitions,
		Tier:          sm2.ClassifyTier(updated).String(),
	})
}
