package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"vocabulary_trainer/db"
	"vocabulary_trainer/llm"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// SettingsHandler serves GET/PATCH /api/settings and PUT /api/settings/api-keys.
type SettingsHandler struct {
	store  settingsStore
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
		PrimaryLang                 string `json:"primary_lang"`
		SecondaryLang               string `json:"secondary_lang"`
		ProgNew                     string `json:"prog_new"`
		ProgTierStruggling          string `json:"prog_tier_struggling"`
		ProgTierLearning            string `json:"prog_tier_learning"`
		ProgTierPracticing          string `json:"prog_tier_practicing"`
		ProgTierMastered            string `json:"prog_tier_mastered"`
		NewWordMode0                string `json:"new_word_mode_0"`
		NewWordMode1                string `json:"new_word_mode_1"`
		NewWordMode2                string `json:"new_word_mode_2"`
		CycleSequence               string `json:"cycle_sequence"`
		CycleAdvanceOnSuccessOnly   bool   `json:"cycle_advance_on_success_only"`
		NewWordRequireZh            bool   `json:"new_word_require_zh"`
		NewWordRequireTrans         bool   `json:"new_word_require_trans"`
		AcceptCorrectMode           string `json:"accept_correct_mode"`
		MaxNewWordsPerDay           *int   `json:"max_new_words_per_day"`
		NewWordCooldownMinutes      int    `json:"new_word_cooldown_minutes"`
		SkipNewWordsVisible         bool   `json:"skip_new_words_visible"`
		ExtendSessionWithExtraWords bool   `json:"extend_session_with_extra_words"`
		BaselineDueTodayEnabled     bool   `json:"baseline_due_today_enabled"`
		BaselineDueTodayValue       int    `json:"baseline_due_today_value"`
		BaselineStrugglingEnabled   bool   `json:"baseline_struggling_enabled"`
		BaselineStrugglingValue     int    `json:"baseline_struggling_value"`
		BaselineLearningEnabled     bool   `json:"baseline_learning_enabled"`
		BaselineLearningValue       int    `json:"baseline_learning_value"`
		BaselineNewBucketEnabled    bool   `json:"baseline_new_bucket_enabled"`
		BaselineNewBucketValue      int    `json:"baseline_new_bucket_value"`
		GamificationEnabled         bool   `json:"gamification_enabled"`
		GamificationFrequency       *int   `json:"gamification_frequency"`
		BlurPinyin                  bool   `json:"blur_pinyin"`
		NoAutoVoiceOnBlur           bool   `json:"no_auto_voice_on_blur"`
		CelebrateBucketChange       bool   `json:"celebrate_bucket_change"`
		VoiceUnavailable            bool   `json:"voice_unavailable"`
		SentenceBlankEnabled        bool   `json:"sentence_blank_enabled"`
		SentenceBlankRatio          int    `json:"sentence_blank_ratio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.PrimaryLang == "" {
		writeError(w, http.StatusBadRequest, "primary_lang is required")
		return
	}
	if req.SecondaryLang != "" && req.PrimaryLang == req.SecondaryLang {
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

	cycleSeq := req.CycleSequence
	if cycleSeq == "" {
		cycleSeq = sm2.DefaultCycleSequence
	} else {
		steps := strings.Split(cycleSeq, ",")
		if len(steps) < 2 {
			writeError(w, http.StatusBadRequest, "cycle_sequence must have at least 2 steps")
			return
		}
		if len(steps) > 5 {
			writeError(w, http.StatusBadRequest, "cycle_sequence must have at most 5 steps")
			return
		}
		for _, step := range steps {
			if !isValidCycleMode(strings.TrimSpace(step)) {
				writeError(w, http.StatusBadRequest, "invalid cycle mode: "+step)
				return
			}
		}
	}

	if req.AcceptCorrectMode == "" {
		req.AcceptCorrectMode = "typo"
	}
	if !isValidAcceptCorrectMode(req.AcceptCorrectMode) {
		writeError(w, http.StatusBadRequest, "invalid accept_correct_mode: must be never, typo, or always")
		return
	}

	// Resolve max_new_words_per_day: nil means field was omitted → keep stored value.
	var resolvedMaxNew int
	if req.MaxNewWordsPerDay == nil {
		if existing, err := h.store.GetUserSettings(r.Context(), UserIDFromContext(r.Context())); err == nil {
			resolvedMaxNew = existing.MaxNewWordsPerDay
		}
		if resolvedMaxNew < 1 {
			resolvedMaxNew = 5
		}
	} else {
		resolvedMaxNew = *req.MaxNewWordsPerDay
	}
	if resolvedMaxNew < 1 {
		writeError(w, http.StatusBadRequest, "max_new_words_per_day must be at least 1")
		return
	}
	if req.BaselineDueTodayValue < 0 {
		writeError(w, http.StatusBadRequest, "baseline_due_today_value must be >= 0")
		return
	}
	if req.BaselineStrugglingValue < 0 {
		writeError(w, http.StatusBadRequest, "baseline_struggling_value must be >= 0")
		return
	}
	if req.BaselineLearningValue < 0 {
		writeError(w, http.StatusBadRequest, "baseline_learning_value must be >= 0")
		return
	}
	if req.BaselineNewBucketValue < 0 {
		writeError(w, http.StatusBadRequest, "baseline_new_bucket_value must be >= 0")
		return
	}
	if req.SentenceBlankRatio < 0 || req.SentenceBlankRatio > 100 {
		writeError(w, http.StatusBadRequest, "sentence_blank_ratio must be between 0 and 100")
		return
	}

	resolvedFrequency := 5
	if req.GamificationFrequency == nil {
		if existing, err := h.store.GetUserSettings(r.Context(), UserIDFromContext(r.Context())); err == nil {
			resolvedFrequency = existing.GamificationFrequency
		}
		if resolvedFrequency < 1 {
			resolvedFrequency = 5
		}
	} else {
		resolvedFrequency = *req.GamificationFrequency
	}
	if resolvedFrequency < 1 || resolvedFrequency > 1440 {
		writeError(w, http.StatusBadRequest, "gamification_frequency must be between 1 and 1440")
		return
	}

	userID := UserIDFromContext(r.Context())
	st := models.UserSettings{
		PrimaryLang:                 req.PrimaryLang,
		SecondaryLang:               req.SecondaryLang,
		ProgNew:                     req.ProgNew,
		ProgTierStruggling:          req.ProgTierStruggling,
		ProgTierLearning:            req.ProgTierLearning,
		ProgTierPracticing:          req.ProgTierPracticing,
		ProgTierMastered:            req.ProgTierMastered,
		NewWordMode0:                req.NewWordMode0,
		NewWordMode1:                req.NewWordMode1,
		NewWordMode2:                req.NewWordMode2,
		CycleSequence:               cycleSeq,
		CycleAdvanceOnSuccessOnly:   req.CycleAdvanceOnSuccessOnly,
		NewWordRequireZh:            req.NewWordRequireZh,
		NewWordRequireTrans:         req.NewWordRequireTrans,
		AcceptCorrectMode:           req.AcceptCorrectMode,
		MaxNewWordsPerDay:           resolvedMaxNew,
		NewWordCooldownMinutes:      req.NewWordCooldownMinutes,
		SkipNewWordsVisible:         req.SkipNewWordsVisible,
		ExtendSessionWithExtraWords: req.ExtendSessionWithExtraWords,
		BaselineDueTodayEnabled:     req.BaselineDueTodayEnabled,
		BaselineDueTodayValue:       req.BaselineDueTodayValue,
		BaselineStrugglingEnabled:   req.BaselineStrugglingEnabled,
		BaselineStrugglingValue:     req.BaselineStrugglingValue,
		BaselineLearningEnabled:     req.BaselineLearningEnabled,
		BaselineLearningValue:       req.BaselineLearningValue,
		BaselineNewBucketEnabled:    req.BaselineNewBucketEnabled,
		BaselineNewBucketValue:      req.BaselineNewBucketValue,
		GamificationEnabled:         req.GamificationEnabled,
		GamificationFrequency:       resolvedFrequency,
		BlurPinyin:                  req.BlurPinyin,
		NoAutoVoiceOnBlur:           req.NoAutoVoiceOnBlur,
		CelebrateBucketChange:       req.CelebrateBucketChange,
		VoiceUnavailable:            req.VoiceUnavailable,
		SentenceBlankEnabled:        req.SentenceBlankEnabled,
		SentenceBlankRatio:          req.SentenceBlankRatio,
	}
	if err := h.store.UpdateUserSettings(r.Context(), userID, st); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

var validTrainModes = map[string]bool{
	"progressive":           true,
	"random":                true,
	"cycle":                 true,
	"transl_to_zh":          true,
	"zh_to_transl":          true,
	"zh_pinyin_to_transl":   true,
	"zh_to_transl_no_sound": true,
	"":                      true,
}

// PatchTrainingFilters handles PATCH /api/training-filters — persists the
// training page filter state (mode, tier bucket, langs, tags, etc.) per user.
func (h *SettingsHandler) PatchTrainingFilters(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode       string   `json:"mode"`
		Bucket     string   `json:"bucket"`
		Langs      []string `json:"langs"`
		Mnemonics  bool     `json:"mnemonics"`
		Components bool     `json:"components"`
		Tags       []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !validTrainModes[req.Mode] {
		writeError(w, http.StatusBadRequest, "invalid mode: "+req.Mode)
		return
	}
	if len(req.Langs) == 0 {
		req.Langs = []string{"en"}
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	userID := UserIDFromContext(r.Context())
	if err := h.store.UpdateTrainingFilters(r.Context(), userID,
		req.Mode, req.Bucket, req.Langs, req.Mnemonics, req.Components, req.Tags); err != nil {
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

	// A user-supplied local LLM URL is an outbound request target: reject
	// internal/non-public addresses to prevent SSRF before storing it.
	req.LLMLocalURL = strings.TrimSpace(req.LLMLocalURL)
	if req.LLMProvider == "local" && req.LLMLocalURL != "" {
		if err := llm.ValidateExternalURL(req.LLMLocalURL); err != nil {
			writeError(w, http.StatusBadRequest, "invalid llm_local_url: must be a public http(s) address")
			return
		}
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
	models.ModeTranslToZh:        true,
	models.ModeZhToTransl:        true,
	models.ModeZhPinyinToTransl:  true,
	models.ModeMaskPinyin:        true,
	models.ModeZhToTranslNoSound: true,
	models.ModeVoiceToTransl:     true,
	"random":                     true,
}

func isValidQuizMode(m string) bool {
	return validQuizModes[m]
}

var validCycleModes = map[string]bool{
	models.ModeTranslToZh:        true,
	models.ModeZhToTransl:        true,
	models.ModeZhPinyinToTransl:  true,
	models.ModeMaskPinyin:        true,
	models.ModeZhToTranslNoSound: true,
	models.ModeVoiceToTransl:     true,
}

func isValidCycleMode(m string) bool {
	return validCycleModes[m]
}

var validAcceptCorrectModes = map[string]bool{
	"never":  true,
	"typo":   true,
	"always": true,
}

func isValidAcceptCorrectMode(m string) bool {
	return validAcceptCorrectModes[m]
}
