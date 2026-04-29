package handlers

import (
	"net/http"
	"strings"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

type HanziHandler struct {
	Store *db.Store
}

func (h *HanziHandler) Decompose(w http.ResponseWriter, r *http.Request) {
	chars := r.URL.Query().Get("chars")
	if chars == "" {
		writeJSON(w, http.StatusOK, []models.HanziDecomposition{})
		return
	}

	runes := []rune(chars)
	if len(runes) > 20 {
		runes = runes[:20]
	}

	results, err := h.Store.GetHanziDecomposition(r.Context(), runes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch decomposition")
		return
	}
	if results == nil {
		results = []models.HanziDecomposition{}
	}

	if langsParam := r.URL.Query().Get("langs"); langsParam != "" {
		langs := strings.Split(langsParam, ",")
		if err := h.Store.AnnotateComponentDefinitions(r.Context(), results, langs); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to annotate component definitions")
			return
		}
	}

	if r.URL.Query().Get("mark_new") == "true" {
		userID := UserIDFromContext(r.Context())
		if userID > 0 {
			if err := h.Store.AnnotateNewComponents(r.Context(), userID, results); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to annotate components")
				return
			}
		}
	}

	writeJSON(w, http.StatusOK, results)
}
