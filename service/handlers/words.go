package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
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
	hideUnseen := r.URL.Query().Get("hide_unseen") == "1"
	bucket := r.URL.Query().Get("bucket")
	dueFilter := r.URL.Query().Get("due")
	missingLang := r.URL.Query().Get("missing_lang")
	words, total, err := h.Store.GetWords(r.Context(), UserIDFromContext(r.Context()), q, page, perPage, sortBy, sortDir, tags, reviewOnly, hideUnseen, bucket, dueFilter, missingLang)
	if err != nil {
		internalError(w, err)
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
	if utf8.RuneCountInString(req.ZhText) > 200 {
		writeError(w, http.StatusBadRequest, "zh_text too long (max 200 characters)")
		return
	}
	if utf8.RuneCountInString(req.Pinyin) > 200 {
		writeError(w, http.StatusBadRequest, "pinyin too long (max 200 characters)")
		return
	}
	cleaned := map[string][]string{}
	for lang, texts := range req.Translations {
		for _, t := range texts {
			if s := strings.TrimSpace(t); s != "" {
				cleaned[lang] = append(cleaned[lang], s)
			}
		}
		if len(cleaned[lang]) > 20 {
			writeError(w, http.StatusBadRequest, "too many translations (max 20)")
			return
		}
		for _, t := range cleaned[lang] {
			if utf8.RuneCountInString(t) > 500 {
				writeError(w, http.StatusBadRequest, "translation too long (max 500 characters)")
				return
			}
		}
	}
	totalTranslations := 0
	for _, texts := range cleaned {
		totalTranslations += len(texts)
	}
	if totalTranslations == 0 {
		writeError(w, http.StatusBadRequest, "at least one translation is required")
		return
	}
	if len(req.Tags) > 20 {
		writeError(w, http.StatusBadRequest, "too many tags (max 20)")
		return
	}
	for _, tag := range req.Tags {
		if utf8.RuneCountInString(tag) > 50 {
			writeError(w, http.StatusBadRequest, "tag too long (max 50 characters)")
			return
		}
	}
	req.Translations = cleaned

	id, err := h.Store.CreateWord(r.Context(), UserIDFromContext(r.Context()), req)
	if err != nil {
		internalError(w, err)
		return
	}
	if req.StartTraining {
		if err := h.Store.AcknowledgeWord(r.Context(), UserIDFromContext(r.Context()), id); err != nil {
			internalError(w, err)
			return
		}
		initComponents(r.Context(), h.Store, UserIDFromContext(r.Context()), id, req.ZhText)
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
	wd, err := h.Store.GetWordByID(r.Context(), UserIDFromContext(r.Context()), id)
	if err != nil {
		internalError(w, err)
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
	if utf8.RuneCountInString(req.ZhText) > 200 {
		writeError(w, http.StatusBadRequest, "zh_text too long (max 200 characters)")
		return
	}
	if utf8.RuneCountInString(req.Pinyin) > 200 {
		writeError(w, http.StatusBadRequest, "pinyin too long (max 200 characters)")
		return
	}
	cleaned := map[string][]string{}
	for lang, texts := range req.Translations {
		for _, t := range texts {
			if s := strings.TrimSpace(t); s != "" {
				cleaned[lang] = append(cleaned[lang], s)
			}
		}
		if len(cleaned[lang]) > 20 {
			writeError(w, http.StatusBadRequest, "too many translations (max 20)")
			return
		}
		for _, t := range cleaned[lang] {
			if utf8.RuneCountInString(t) > 500 {
				writeError(w, http.StatusBadRequest, "translation too long (max 500 characters)")
				return
			}
		}
	}
	totalTranslations := 0
	for _, texts := range cleaned {
		totalTranslations += len(texts)
	}
	if totalTranslations == 0 {
		writeError(w, http.StatusBadRequest, "at least one translation is required")
		return
	}
	if len(req.Tags) > 20 {
		writeError(w, http.StatusBadRequest, "too many tags (max 20)")
		return
	}
	for _, tag := range req.Tags {
		if utf8.RuneCountInString(tag) > 50 {
			writeError(w, http.StatusBadRequest, "tag too long (max 50 characters)")
			return
		}
	}
	req.Translations = cleaned

	if err := h.Store.UpdateWord(r.Context(), UserIDFromContext(r.Context()), id, req); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		internalError(w, err)
		return
	}
	if req.StartTraining {
		if err := h.Store.AcknowledgeWord(r.Context(), UserIDFromContext(r.Context()), id); err != nil {
			internalError(w, err)
			return
		}
		initComponents(r.Context(), h.Store, UserIDFromContext(r.Context()), id, req.ZhText)
	}
	if h.Audio != nil {
		go h.Audio.regenerate(id, req.ZhText)
	}
	wd, err := h.Store.GetWordByID(r.Context(), UserIDFromContext(r.Context()), id)
	if err != nil {
		internalError(w, err)
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
		Text string `json:"text"`
		Lang string `json:"lang"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.Text = strings.TrimSpace(body.Text)
	if body.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if body.Lang == "" {
		if st, _ := h.Store.GetUserSettings(r.Context(), UserIDFromContext(r.Context())); st != nil {
			body.Lang = st.PrimaryLang
		} else {
			body.Lang = "en"
		}
	}
	if err := h.Store.AddTranslation(r.Context(), UserIDFromContext(r.Context()), id, body.Lang, body.Text); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		internalError(w, err)
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
	if err := h.Store.MarkWordForReview(r.Context(), UserIDFromContext(r.Context()), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		internalError(w, err)
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
	if err := h.Store.DeleteWord(r.Context(), UserIDFromContext(r.Context()), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WordsHandler) Export(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	sortBy := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("order")
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	reviewOnly := r.URL.Query().Get("review") == "1"
	hideUnseen := r.URL.Query().Get("hide_unseen") == "1"
	bucket := r.URL.Query().Get("bucket")
	dueFilter := r.URL.Query().Get("due")
	words, _, err := h.Store.GetWords(r.Context(), UserIDFromContext(r.Context()), q, 1, 0, sortBy, sortDir, tags, reviewOnly, hideUnseen, bucket, dueFilter, "")
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, words)
}

func (h *WordsHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	tags, err := h.Store.GetAllTags(r.Context(), userID)
	if err != nil {
		internalError(w, err)
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

func internalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// initComponents adds component_progress rows for a zh word after it enters
// training. Errors are non-fatal: we log them and continue.
func initComponents(ctx context.Context, s *db.Store, userID, wordID int64, zhText string) {
	p, err := s.GetSM2Progress(ctx, wordID)
	if err != nil || p == nil {
		return
	}
	if err := s.InitComponentsForWord(ctx, userID, zhText, p.DueDate); err != nil {
		log.Printf("initComponents %q: %v", zhText, err)
	}
}
