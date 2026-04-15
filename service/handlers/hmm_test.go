package handlers

import (
	"reflect"
	"testing"

	"vocabulary_trainer/models"
)

func TestCollectRadicalDefs(t *testing.T) {
	tests := []struct {
		name  string
		input models.HanziDecomposition
		want  map[string]string
	}{
		{
			name:  "empty decomposition returns empty map",
			input: models.HanziDecomposition{Character: "日"},
			want:  map[string]string{},
		},
		{
			name: "single component with definition",
			input: models.HanziDecomposition{
				Character: "明",
				Components: []models.HanziDecomposition{
					{Character: "日", Definition: "sun, day"},
					{Character: "月", Definition: "moon, month"},
				},
			},
			want: map[string]string{"日": "sun, day", "月": "moon, month"},
		},
		{
			name: "component without definition is skipped",
			input: models.HanziDecomposition{
				Character: "明",
				Components: []models.HanziDecomposition{
					{Character: "日", Definition: "sun, day"},
					{Character: "月"},
				},
			},
			want: map[string]string{"日": "sun, day"},
		},
		{
			name: "nested components are collected",
			input: models.HanziDecomposition{
				Character: "森",
				Components: []models.HanziDecomposition{
					{
						Character:  "木",
						Definition: "tree, wood",
						Components: []models.HanziDecomposition{
							{Character: "十", Definition: "ten"},
						},
					},
				},
			},
			want: map[string]string{"木": "tree, wood", "十": "ten"},
		},
		{
			name: "first occurrence wins for duplicate characters",
			input: models.HanziDecomposition{
				Character: "森",
				Components: []models.HanziDecomposition{
					{Character: "木", Definition: "tree, wood"},
					{Character: "木", Definition: "different definition"},
				},
			},
			want: map[string]string{"木": "tree, wood"},
		},
		{
			name: "multi-rune characters are excluded",
			input: models.HanziDecomposition{
				Character: "X",
				Components: []models.HanziDecomposition{
					{Character: "ab", Definition: "multi-rune"},
					{Character: "木", Definition: "tree, wood"},
				},
			},
			want: map[string]string{"木": "tree, wood"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectRadicalDefs(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("collectRadicalDefs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePinyin(t *testing.T) {
	tests := []struct {
		input   string
		initial string
		final   string
		tone    int
	}{
		// Male initials
		{"dà", "d", "a", 4},
		{"bā", "b", "a", 1},
		{"gōng", "g", "ong", 1},
		{"shān", "sh", "an", 1},
		{"zhōng", "zh", "ong", 1},
		{"chī", "ch", "null", 1},    // ch + i → male initial "ch", final simplifies "i"→"null"
		{"rén", "r", "en", 2},
		{"sān", "s", "an", 1},

		// Female initials (consonant + i)
		{"bǐ", "bi", "null", 3},     // bi + standalone i → final "null"
		{"piān", "pi", "an", 1},      // pi + an
		{"míng", "mi", "eng", 2},     // mi + ing → eng
		{"dì", "di", "null", 4},
		{"tiān", "ti", "an", 1},
		{"niú", "ni", "ou", 2},       // ni + iu → ou
		{"liǎng", "li", "ang", 3},    // li + iang → ang

		// Fictional initials
		{"jiā", "j", "a", 1},         // j + ia → a
		{"qī", "q", "null", 1},       // q + i → null (j/q/x are not female)
		{"xiǎo", "x", "ao", 3},       // x + iao → ao

		// Wildcard initials
		{"yī", "y", "null", 1},       // y + i → null
		{"wǒ", "w", "o", 3},

		// Null initial (vowel-only)
		{"ā", "null", "a", 1},
		{"ér", "null", "er", 2},

		// Numeric tone suffix
		{"da4", "d", "a", 4},
		{"ma1", "m", "a", 1},

		// Neutral tone
		{"de", "d", "e", 5},
		{"ma", "m", "a", 5},

		// u-prefixed finals
		{"guān", "g", "an", 1},       // g + uan → an
		{"duì", "d", "ei", 4},        // d + ui → ei
		{"huǒ", "h", "o", 3},         // h + uo → o
		{"kuài", "k", "ai", 4},       // k + uai → ai

		// Multi-syllable: takes first syllable only
		{"nǐ hǎo", "ni", "null", 3},

		// Empty
		{"", "null", "null", 5},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			initial, final, tone := parsePinyin(tt.input)
			if initial != tt.initial || final != tt.final || tone != tt.tone {
				t.Errorf("parsePinyin(%q) = (%q, %q, %d), want (%q, %q, %d)",
					tt.input, initial, final, tone, tt.initial, tt.final, tt.tone)
			}
		})
	}
}
