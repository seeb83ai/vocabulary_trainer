package handlers

import (
	"net/http"
)

type MismatchesHandler struct {
	Store mismatchesStore
}

func (h *MismatchesHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.Store.GetConfusions(r.Context(), UserIDFromContext(r.Context()))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
