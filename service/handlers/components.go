package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
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

// Seen marks a component as introduced so it enters the regular quiz rotation.
func (h *ComponentHandler) Seen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Character string `json:"character"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Character = strings.TrimSpace(req.Character)
	if req.Character == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	if err := h.Store.MarkComponentSeen(r.Context(), UserIDFromContext(r.Context()), req.Character); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Skip moves a component's due date forward by the requested number of days
// (default 7) without recording an attempt.
func (h *ComponentHandler) Skip(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Character string `json:"character"`
		Days      int    `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Character = strings.TrimSpace(req.Character)
	if req.Character == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	if req.Days <= 0 {
		req.Days = 7
	}
	if err := h.Store.SkipComponent(r.Context(), UserIDFromContext(r.Context()), req.Character, req.Days); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "component not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// List returns a paginated list of component_progress rows for the authenticated user.
func (h *ComponentHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	q := r.URL.Query().Get("q")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 200 {
		perPage = 200
	}

	items, total, err := h.Store.GetComponentList(r.Context(), userID, q, page, perPage)
	if err != nil {
		internalError(w, err)
		return
	}
	if items == nil {
		items = []db.ComponentListItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"components": items,
		"total":      total,
		"page":       page,
		"per_page":   perPage,
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
