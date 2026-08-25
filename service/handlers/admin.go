package handlers

import (
	"net/http"
)

// AdminHandler serves cross-user usage insights. Every route using it must
// be gated behind RequireAdmin.
type AdminHandler struct {
	Store adminStore
}

// Overview handles GET /api/admin/overview.
func (h *AdminHandler) Overview(w http.ResponseWriter, r *http.Request) {
	ov, err := h.Store.GetAdminOverview(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}
