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

	"github.com/go-chi/chi/v5"
)

// GetComponentHMMContext returns actor/location/room/props/scene context for a
// component character. Same JSON shape as GET /api/words/{id}/hmm/context so
// that loadCompHMMBuilder in hmm-builder.js can share the rendering code.
func (h *ComponentHandler) GetComponentHMMContext(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	if char == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}

	userID := UserIDFromContext(r.Context())
	ctx := r.Context()

	pinyin := h.Store.GetComponentPinyin(ctx, char)
	var initial, final string
	var tone int
	if pinyin != "" {
		initial, final, tone = parsePinyin(pinyin)
	}

	// Decompose the single character into radicals.
	runes := []rune(char)
	var radicals []string
	radicalDefs := map[string]string{}
	var decompositionStr string
	decomps, _ := h.Store.GetHanziDecomposition(ctx, runes)
	if len(decomps) > 0 {
		radicals = collectRadicals(decomps[0])
		radicalDefs = collectRadicalDefs(decomps[0])
		decompositionStr = decomps[0].Decomposition
	}

	resp := models.HMMSceneContext{
		Initial:       initial,
		Final:         final,
		Tone:          tone,
		Pinyin:        pinyin,
		Decomposition: decompositionStr,
		Radicals:      radicals,
		RadicalDefs:   radicalDefs,
		RadicalDeDefs: map[string]string{},
	}

	if initial != "" {
		resp.Actor, _ = h.Store.GetHMMActorByInitial(ctx, userID, initial)
	}
	if final != "" {
		resp.Location, _ = h.Store.GetHMMLocationByFinal(ctx, userID, final)
	}
	if tone >= 1 && tone <= 5 {
		resp.ToneRoom, _ = h.Store.GetHMMToneRoom(ctx, userID, tone)
	}
	if len(radicals) > 0 {
		resp.Props, _ = h.Store.GetHMMPropsByRadicals(ctx, userID, radicals)
	}
	if resp.Props == nil {
		resp.Props = []models.HMMProp{}
	}
	if resp.Radicals == nil {
		resp.Radicals = []string{}
	}

	resp.Scene, _ = h.Store.GetComponentHMMSceneRecord(ctx, userID, char)

	writeJSON(w, http.StatusOK, resp)
}

// SaveCompScene saves the mnemonic scene and library entries for a component character.
// Mirrors PUT /api/words/{id}/hmm used by the word HMM builder.
func (h *ComponentHandler) SaveCompScene(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	if char == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}

	var req models.HMMSaveSceneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	ctx := r.Context()
	userID := UserIDFromContext(ctx)

	pinyin := h.Store.GetComponentPinyin(ctx, char)
	var initial, final string
	var tone int
	if pinyin != "" {
		initial, final, tone = parsePinyin(pinyin)
	}

	if err := h.Store.SaveComponentHMMSceneWithLibrary(ctx, userID, char, initial, final, tone, req); err != nil {
		internalError(w, err)
		return
	}

	if req.Decomposition != "" {
		_ = h.Store.UpsertHanziDecomposition(ctx, char, req.Decomposition)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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
		if st, _ := h.Store.GetUserSettings(r.Context(), UserIDFromContext(r.Context())); st != nil {
			langs = []string{st.PrimaryLang}
		} else {
			langs = []string{"en"}
		}
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

	sceneText, _ := h.Store.GetComponentHMMSceneText(r.Context(), userID, req.Character)
	writeJSON(w, http.StatusOK, models.ComponentAnswerResponse{
		Correct:        correct,
		CorrectAnswers: defs,
		NextDue:        nextDue,
		IntervalDays:   progress.IntervalDays,
		TotalCorrect:   progress.TotalCorrect,
		TotalAttempts:  progress.TotalAttempts,
		Repetitions:    progress.Repetitions,
		SceneText:      sceneText,
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

	reviewOnly := r.URL.Query().Get("review") == "1"
	items, total, err := h.Store.GetComponentList(r.Context(), userID, q, page, perPage, reviewOnly)
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

// Review flags a component for review by setting needs_review = 1.
func (h *ComponentHandler) Review(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	if char == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	if err := h.Store.MarkComponentForReview(UserIDFromContext(r.Context()), char); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetTranslations returns all stored translations for a component character.
func (h *ComponentHandler) GetTranslations(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	if char == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	translations, err := h.Store.GetComponentTranslations(char)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, translations)
}

// UpdateTranslation stores or updates a translation for a component character.
func (h *ComponentHandler) UpdateTranslation(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	if char == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	var req struct {
		Lang       string `json:"lang"`
		Definition string `json:"definition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Lang) == "" {
		writeError(w, http.StatusBadRequest, "lang is required")
		return
	}
	if err := h.Store.StoreComponentTranslation(char, req.Lang, req.Definition); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetHMMScene returns the saved mnemonic scene text for a component character.
func (h *ComponentHandler) GetHMMScene(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	if char == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	text, err := h.Store.GetComponentHMMSceneText(r.Context(), UserIDFromContext(r.Context()), char)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"scene_text": text})
}

// PutHMMScene saves (or replaces) a mnemonic scene text for a component character.
func (h *ComponentHandler) PutHMMScene(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	if char == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	var req struct {
		SceneText string `json:"scene_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.Store.UpsertComponentHMMScene(r.Context(), UserIDFromContext(r.Context()), char, req.SceneText); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteHMMScene removes the mnemonic scene for a component character.
func (h *ComponentHandler) DeleteHMMScene(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	if char == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	if err := h.Store.DeleteComponentHMMScene(r.Context(), UserIDFromContext(r.Context()), char); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
