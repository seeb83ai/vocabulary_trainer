package sm2

import (
	"testing"
)

func TestNumberedToToneMark(t *testing.T) {
	tests := []struct {
		syllable string
		tone     int
		want     string
	}{
		{"ba", 1, "bā"},
		{"ba", 2, "bá"},
		{"ba", 3, "bǎ"},
		{"ba", 4, "bà"},
		{"a", 1, "ā"},
		{"e", 4, "è"},
		{"lv", 3, "lǚ"},
		{"ou", 2, "óu"},
		{"ai", 4, "ài"},
		{"gui", 4, "guì"},
		{"dui", 1, "duī"},
		{"zhuang", 1, "zhuāng"},
		{"xian", 2, "xián"},
		{"lei", 3, "lěi"},
		// Invalid tone
		{"ba", 0, "ba"},
		{"ba", 5, "ba"},
	}
	for _, tt := range tests {
		got := NumberedToToneMark(tt.syllable, tt.tone)
		if got != tt.want {
			t.Errorf("NumberedToToneMark(%q, %d) = %q, want %q", tt.syllable, tt.tone, got, tt.want)
		}
	}
}

func TestFormatPinyinDisplay(t *testing.T) {
	got := FormatPinyinDisplay("ba", 1)
	want := "bā (ba1)"
	if got != want {
		t.Errorf("FormatPinyinDisplay(\"ba\", 1) = %q, want %q", got, want)
	}

	got = FormatPinyinDisplay("zhi", 4)
	want = "zhì (zhi4)"
	if got != want {
		t.Errorf("FormatPinyinDisplay(\"zhi\", 4) = %q, want %q", got, want)
	}
}

func TestParsePinyinAnswer(t *testing.T) {
	tests := []struct {
		input       string
		wantSyl     string
		wantTone    int
		wantErr     bool
	}{
		{"ba1", "ba", 1, false},
		{"BA1", "ba", 1, false},
		{"  ba 1  ", "ba", 1, false},
		{"zhuang4", "zhuang", 4, false},
		{"a2", "a", 2, false},
		// Errors
		{"ba", "", 0, true},   // no tone
		{"ba5", "", 0, true},  // invalid tone
		{"b", "", 0, true},    // too short
		{"", "", 0, true},     // empty
	}
	for _, tt := range tests {
		syl, tone, err := ParsePinyinAnswer(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParsePinyinAnswer(%q) expected error, got (%q, %d)", tt.input, syl, tone)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParsePinyinAnswer(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if syl != tt.wantSyl || tone != tt.wantTone {
			t.Errorf("ParsePinyinAnswer(%q) = (%q, %d), want (%q, %d)", tt.input, syl, tone, tt.wantSyl, tt.wantTone)
		}
	}
}

func TestCheckPinyinAnswer(t *testing.T) {
	tests := []struct {
		answer  string
		target  string
		tone    int
		want    bool
	}{
		{"ba1", "ba", 1, true},
		{"BA1", "ba", 1, true},
		{"ba 1", "ba", 1, true},
		{"ba2", "ba", 1, false},
		{"pa1", "ba", 1, false},
		{"lv3", "lv", 3, true},   // v/ü equivalence
		{"lü3", "lv", 3, true},
		{"lv3", "lü", 3, true},
	}
	for _, tt := range tests {
		got := CheckPinyinAnswer(tt.answer, tt.target, tt.tone)
		if got != tt.want {
			t.Errorf("CheckPinyinAnswer(%q, %q, %d) = %v, want %v", tt.answer, tt.target, tt.tone, got, tt.want)
		}
	}
}
