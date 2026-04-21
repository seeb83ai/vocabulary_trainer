package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

type ComponentHandler struct {
	Store *db.Store
}

// Answer processes a component quiz answer.
func (h *ComponentHandler) Answer(w http.ResponseWriter, r *http.Request) {
	var req models.ComponentAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Character = strings.TrimSpace(req.Character)
	req.Answer = strings.TrimSpace(req.Answer)
	if req.Character == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}

	langs := req.Langs
	if len(langs) == 0 {
		langs = []string{"en"}
	}

	defs, err := h.Store.GetComponentDefinitions(r.Context(), req.Character, langs)
	if err != nil {
		internalError(w, err)
		return
	}
	if len(defs) == 0 {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}

	correct := false
	for _, def := range defs {
		if sm2.CheckComponentAnswer(req.Answer, def) {
			correct = true
			break
		}
	}

	userID := UserIDFromContext(r.Context())
	progress, nextDue, err := h.Store.RecordComponentAnswer(r.Context(), userID, req.Character, correct)
	if err != nil {
		internalError(w, err)
		return
	}

	if err := h.Store.RecordComponentStat(r.Context(), userID, correct); err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.ComponentAnswerResponse{
		Correct:        correct,
		CorrectAnswers: defs,
		NextDue:        nextDue,
		IntervalDays:   progress.IntervalDays,
		TotalCorrect:   progress.TotalCorrect,
		TotalAttempts:  progress.TotalAttempts,
		Repetitions:    progress.Repetitions,
	})
}

// Stats returns daily component stat history.
func (h *ComponentHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Store.GetComponentStatsHistory(r.Context(), UserIDFromContext(r.Context()))
	if err != nil {
		internalError(w, err)
		return
	}
	if stats == nil {
		stats = []models.ComponentDailyStat{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": stats})
}
