package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

type TranslateHandler struct {
	APIKey          string
	TargetLang      string
	Store           translateStore
	SettingsHandler *SettingsHandler // may be nil when auth is disabled
	// BaseURL overrides the DeepL API endpoint. Empty uses the real DeepL
	// endpoints (selected by API key suffix); tests point it at a local
	// httptest server instead of calling out to DeepL.
	BaseURL string
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
		translated, err := deeplTranslate(h.BaseURL, []string{req.ZhText}, targetLang, "ZH", apiKey, instructions)
		if err != nil {
			log.Printf("deepl translate: %v", err) // full upstream detail stays server-side
			writeError(w, http.StatusBadGateway, "translation service unavailable")
			return
		}
		parts := splitTranslations(translated[0])
		resp.SourceText = parts[0]
		resp.Translations = parts
		resp.Pinyin = toPinyin(req.ZhText)
	} else if req.SourceText != "" && req.ZhText == "" {
		// Source language text provided → translate to Chinese
		translated, err := deeplTranslate(h.BaseURL, []string{req.SourceText}, "ZH", "", apiKey, nil)
		if err != nil {
			log.Printf("deepl translate: %v", err) // full upstream detail stays server-side
			writeError(w, http.StatusBadGateway, "translation service unavailable")
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
func (h *TranslateHandler) Config(deeplConfigured, llmConfigured, imagesConfigured bool) http.HandlerFunc {
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
			"images_configured":  imagesConfigured,
		})
	}
}

// pinyinBracket passes parenthesis characters through the pinyin conversion
// unchanged, so the generated pinyin reflects brackets marking optional text
// (issue #310). All other non-Chinese characters (e.g. Latin letters,
// whitespace) are dropped, matching the library's default behaviour.
func pinyinBracket(r rune, a pinyin.Args) []string {
	switch r {
	case '(', ')', '（', '）':
		return []string{string(r)}
	default:
		return []string{}
	}
}

func toPinyin(zh string) string {
	a := pinyin.NewArgs()
	a.Style = pinyin.Tone
	a.Fallback = pinyinBracket
	result := pinyin.Pinyin(zh, a)
	var b strings.Builder
	for _, p := range result {
		if len(p) == 0 {
			continue
		}
		tok := p[0]
		closing := tok == ")" || tok == "）"
		openSuffix := strings.HasSuffix(b.String(), "(") || strings.HasSuffix(b.String(), "（")
		if b.Len() > 0 && !closing && !openSuffix {
			b.WriteByte(' ')
		}
		b.WriteString(tok)
	}
	return b.String()
}

// pinyinCoversText reports whether pinyin has at least one space-separated
// syllable per Han character in zhText — i.e. every hanzi in the text
// (including inside a bracketed annotation) has a pinyin reading.
func pinyinCoversText(pinyin, zhText string) bool {
	hanCount := 0
	for _, r := range zhText {
		if unicode.Is(unicode.Han, r) {
			hanCount++
		}
	}
	if hanCount == 0 {
		return true
	}
	return len(strings.Fields(pinyin)) >= hanCount
}

// bracketChars strips the bracket characters toPinyin preserves for the
// auto-fill use case (issue #310) — a quiz-card reading should read as
// plain syllables, not carry the punctuation marking the optional segment.
var bracketChars = strings.NewReplacer("(", "", ")", "", "（", "", "）", "")

// fullPinyinForDisplay returns stored as-is when it already covers every Han
// character in zhText (never overwriting a hand-curated reading, e.g. a
// deliberate choice for a polyphonic character); otherwise it regenerates
// pinyin for the full text so annotations like "过（动词）" get a complete
// reading instead of a stale partial one.
func fullPinyinForDisplay(zhText string, stored *string) *string {
	if stored != nil && *stored != "" && pinyinCoversText(*stored, zhText) {
		return stored
	}
	p := toPinyin(zhText)
	if p == "" {
		return stored
	}
	p = strings.Join(strings.Fields(bracketChars.Replace(p)), " ")
	return &p
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

func deeplTranslate(baseURL string, texts []string, targetLang, sourceLang, apiKey string, customInstructions []string) ([]string, error) {
	base := "https://api.deepl.com/v2/translate"
	if strings.HasSuffix(apiKey, ":fx") {
		base = "https://api-free.deepl.com/v2/translate"
	}
	if baseURL != "" {
		base = baseURL
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
		detail := respBytes
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return nil, fmt.Errorf("DeepL returned HTTP %d: %s", resp.StatusCode, detail)
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
