package handlers

import (
	"encoding/json"
	"net/http"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

// SettingsHandler serves GET/PATCH /api/settings and PUT /api/settings/api-keys.
type SettingsHandler struct {
	store  *db.Store
	secret []byte
}

// NewSettingsHandler creates a SettingsHandler.
func NewSettingsHandler(store *db.Store, secret []byte) *SettingsHandler {
	return &SettingsHandler{store: store, secret: secret}
}

// Get handles GET /api/settings.
func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	st, salt, deeplEnc, llmEnc, err := h.store.GetUserSettingsRaw(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.populateMaskedKeys(st, salt, deeplEnc, llmEnc, r)
	writeJSON(w, http.StatusOK, st)
}

// Patch handles PATCH /api/settings — updates language prefs and quiz mode settings.
func (h *SettingsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PrimaryLang        string `json:"primary_lang"`
		SecondaryLang      string `json:"secondary_lang"`
		ProgNew            string `json:"prog_new"`
		ProgTierStruggling string `json:"prog_tier_struggling"`
		ProgTierLearning   string `json:"prog_tier_learning"`
		ProgTierPracticing string `json:"prog_tier_practicing"`
		ProgTierMastered   string `json:"prog_tier_mastered"`
		NewWordMode0       string `json:"new_word_mode_0"`
		NewWordMode1       string `json:"new_word_mode_1"`
		NewWordMode2       string `json:"new_word_mode_2"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.PrimaryLang == "" || req.SecondaryLang == "" {
		writeError(w, http.StatusBadRequest, "primary_lang and secondary_lang are required")
		return
	}
	if req.PrimaryLang == req.SecondaryLang {
		writeError(w, http.StatusBadRequest, "primary_lang and secondary_lang must differ")
		return
	}

	modeFields := []string{
		req.ProgNew, req.ProgTierStruggling, req.ProgTierLearning,
		req.ProgTierPracticing, req.ProgTierMastered,
		req.NewWordMode0, req.NewWordMode1, req.NewWordMode2,
	}
	for _, m := range modeFields {
		if !isValidQuizMode(m) {
			writeError(w, http.StatusBadRequest, "invalid quiz mode: "+m)
			return
		}
	}

	userID := UserIDFromContext(r.Context())
	st := models.UserSettings{
		PrimaryLang:        req.PrimaryLang,
		SecondaryLang:      req.SecondaryLang,
		ProgNew:            req.ProgNew,
		ProgTierStruggling: req.ProgTierStruggling,
		ProgTierLearning:   req.ProgTierLearning,
		ProgTierPracticing: req.ProgTierPracticing,
		ProgTierMastered:   req.ProgTierMastered,
		NewWordMode0:       req.NewWordMode0,
		NewWordMode1:       req.NewWordMode1,
		NewWordMode2:       req.NewWordMode2,
	}
	if err := h.store.UpdateUserSettings(r.Context(), userID, st); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// PutAPIKeys handles PUT /api/settings/api-keys — encrypts and stores API keys.
func (h *SettingsHandler) PutAPIKeys(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeeplKey    string `json:"deepl_key"`
		LLMProvider string `json:"llm_provider"`
		LLMKey      string `json:"llm_key"`
		LLMLocalURL string `json:"llm_local_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	c, err := r.Cookie(settingsKeyCookie)
	if err != nil {
		writeError(w, http.StatusBadRequest, "settings key not available — please log out and log in again")
		return
	}
	derivedKey, err := OpenSettingsKey(h.secret, c.Value)
	if err != nil {
		writeError(w, http.StatusBadRequest, "settings key invalid — please log out and log in again")
		return
	}

	deeplEnc, err := EncryptAPIKey(derivedKey, req.DeeplKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	llmEnc, err := EncryptAPIKey(derivedKey, req.LLMKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	userID := UserIDFromContext(r.Context())
	if err := h.store.UpdateUserAPIKeys(r.Context(), userID, deeplEnc, req.LLMProvider, llmEnc, req.LLMLocalURL); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Return updated settings with masked keys.
	st, salt, newDeeplEnc, newLLMEnc, err := h.store.GetUserSettingsRaw(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.populateMaskedKeys(st, salt, newDeeplEnc, newLLMEnc, r)
	writeJSON(w, http.StatusOK, st)
}

// UserAPIKeys decrypts and returns the user's plain-text API keys.
// Returns empty strings if no key is stored or the settings cookie is missing.
func (h *SettingsHandler) UserAPIKeys(r *http.Request, userID int64) (deeplKey, llmProvider, llmKey, llmLocalURL string) {
	c, err := r.Cookie(settingsKeyCookie)
	if err != nil {
		return
	}
	derivedKey, err := OpenSettingsKey(h.secret, c.Value)
	if err != nil {
		return
	}
	_, _, deeplEnc, llmEnc, err := h.store.GetUserSettingsRaw(r.Context(), userID)
	if err != nil {
		return
	}
	st, err := h.store.GetUserSettings(r.Context(), userID)
	if err != nil {
		return
	}
	deeplKey, _ = DecryptAPIKey(derivedKey, deeplEnc)
	llmKey, _ = DecryptAPIKey(derivedKey, llmEnc)
	llmProvider = st.LLMProvider
	llmLocalURL = st.LLMLocalURL
	return
}

func (h *SettingsHandler) populateMaskedKeys(st *models.UserSettings, salt, deeplEnc, llmEnc string, r *http.Request) {
	if deeplEnc != "" {
		st.DeeplKeySet = true
		if c, err := r.Cookie(settingsKeyCookie); err == nil {
			if dk, err := OpenSettingsKey(h.secret, c.Value); err == nil {
				if pt, err := DecryptAPIKey(dk, deeplEnc); err == nil {
					st.DeeplKeyMasked = MaskKey(pt)
				}
			}
		}
	}
	if llmEnc != "" {
		st.LLMKeySet = true
		if c, err := r.Cookie(settingsKeyCookie); err == nil {
			if dk, err := OpenSettingsKey(h.secret, c.Value); err == nil {
				if pt, err := DecryptAPIKey(dk, llmEnc); err == nil {
					st.LLMKeyMasked = MaskKey(pt)
				}
			}
		}
	}
}

var validQuizModes = map[string]bool{
	models.ModeTranslToZh:       true,
	models.ModeZhToTransl:       true,
	models.ModeZhPinyinToTransl: true,
	models.ModeMaskPinyin:       true,
	"random":                    true,
}

func isValidQuizMode(m string) bool {
	return validQuizModes[m]
}
