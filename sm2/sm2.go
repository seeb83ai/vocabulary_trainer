package sm2

import (
	"math"
	"math/rand"
	"regexp"
	"strings"
	"time"
	"vocabulary_trainer/models"
)

// reParens matches a parenthesized segment and any surrounding whitespace.
var reParens = regexp.MustCompile(`\s*\([^)]*\)\s*`)

// reTrailingPunct matches any trailing punctuation (Unicode \p{P} and \p{S}) and whitespace.
var reTrailingPunct = regexp.MustCompile(`[\p{P}\p{S}\s]+$`)

const (
	QualityCorrect  = 4
	QualityWrong    = 0
	WrongRetryDelay = 3 * time.Minute
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
	p.DueDate = time.Now().UTC().Add(time.Duration(intervalDays) * 24 * time.Hour)
	return p
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

// normalize lowercases, trims whitespace, and strips all trailing punctuation and whitespace.
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = reTrailingPunct.ReplaceAllString(s, "")
	return s
}

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

	// Form with parens stripped
	noParens := strings.TrimSpace(reParens.ReplaceAllString(a, " "))
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

// SelectMode randomly picks one of the three quiz modes with equal probability.
func SelectMode() string {
	modes := []string{
		models.ModeEnToZh,
		models.ModeZhToEn,
		models.ModeZhPinyinToEn,
	}
	return modes[rand.Intn(len(modes))]
}
