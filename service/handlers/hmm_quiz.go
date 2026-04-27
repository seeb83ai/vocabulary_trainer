package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// hmmReParens matches parenthesised segments (and surrounding whitespace) that
// are optional in mnemonic names, e.g. "(人) Arnold" or "Kreuz (十) und ...".
var hmmReParens = regexp.MustCompile(`\s*\([^)]*\)\s*`)

type HMMQuizHandler struct {
	Store *db.Store
}

// hmmTier returns the accuracy bucket label for an HMM progress record.
func hmmTier(p models.HMMProgress) string {
	if p.TotalAttempts == 0 {
		return ""
	}
	if p.Learning {
		return "New"
	}
	acc := float64(p.TotalCorrect+p.StreakBonus) / float64(p.TotalAttempts)
	switch {
	case p.TotalAttempts >= 10 && acc >= 0.85:
		return "Mastered"
	case p.TotalAttempts >= 10 && acc >= 0.70:
		return "Practicing"
	case p.TotalAttempts >= 3 && acc >= 0.50:
		return "Learning"
	default:
		return "Struggling"
	}
}

// hmmToSM2 converts an HMMProgress into SM2Progress for use with sm2 functions.
func hmmToSM2(p models.HMMProgress) models.SM2Progress {
	return models.SM2Progress{
		WordID:          0, // unused by sm2 algorithm functions
		Repetitions:     p.Repetitions,
		Easiness:        p.Easiness,
		IntervalDays:    p.IntervalDays,
		DueDate:         p.DueDate,
		TotalCorrect:    p.TotalCorrect,
		TotalAttempts:   p.TotalAttempts,
		StreakBonus:     p.StreakBonus,
		LearningNewWord: p.Learning,
	}
}

// sm2ToHMM converts an SM2Progress back to HMMProgress.
func sm2ToHMM(src models.SM2Progress, userID int64, entityType, entityKey string) models.HMMProgress {
	return models.HMMProgress{
		UserID:        userID,
		EntityType:    entityType,
		EntityKey:     entityKey,
		Repetitions:   src.Repetitions,
		Easiness:      src.Easiness,
		IntervalDays:  src.IntervalDays,
		DueDate:       src.DueDate,
		TotalCorrect:  src.TotalCorrect,
		TotalAttempts: src.TotalAttempts,
		StreakBonus:   src.StreakBonus,
		Learning:      src.LearningNewWord,
	}
}

func (h *HMMQuizHandler) Answer(w http.ResponseWriter, r *http.Request) {
	var req models.HMMAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Answer = strings.TrimSpace(req.Answer)

	switch req.EntityType {
	case models.HMMEntityActor, models.HMMEntityLocation, models.HMMEntityToneRoom, models.HMMEntityProp:
	default:
		writeError(w, http.StatusBadRequest, "invalid entity_type")
		return
	}
	if req.EntityKey == "" {
		writeError(w, http.StatusBadRequest, "entity_key is required")
		return
	}

	// Look up the correct name from the library.
	var correctName string
	switch req.EntityType {
	case models.HMMEntityActor:
		actor, err := h.Store.GetHMMActorByInitial(r.Context(), UserIDFromContext(r.Context()), req.EntityKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if actor == nil {
			writeError(w, http.StatusNotFound, "actor not found")
			return
		}
		correctName = actor.ActorName
	case models.HMMEntityLocation:
		loc, err := h.Store.GetHMMLocationByFinal(r.Context(), UserIDFromContext(r.Context()), req.EntityKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if loc == nil {
			writeError(w, http.StatusNotFound, "location not found")
			return
		}
		correctName = loc.LocationName
	case models.HMMEntityToneRoom:
		tone, err := strconv.Atoi(req.EntityKey)
		if err != nil {
			writeError(w, http.StatusBadRequest, "entity_key must be a tone number for tone_room")
			return
		}
		room, err := h.Store.GetHMMToneRoom(r.Context(), UserIDFromContext(r.Context()), tone)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if room == nil {
			writeError(w, http.StatusNotFound, "tone room not found")
			return
		}
		correctName = room.RoomName
	case models.HMMEntityProp:
		props, err := h.Store.GetHMMPropsByRadicals(r.Context(), UserIDFromContext(r.Context()), []string{req.EntityKey})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(props) == 0 {
			writeError(w, http.StatusNotFound, "prop not found")
			return
		}
		correctName = props[0].PropName
	}

	normalizeHMM := func(s string) string {
		return strings.TrimSpace(hmmReParens.ReplaceAllString(s, " "))
	}
	correct := strings.EqualFold(normalizeHMM(req.Answer), normalizeHMM(correctName))

	progress, err := h.Store.GetHMMProgress(r.Context(), UserIDFromContext(r.Context()), req.EntityType, req.EntityKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if progress == nil {
		writeError(w, http.StatusNotFound, "progress not found")
		return
	}
	prevTier := hmmTier(*progress)

	quality := sm2.QualityWrong
	if correct {
		quality = sm2.QualityCorrect
	}

	sm2prog := hmmToSM2(*progress)
	var updatedSM2 models.SM2Progress
	var graduated bool
	if progress.Learning {
		updatedSM2, graduated = sm2.UpdateLearning(sm2prog, quality)
		if !graduated {
			updatedSM2.TotalAttempts++
			if correct {
				updatedSM2.TotalCorrect++
			}
		}
	} else {
		updatedSM2 = sm2.Update(sm2prog, quality)
		updatedSM2.TotalAttempts++
		if correct {
			updatedSM2.TotalCorrect++
		}
	}
	updatedSM2.StreakBonus = sm2.CalcStreakBonus(updatedSM2.StreakBonus, updatedSM2.Repetitions, updatedSM2.TotalCorrect, updatedSM2.TotalAttempts)

	updated := sm2ToHMM(updatedSM2, progress.UserID, req.EntityType, req.EntityKey)

	if err := h.Store.UpdateHMMProgress(r.Context(), updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "progress not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := models.HMMAnswerResponse{
		Correct:       correct,
		CorrectAnswer: correctName,
		NextDue:       updated.DueDate,
		IntervalDays:  updated.IntervalDays,
		TotalCorrect:  updated.TotalCorrect,
		TotalAttempts: updated.TotalAttempts,
		StreakBonus:   updated.StreakBonus,
		Repetitions:   updated.Repetitions,
		Learning:      updated.Learning,
		Graduated:     graduated,
	}
	if !correct {
		resp.YourAnswer = req.Answer
	}
	if correct {
		resp.Tier = hmmTier(updated)
		if prevTier != "" && prevTier != resp.Tier {
			resp.PrevTier = prevTier
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// Skip moves an HMM entity's due date forward by the requested number of days
// (default 7) without recording an attempt.
func (h *HMMQuizHandler) Skip(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EntityType string `json:"entity_type"`
		EntityKey  string `json:"entity_key"`
		Days       int    `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	switch req.EntityType {
	case models.HMMEntityActor, models.HMMEntityLocation, models.HMMEntityToneRoom, models.HMMEntityProp:
	default:
		writeError(w, http.StatusBadRequest, "invalid entity_type")
		return
	}
	if req.EntityKey == "" {
		writeError(w, http.StatusBadRequest, "entity_key is required")
		return
	}
	if req.Days <= 0 {
		req.Days = 7
	}
	if err := h.Store.SkipHMM(r.Context(), UserIDFromContext(r.Context()), req.EntityType, req.EntityKey, req.Days); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "progress not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

