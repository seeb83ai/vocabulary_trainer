package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mozillazg/go-pinyin"
)

type TranslateHandler struct {
	APIKey     string
	TargetLang string
}

type translateRequest struct {
	ZhText string `json:"zh_text"`
	EnText string `json:"en_text"`
}

type translateResponse struct {
	ZhText  string   `json:"zh_text"`
	Pinyin  string   `json:"pinyin"`
	EnText  string   `json:"en_text"`
	EnTexts []string `json:"en_texts,omitempty"`
}

func (h *TranslateHandler) Translate(w http.ResponseWriter, r *http.Request) {
	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.ZhText = strings.TrimSpace(req.ZhText)
	req.EnText = strings.TrimSpace(req.EnText)

	if req.ZhText == "" && req.EnText == "" {
		writeError(w, http.StatusBadRequest, "provide zh_text or en_text")
		return
	}

	resp := translateResponse{ZhText: req.ZhText, EnText: req.EnText}

	if req.ZhText != "" && req.EnText == "" {
		// Chinese provided → translate to target language (request multiple meanings)
		instructions := []string{
			"If this word has multiple distinct meanings in the target language, list up to 3 translations separated by ' / '. Only include genuinely different meanings, not synonyms.",
		}
		translated, err := deeplTranslate([]string{req.ZhText}, h.TargetLang, "ZH", h.APIKey, instructions)
		if err != nil {
			writeError(w, http.StatusBadGateway, "DeepL error: "+err.Error())
			return
		}
		parts := splitTranslations(translated[0])
		resp.EnText = parts[0]
		resp.EnTexts = parts
		resp.Pinyin = toPinyin(req.ZhText)
	} else if req.EnText != "" && req.ZhText == "" {
		// Target language provided → translate to Chinese
		translated, err := deeplTranslate([]string{req.EnText}, "ZH", "", h.APIKey, nil)
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

func Config(deeplEnabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"deepl_enabled": deeplEnabled})
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
