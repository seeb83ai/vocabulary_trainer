package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

// TagsHandler handles tag metadata (description, importable flag) for the current user.
type TagsHandler struct {
	Store *db.Store
}

// Details returns all tags for the current user with their description and importable flag.
func (h *TagsHandler) Details(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	tags, err := h.Store.GetTagDetails(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load tag details")
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

// Update saves description and importable flag for a single tag owned by the current user.
func (h *TagsHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "tag name is required")
		return
	}
	if utf8.RuneCountInString(name) > 50 {
		writeError(w, http.StatusBadRequest, "tag name too long (max 50 chars)")
		return
	}

	var req models.UpsertTagMetaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if utf8.RuneCountInString(req.Description) > 200 {
		writeError(w, http.StatusBadRequest, "description too long (max 200 chars)")
		return
	}

	userID := UserIDFromContext(r.Context())
	if err := h.Store.UpsertTagMeta(r.Context(), userID, name, req.Description, req.Importable); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update tag")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
