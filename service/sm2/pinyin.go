package sm2

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// toneMarks maps vowel runes to their tone-marked variants (tones 1-4).
var toneMarks = map[rune][4]rune{
	'a': {'ā', 'á', 'ǎ', 'à'},
	'e': {'ē', 'é', 'ě', 'è'},
	'i': {'ī', 'í', 'ǐ', 'ì'},
	'o': {'ō', 'ó', 'ǒ', 'ò'},
	'u': {'ū', 'ú', 'ǔ', 'ù'},
	'ü': {'ǖ', 'ǘ', 'ǚ', 'ǜ'},
}

// NumberedToToneMark converts a syllable and tone number to tone-marked pinyin.
// e.g. ("ba", 1) → "bā", ("lv", 3) → "lǚ", ("a", 2) → "á"
func NumberedToToneMark(syllable string, tone int) string {
	if tone == 5 {
		// Neutral tone — no tone mark, just normalize ü
		s := strings.ToLower(syllable)
		return strings.ReplaceAll(s, "v", "ü")
	}
	if tone < 1 || tone > 4 {
		return syllable
	}
	s := strings.ToLower(syllable)
	// Replace "v" with "ü" for display
	s = strings.ReplaceAll(s, "v", "ü")

	// Find which vowel gets the tone mark using standard pinyin rules:
	// 1. If there is an 'a' or 'e', it takes the mark.
	// 2. If there is 'ou', the 'o' takes the mark.
	// 3. Otherwise the second vowel takes the mark.
	runes := []rune(s)
	idx := findToneVowel(runes)
	if idx < 0 {
		return s
	}
	vowel := runes[idx]
	marks, ok := toneMarks[vowel]
	if !ok {
		return s
	}
	runes[idx] = marks[tone-1]
	return string(runes)
}

// findToneVowel returns the index of the rune that should receive the tone mark.
func findToneVowel(runes []rune) int {
	vowels := "aeioüu"
	// Rule 1: 'a' or 'e' always gets the mark
	for i, r := range runes {
		if r == 'a' || r == 'e' {
			return i
		}
	}
	// Rule 2: 'ou' → mark goes on 'o'
	for i, r := range runes {
		if r == 'o' && i+1 < len(runes) && runes[i+1] == 'u' {
			return i
		}
	}
	// Rule 3: last vowel gets the mark
	lastIdx := -1
	for i, r := range runes {
		if strings.ContainsRune(vowels, r) {
			lastIdx = i
		}
	}
	return lastIdx
}

// FormatPinyinDisplay returns a display string like "bā (ba1)".
func FormatPinyinDisplay(syllable string, tone int) string {
	marked := NumberedToToneMark(syllable, tone)
	return fmt.Sprintf("%s (%s%d)", marked, strings.ToLower(syllable), tone)
}

// ParsePinyinAnswer parses user input like "ba1" into (syllable, tone).
// Accepts: "ba1", "BA1", "ba 1", "  ba1  ".
func ParsePinyinAnswer(answer string) (string, int, error) {
	s := strings.ToLower(strings.TrimSpace(answer))
	// Remove spaces between syllable and tone number
	s = strings.ReplaceAll(s, " ", "")
	if len(s) < 2 {
		return "", 0, fmt.Errorf("answer too short")
	}
	last := s[len(s)-1]
	if last < '1' || last > '5' {
		return "", 0, fmt.Errorf("last character must be a tone number 1-5")
	}
	tone := int(last - '0')
	syllable := s[:len(s)-1]
	if !utf8.ValidString(syllable) || syllable == "" {
		return "", 0, fmt.Errorf("invalid syllable")
	}
	return syllable, tone, nil
}

// CheckPinyinAnswer checks if the user's typed answer matches the target sound.
func CheckPinyinAnswer(answer string, targetSyllable string, targetTone int) bool {
	syllable, tone, err := ParsePinyinAnswer(answer)
	if err != nil {
		return false
	}
	// Normalize: treat "v" as "ü" and vice versa
	syllable = strings.ReplaceAll(syllable, "v", "ü")
	target := strings.ReplaceAll(strings.ToLower(targetSyllable), "v", "ü")
	return syllable == target && tone == targetTone
}

// toneless maps every tone-marked vowel back to its plain vowel.
var toneless = func() map[rune]rune {
	m := map[rune]rune{}
	for base, marked := range toneMarks {
		for _, r := range marked {
			m[r] = base
		}
	}
	return m
}()

// plainPinyin lowercases s, drops parenthesised parts and tone marks, and
// keeps only letters, so "Xi'an (Eig)" and "xī ān" both become "xian".
func plainPinyin(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(stripParens(s)) {
		if base, ok := toneless[r]; ok {
			r = base
		}
		if r == 'v' {
			r = 'ü'
		}
		if r == 'ü' || (r >= 'a' && r <= 'z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TranslationEqualsPinyin reports whether a translation is only the word's
// pinyin, ignoring case, tones, spaces and parenthesised parts — e.g. the
// surname gloss "Nan (Eig, Fam)" for 南 nán (issues #464/#481).
func TranslationEqualsPinyin(text, pinyin string) bool {
	p := plainPinyin(pinyin)
	return p != "" && plainPinyin(text) == p
}

// MatchGameTranslations returns the translations usable as a match-game
// answer: a bracket-only gloss such as "(Partikel, …)" is always dropped
// (issue #482), a gloss that is only the pinyin is dropped unless nothing
// else is left in that language (#464). Languages left empty are omitted.
func MatchGameTranslations(translations map[string][]string, pinyin string) map[string][]string {
	out := map[string][]string{}
	for lang, texts := range translations {
		var real, pinyinOnly []string
		for _, t := range texts {
			switch {
			case stripParens(t) == "":
			case TranslationEqualsPinyin(t, pinyin):
				pinyinOnly = append(pinyinOnly, t)
			default:
				real = append(real, t)
			}
		}
		if len(real) == 0 {
			real = pinyinOnly
		}
		if len(real) > 0 {
			out[lang] = real
		}
	}
	return out
}
