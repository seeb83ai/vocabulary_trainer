package sm2

import (
	"math"
	"math/rand"
	"regexp"
	"strings"
	"time"
	"vocabulary_trainer/models"
)

// reParens matches a parenthesized segment (no nested parens) and any surrounding whitespace.
// Applied iteratively so that nested parens are stripped inside-out.
var reParens = regexp.MustCompile(`\s*\([^()]*\)\s*`)

// reTrailingPunct matches any trailing punctuation (Unicode \p{P} and \p{S}) and whitespace.
var reTrailingPunct = regexp.MustCompile(`[\p{P}\p{S}\s]+$`)

const (
	QualityCorrect       = 4
	QualityWrong         = 0
	WrongRetryDelay      = 3 * time.Minute
	LearningCorrectDelay = 2 * time.Minute
	LearningGraduateReps = 3
)

// Update applies the SM-2 algorithm and returns an updated SM2Progress.
func Update(p models.SM2Progress, quality int) models.SM2Progress {
	// Update easiness factor
	ef := p.Easiness + (0.1 - float64(5-quality)*(0.08+float64(5-quality)*0.02))
	if ef < 1.3 {
		ef = 1.3
	}

	var repetitions int
	var intervalDays int

	if quality < 3 {
		repetitions = 0
		intervalDays = 0
		p.Easiness = ef
		p.Repetitions = repetitions
		p.IntervalDays = intervalDays
		p.DueDate = time.Now().UTC().Add(WrongRetryDelay + time.Duration(rand.Int63n(int64(WrongRetryDelay*2))))
		return p
	} else {
		switch p.Repetitions {
		case 0:
			intervalDays = 1
		case 1:
			intervalDays = 6
		default:
			intervalDays = int(math.Round(float64(p.IntervalDays) * ef))
		}
		repetitions = p.Repetitions + 1
	}

	p.Easiness = ef
	p.Repetitions = repetitions
	p.IntervalDays = intervalDays
	jitter := time.Duration(rand.Int63n(int64(2*time.Hour))) - 2*time.Hour
	p.DueDate = time.Now().UTC().Add(time.Duration(intervalDays)*24*time.Hour + jitter)
	return p
}

// UpdateLearning applies a simplified update for words still in the learning phase.
// Uses short intervals (minutes) so all 3 correct answers can happen in one session.
// Returns the updated progress and whether the word has graduated (repetitions >= 3).
func UpdateLearning(p models.SM2Progress, quality int) (models.SM2Progress, bool) {
	if quality < 3 {
		// Wrong answer: reset streak
		p.Repetitions = 0
		p.DueDate = time.Now().UTC().Add(WrongRetryDelay + time.Duration(rand.Int63n(int64(WrongRetryDelay*2))))
		return p, false
	}

	p.Repetitions++
	jitter := time.Duration(rand.Int63n(int64(LearningCorrectDelay)))
	p.DueDate = time.Now().UTC().Add(LearningCorrectDelay + jitter)

	if p.Repetitions >= LearningGraduateReps {
		// Graduate: reset SM-2 state for a clean start
		p.LearningNewWord = false
		p.Repetitions = 0
		p.Easiness = 2.5
		p.IntervalDays = 1
		p.TotalCorrect = 3
		p.TotalAttempts = 3
		p.DueDate = time.Now().UTC().Add(24*time.Hour + time.Duration(rand.Int63n(int64(2*time.Hour))))
		return p, true
	}

	return p, false
}

// CheckAnswer returns true if the user's answer matches any accepted answer
// (case-insensitive, whitespace-trimmed).
//
// Two normalisation rules apply to each accepted answer before comparing:
//  1. Parenthesized segments are optional: "(das Gehörte) nicht verstehen"
//     also accepts "nicht verstehen".
//  2. Slash-separated alternatives are each valid on their own:
//     "Essen / Gericht" also accepts "Essen" or "Gericht".
//
// All combinations of the two rules are tried.
func CheckAnswer(userAnswer string, accepted []string) bool {
	ua := normalize(userAnswer)
	for _, a := range accepted {
		for _, variant := range expandVariants(a) {
			if variant == ua {
				return true
			}
		}
	}
	return false
}

// NormalizeAnswer lowercases, trims whitespace, and strips all trailing punctuation and whitespace.
func NormalizeAnswer(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = reTrailingPunct.ReplaceAllString(s, "")
	return s
}

// normalize is the package-internal alias used by expandVariants/CheckAnswer.
func normalize(s string) string { return NormalizeAnswer(s) }

// expandVariants returns all valid answer strings derived from a single
// accepted answer by applying the optional-parens and slash-split rules.
func expandVariants(a string) []string {
	seen := map[string]struct{}{}
	add := func(s string) {
		s = normalize(s)
		if s != "" {
			seen[s] = struct{}{}
		}
	}

	// Full form (with parens, with slashes)
	add(a)

	// Form with parens stripped (iterate until stable to handle nested parens)
	stripped := a
	for {
		next := strings.TrimSpace(reParens.ReplaceAllString(stripped, " "))
		if next == strings.TrimSpace(stripped) {
			break
		}
		stripped = next
	}
	noParens := stripped
	add(noParens)

	// Slash-split variants of both the original and the paren-stripped form
	for _, base := range []string{a, noParens} {
		for _, part := range strings.Split(base, "/") {
			add(part)
			// Also strip parens from each slash part
			add(strings.TrimSpace(reParens.ReplaceAllString(part, " ")))
		}
	}

	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	return out
}

// MaskPinyin returns a masked pinyin hint for learning-phase en_to_zh cards.
// The masking level depends on totalCorrect:
//
//	0 → first char of each syllable visible, rest replaced with * per char ("nǐ hǎo" → "n** h**")
//	1 → first char of full pinyin visible + * per remaining char            ("nǐ hǎo" → "n*****")
//	2+ → empty string (no hint)
func MaskPinyin(pinyin string, totalCorrect int) string {
	if pinyin == "" || totalCorrect >= 2 {
		return ""
	}
	runes := []rune(pinyin)
	if totalCorrect == 1 {
		var b strings.Builder
		b.WriteRune(runes[0])
		for _, r := range runes[1:] {
			if r == ' ' {
				b.WriteRune(' ')
			} else {
				b.WriteRune('*')
			}
		}
		return b.String()
	}
	// totalCorrect == 0: mask each space-separated syllable
	words := strings.Split(pinyin, " ")
	for i, w := range words {
		wr := []rune(w)
		if len(wr) == 0 {
			continue
		}
		var b strings.Builder
		b.WriteRune(wr[0])
		for range wr[1:] {
			b.WriteRune('*')
		}
		words[i] = b.String()
	}
	return strings.Join(words, " ")
}

// SelectProgressiveMode picks a quiz mode based on the word's accuracy (correct/attempts).
// This implements the progressive training ladder:
//   - attempts < 3                          → en_to_zh (not enough data)
//   - accuracy < 50%                        → en_to_zh (still struggling)
//   - accuracy < 70% or attempts < 10       → zh_pinyin_to_en (progressing; pinyin scaffold)
//   - accuracy < 85%                        → zh_to_en (reliable; drop pinyin)
//   - accuracy ≥ 85% and attempts ≥ 10      → random (mastered)
func SelectProgressiveMode(totalCorrect, totalAttempts int) string {
	if totalAttempts < 3 {
		return models.ModeEnToZh
	}
	accuracy := float64(totalCorrect) / float64(totalAttempts)
	switch {
	case accuracy < 0.50:
		return models.ModeEnToZh
	case accuracy < 0.70 || totalAttempts < 10:
		return models.ModeZhPinyinToEn
	case accuracy < 0.85:
		return models.ModeZhToEn
	default:
		return SelectMode()
	}
}

// SelectMode randomly picks one of the three quiz modes with equal probability.
func SelectMode() string {
	modes := []string{
		models.ModeEnToZh,
		models.ModeZhToEn,
		models.ModeZhPinyinToEn,
	}
	return modes[rand.Intn(len(modes))]
}
