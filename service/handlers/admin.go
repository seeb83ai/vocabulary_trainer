package handlers

import (
	"net/http"
)

// AdminHandler serves cross-user usage insights. Every route using it must
// be gated behind RequireAdmin.
type AdminHandler struct {
	Store adminStore
}

// Overview handles GET /api/admin/overview. The optional exclude_seed=1
// query param leaves the seed accounts (id 1, 2) out of every count.
func (h *AdminHandler) Overview(w http.ResponseWriter, r *http.Request) {
	excludeSeedUsers := r.URL.Query().Get("exclude_seed") == "1"
	ov, err := h.Store.GetAdminOverview(r.Context(), excludeSeedUsers)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}
