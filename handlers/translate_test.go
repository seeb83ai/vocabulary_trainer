package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToPinyin(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"你好", "nǐ hǎo"},
		{"中文", "zhōng wén"},
		{"", ""},
		{"hello", ""},
	}
	for _, tt := range tests {
		got := toPinyin(tt.input)
		if got != tt.want {
			t.Errorf("toPinyin(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestConfigEndpoint(t *testing.T) {
	tests := []struct {
		enabled bool
		want    bool
	}{
		{true, true},
		{false, false},
	}
	for _, tt := range tests {
		handler := Config(tt.enabled)
		req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
		rec := httptest.NewRecorder()
		handler(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Config(%v): status = %d, want 200", tt.enabled, rec.Code)
		}
		var resp map[string]bool
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("Config(%v): decode error: %v", tt.enabled, err)
		}
		if resp["deepl_enabled"] != tt.want {
			t.Errorf("Config(%v): deepl_enabled = %v, want %v", tt.enabled, resp["deepl_enabled"], tt.want)
		}
	}
}

func TestSplitTranslations(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello / hi / greetings", []string{"hello", "hi", "greetings"}},
		{"hello", []string{"hello"}},
		{"", []string{""}},
		{"  hello  /  hi  ", []string{"hello", "hi"}},
		{"on/off", []string{"on/off"}},
		{"hello /  / hi", []string{"hello", "hi"}},
		{" / ", []string{}},
	}
	for _, tt := range tests {
		got := splitTranslations(tt.input)
		// Special case: empty-ish input returns original text
		if tt.input == " / " {
			if len(got) != 1 || got[0] != " / " {
				t.Errorf("splitTranslations(%q) = %v, want [%q]", tt.input, got, " / ")
			}
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("splitTranslations(%q): got %d parts %v, want %d parts %v", tt.input, len(got), got, len(tt.want), tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitTranslations(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestTranslateHandler_ValidationErrors(t *testing.T) {
	h := &TranslateHandler{APIKey: "test-key", TargetLang: "DE"}

	t.Run("empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/translate",
			strings.NewReader(`{"zh_text":"","en_text":""}`))
		rec := httptest.NewRecorder()
		h.Translate(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/translate",
			strings.NewReader(`not json`))
		rec := httptest.NewRecorder()
		h.Translate(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}
