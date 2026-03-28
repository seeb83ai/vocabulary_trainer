package handlers

import "testing"

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
