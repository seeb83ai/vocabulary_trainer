package handlers

import (
	"net/http"
	"vocabulary_trainer/db"
)

type MismatchesHandler struct {
	Store *db.Store
}

func (h *MismatchesHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.Store.GetConfusions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}
