package handlers

import (
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

func TestPinyinCoversText(t *testing.T) {
	tests := []struct {
		pinyin string
		zhText string
		want   bool
	}{
		{"guò", "过（动词）", false},            // 3 Han chars, 1 syllable
		{"guò dòng cí", "过（动词）", true},     // 3 Han chars, 3 syllables
		{"guò dòng cí lei", "过（动词）", true}, // more syllables than needed is fine
		{"", "过", false},
		{"", "", true},
		{"anything", "hello", true}, // no Han chars — trivially covered
	}
	for _, tt := range tests {
		if got := pinyinCoversText(tt.pinyin, tt.zhText); got != tt.want {
			t.Errorf("pinyinCoversText(%q, %q) = %v, want %v", tt.pinyin, tt.zhText, got, tt.want)
		}
	}
}

func TestFullPinyinForDisplay(t *testing.T) {
	stale := "guò"
	got := fullPinyinForDisplay("过（动词）", &stale)
	if got == nil || *got != "guò dòng cí" {
		t.Errorf("want regenerated full-text pinyin, got %v", got)
	}

	complete := "dei3 dong4 ci2"
	got = fullPinyinForDisplay("得（动词）", &complete)
	if got != &complete {
		t.Error("want the already-complete stored pinyin returned unchanged (same pointer)")
	}

	got = fullPinyinForDisplay("你好", nil)
	if got == nil || *got != "nǐ hǎo" {
		t.Errorf("want generated pinyin when stored is nil, got %v", got)
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
			strings.NewReader(`{"zh_text":"","source_text":""}`))
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
