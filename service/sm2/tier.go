package sm2

import "vocabulary_trainer/models"

// Tier is the accuracy/attempt bucket a word falls into. It is the single source
// of truth for tier classification; the SQL fragments in db.tierFilter and the
// JS copy (wordTier in app.js) are derived from / pinned against these rules.
type Tier int

const (
	// TierNone means the word has never been attempted; it renders as "" (no
	// pill), matching app.js wordTier returning null for total_attempts == 0.
	TierNone Tier = iota
	TierNew
	TierStruggling
	TierLearning
	TierPracticing
	TierMastered
)

// Tier boundary thresholds. tierFilter() in db/words.go derives its SQL from
// these so the numbers are not restated in SQL strings.
const (
	TierLearningAttempts   = 3  // min attempts to leave Struggling on accuracy
	TierGraduatedAttempts  = 10 // min attempts to reach Practicing/Mastered
	TierLearningAccuracy   = 0.50
	TierPracticingAccuracy = 0.70
	TierMasteredAccuracy   = 0.85
)

// String returns the display label for the tier (matches the historical
// wordTier() strings and the JS TIERS labels). TierNone renders as "".
func (tr Tier) String() string {
	switch tr {
	case TierNew:
		return "New"
	case TierStruggling:
		return "Struggling"
	case TierLearning:
		return "Learning"
	case TierPracticing:
		return "Practicing"
	case TierMastered:
		return "Mastered"
	default:
		return ""
	}
}

// ClassifyTier returns the accuracy/attempt bucket for a progress record.
// A word with no attempts is TierNone; a word still in the learning phase is
// TierNew; otherwise it is bucketed by accuracy and attempt count.
func ClassifyTier(p models.SM2Progress) Tier {
	if p.TotalAttempts == 0 {
		return TierNone
	}
	if p.LearningNewWord {
		return TierNew
	}
	acc := float64(p.TotalCorrect+p.StreakBonus) / float64(p.TotalAttempts)
	switch {
	case p.TotalAttempts >= TierGraduatedAttempts && acc >= TierMasteredAccuracy:
		return TierMastered
	case p.TotalAttempts >= TierGraduatedAttempts && acc >= TierPracticingAccuracy:
		return TierPracticing
	case p.TotalAttempts >= TierLearningAttempts && acc >= TierLearningAccuracy:
		return TierLearning
	default:
		return TierStruggling
	}
}
