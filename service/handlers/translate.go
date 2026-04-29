package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mozillazg/go-pinyin"
	"vocabulary_trainer/db"
)

type TranslateHandler struct {
	APIKey          string
	TargetLang      string
	Store           *db.Store
	SettingsHandler *SettingsHandler // may be nil when auth is disabled
}

type translateRequest struct {
	ZhText     string `json:"zh_text"`
	SourceText string `json:"source_text"`
	TargetLang string `json:"target_lang"`
}

type translateResponse struct {
	ZhText       string   `json:"zh_text"`
	Pinyin       string   `json:"pinyin"`
	SourceText   string   `json:"source_text"`
	Translations []string `json:"translations,omitempty"`
}

func (h *TranslateHandler) Translate(w http.ResponseWriter, r *http.Request) {
	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.ZhText = strings.TrimSpace(req.ZhText)
	req.SourceText = strings.TrimSpace(req.SourceText)

	if req.ZhText == "" && req.SourceText == "" {
		writeError(w, http.StatusBadRequest, "provide zh_text or source_text")
		return
	}

	// Pinyin-only path (both zh and source_text provided) is available to all users.
	// DeepL translation requires plus/admin role OR a personal user key.
	pinyinOnly := req.ZhText != "" && req.SourceText != ""
	if !pinyinOnly {
		hasUserKey := false
		if h.SettingsHandler != nil {
			dk, _, _, _ := h.SettingsHandler.UserAPIKeys(r, UserIDFromContext(r.Context()))
			hasUserKey = dk != ""
		}
		if !hasUserKey {
			role, err := h.Store.GetUserRole(r.Context(), UserIDFromContext(r.Context()))
			if err != nil || (role != "plus" && role != "admin") {
				writeError(w, http.StatusForbidden, "feature requires plus account or a personal DeepL key")
				return
			}
		}
	}

	resp := translateResponse{ZhText: req.ZhText, SourceText: req.SourceText}

	targetLang := h.TargetLang
	if req.TargetLang != "" {
		targetLang = strings.ToUpper(req.TargetLang)
	}

	// Resolve the API key: user-specific key takes precedence over server env key.
	apiKey := h.APIKey
	if h.SettingsHandler != nil {
		if userKey, _, _, _ := h.SettingsHandler.UserAPIKeys(r, UserIDFromContext(r.Context())); userKey != "" {
			apiKey = userKey
		}
	}
	if apiKey == "" {
		writeError(w, http.StatusServiceUnavailable, "DeepL not configured")
		return
	}

	if req.ZhText != "" && req.SourceText == "" {
		// Chinese provided → translate to target language (request multiple meanings)
		instructions := []string{
			"If this word has multiple distinct meanings in the target language, list up to 3 translations separated by ' / '. Only include genuinely different meanings, not synonyms.",
		}
		translated, err := deeplTranslate([]string{req.ZhText}, targetLang, "ZH", apiKey, instructions)
		if err != nil {
			writeError(w, http.StatusBadGateway, "DeepL error: "+err.Error())
			return
		}
		parts := splitTranslations(translated[0])
		resp.SourceText = parts[0]
		resp.Translations = parts
		resp.Pinyin = toPinyin(req.ZhText)
	} else if req.SourceText != "" && req.ZhText == "" {
		// Source language text provided → translate to Chinese
		translated, err := deeplTranslate([]string{req.SourceText}, "ZH", "", apiKey, nil)
		if err != nil {
			writeError(w, http.StatusBadGateway, "DeepL error: "+err.Error())
			return
		}
		resp.ZhText = translated[0]
		resp.Pinyin = toPinyin(translated[0])
	} else {
		// Both provided → just generate pinyin
		resp.Pinyin = toPinyin(req.ZhText)
	}

	writeJSON(w, http.StatusOK, resp)
}

func Pinyin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ZhText string `json:"zh_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.ZhText = strings.TrimSpace(req.ZhText)
	if req.ZhText == "" {
		writeError(w, http.StatusBadRequest, "zh_text is required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"pinyin": toPinyin(req.ZhText)})
}

// Config returns feature availability for the current user.
// *_configured: whether the API key/service is set up server-side.
// *_available:  configured AND the user's role allows access (plus or admin).
// user_*_key_set: the user has a personal key stored in settings.
func (h *TranslateHandler) Config(deeplConfigured, llmConfigured bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		role, _ := h.Store.GetUserRole(r.Context(), userID)
		canUse := role == "plus" || role == "admin"

		userDeeplSet := false
		userLLMSet := false
		if h.SettingsHandler != nil {
			dk, _, lk, _ := h.SettingsHandler.UserAPIKeys(r, userID)
			userDeeplSet = dk != ""
			userLLMSet = lk != ""
		}

		writeJSON(w, http.StatusOK, map[string]bool{
			"deepl_configured":   deeplConfigured,
			"deepl_available":    (deeplConfigured || userDeeplSet) && canUse,
			"llm_configured":     llmConfigured,
			"llm_available":      (llmConfigured || userLLMSet) && canUse,
			"user_deepl_key_set": userDeeplSet,
			"user_llm_key_set":   userLLMSet,
		})
	}
}

func toPinyin(zh string) string {
	a := pinyin.NewArgs()
	a.Style = pinyin.Tone
	result := pinyin.Pinyin(zh, a)
	parts := make([]string, len(result))
	for i, p := range result {
		if len(p) > 0 {
			parts[i] = p[0]
		}
	}
	return strings.Join(parts, " ")
}

func splitTranslations(text string) []string {
	parts := strings.Split(text, " / ")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

func deeplTranslate(texts []string, targetLang, sourceLang, apiKey string, customInstructions []string) ([]string, error) {
	base := "https://api.deepl.com/v2/translate"
	if strings.HasSuffix(apiKey, ":fx") {
		base = "https://api-free.deepl.com/v2/translate"
	}

	type reqBody struct {
		Text               []string `json:"text"`
		TargetLang         string   `json:"target_lang"`
		SourceLang         string   `json:"source_lang,omitempty"`
		CustomInstructions []string `json:"custom_instructions,omitempty"`
	}
	body := reqBody{Text: texts, TargetLang: targetLang, CustomInstructions: customInstructions}
	if sourceLang != "" {
		body.SourceLang = sourceLang
	}

	reqBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, base, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DeepL returned HTTP %d: %s", resp.StatusCode, respBytes)
	}

	var result struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if len(result.Translations) != len(texts) {
		return nil, fmt.Errorf("DeepL returned %d translations for %d texts", len(result.Translations), len(texts))
	}

	out := make([]string, len(result.Translations))
	for i, t := range result.Translations {
		out[i] = t.Text
	}
	return out, nil
}
