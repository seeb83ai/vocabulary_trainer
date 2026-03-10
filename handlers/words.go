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

	"github.com/go-chi/chi/v5"
)

type WordsHandler struct {
	Store *db.Store
	Audio *AudioHandler // optional; nil = TTS disabled
}

func (h *WordsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}

	sortBy := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("order")
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	reviewOnly := r.URL.Query().Get("review") == "1"
	bucket := r.URL.Query().Get("bucket")
	words, total, err := h.Store.GetWords(r.Context(), q, page, perPage, sortBy, sortDir, tags, reviewOnly, bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.WordListResponse{
		Words:   words,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	})
}

func (h *WordsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req models.CreateWordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.ZhText = strings.TrimSpace(req.ZhText)
	req.Pinyin = strings.TrimSpace(req.Pinyin)
	if req.ZhText == "" {
		writeError(w, http.StatusBadRequest, "zh_text is required")
		return
	}
	var filtered []string
	for _, t := range req.EnTexts {
		if s := strings.TrimSpace(t); s != "" {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 {
		writeError(w, http.StatusBadRequest, "at least one en_texts entry is required")
		return
	}
	req.EnTexts = filtered

	id, err := h.Store.CreateWord(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Audio != nil {
		go h.Audio.generate(id, req.ZhText)
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (h *WordsHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	wd, err := h.Store.GetWordByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if wd == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}
	writeJSON(w, http.StatusOK, wd)
}

func (h *WordsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req models.UpdateWordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.ZhText = strings.TrimSpace(req.ZhText)
	req.Pinyin = strings.TrimSpace(req.Pinyin)
	if req.ZhText == "" {
		writeError(w, http.StatusBadRequest, "zh_text is required")
		return
	}
	var filtered []string
	for _, t := range req.EnTexts {
		if s := strings.TrimSpace(t); s != "" {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 {
		writeError(w, http.StatusBadRequest, "at least one en_texts entry is required")
		return
	}
	req.EnTexts = filtered

	if err := h.Store.UpdateWord(r.Context(), id, req); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Audio != nil {
		go h.Audio.regenerate(id, req.ZhText)
	}
	wd, err := h.Store.GetWordByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, wd)
}

func (h *WordsHandler) AddTranslation(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		EnText string `json:"en_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.EnText = strings.TrimSpace(body.EnText)
	if body.EnText == "" {
		writeError(w, http.StatusBadRequest, "en_text is required")
		return
	}
	if err := h.Store.AddTranslation(r.Context(), id, body.EnText); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WordsHandler) MarkReview(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Store.MarkWordForReview(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WordsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Store.DeleteWord(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WordsHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.Store.GetAllTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

// Shared helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}
