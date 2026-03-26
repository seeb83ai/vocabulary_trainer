package handlers

import (
	"net/http"
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
	writeJSON(w, http.StatusOK, results)
}
