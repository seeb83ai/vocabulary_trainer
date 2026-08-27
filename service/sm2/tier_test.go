package sm2

import (
	"testing"
	"vocabulary_trainer/models"
)

func prog(attempts, correct, streak int, learning bool) models.SM2Progress {
	return models.SM2Progress{
		TotalAttempts:   attempts,
		TotalCorrect:    correct,
		StreakBonus:     streak,
		LearningNewWord: learning,
	}
}

func TestClassifyTier(t *testing.T) {
	cases := []struct {
		name     string
		p        models.SM2Progress
		want     Tier
		wantText string
	}{
		{"no attempts", prog(0, 0, 0, false), TierNone, ""},
		{"no attempts even if learning flag", prog(0, 0, 0, true), TierNone, ""},
		{"learning phase with attempts", prog(5, 5, 0, true), TierNew, "New"},
		// Struggling: below the attempt or accuracy floor.
		{"few attempts", prog(2, 2, 0, false), TierStruggling, "Struggling"},
		{"3 attempts but acc < 0.50", prog(4, 1, 0, false), TierStruggling, "Struggling"},
		// Learning: >=3 attempts and acc >= 0.50 but not yet graduated.
		{"learning lower boundary acc=0.50", prog(4, 2, 0, false), TierLearning, "Learning"},
		{"learning at 3 attempts", prog(3, 2, 0, false), TierLearning, "Learning"},
		{"graduated attempts but acc 0.69 -> learning", prog(100, 69, 0, false), TierLearning, "Learning"},
		// Practicing: >=10 attempts, 0.70 <= acc < 0.85.
		{"practicing lower boundary acc=0.70", prog(10, 7, 0, false), TierPracticing, "Practicing"},
		{"practicing just below mastered acc=0.84", prog(100, 84, 0, false), TierPracticing, "Practicing"},
		// Mastered: >=10 attempts and acc >= 0.85.
		{"mastered lower boundary acc=0.85", prog(100, 85, 0, false), TierMastered, "Mastered"},
		{"mastered perfect", prog(20, 20, 0, false), TierMastered, "Mastered"},
		// 10 attempts at acc=0.50 is not yet graduated -> Learning.
		{"10 attempts acc 0.50", prog(10, 5, 0, false), TierLearning, "Learning"},
		// StreakBonus counts toward accuracy.
		{"streak bonus lifts to mastered", prog(10, 7, 2, false), TierMastered, "Mastered"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyTier(c.p)
			if got != c.want {
				t.Errorf("ClassifyTier = %v, want %v", got, c.want)
			}
			if got.String() != c.wantText {
				t.Errorf("String = %q, want %q", got.String(), c.wantText)
			}
		})
	}
}

func TestTierString(t *testing.T) {
	cases := map[Tier]string{
		TierNone:       "",
		TierNew:        "New",
		TierStruggling: "Struggling",
		TierLearning:   "Learning",
		TierPracticing: "Practicing",
		TierMastered:   "Mastered",
	}
	for tr, want := range cases {
		if tr.String() != want {
			t.Errorf("Tier(%d).String() = %q, want %q", tr, tr.String(), want)
		}
	}
}

// jsWordTier mirrors the wordTier() copy in service/frontend/app.js. This test
// pins the JS boundaries: if the JS function changes, update this mirror and the
// assertion below so the two implementations stay aligned. Classify(...).String()
// must agree with the JS labels at every boundary ("" stands in for JS null).
func jsWordTier(totalCorrect, totalAttempts, streakBonus int, learningNewWord bool) string {
	tiers := []string{"New", "Struggling", "Learning", "Practicing", "Mastered"}
	if totalAttempts == 0 {
		return "" // JS returns null
	}
	if learningNewWord {
		return tiers[0]
	}
	acc := float64(totalCorrect+streakBonus) / float64(totalAttempts)
	switch {
	case totalAttempts >= 10 && acc >= 0.85:
		return tiers[4]
	case totalAttempts >= 10 && acc >= 0.70:
		return tiers[3]
	case totalAttempts >= 3 && acc >= 0.50:
		return tiers[2]
	default:
		return tiers[1]
	}
}

func TestPinJSBoundaries(t *testing.T) {
	cases := []models.SM2Progress{
		prog(0, 0, 0, false),
		prog(5, 5, 0, true),
		prog(2, 2, 0, false),
		prog(3, 2, 0, false),
		prog(4, 1, 0, false),
		prog(10, 5, 0, false),
		prog(10, 7, 0, false),
		prog(100, 69, 0, false),
		prog(100, 84, 0, false),
		prog(100, 85, 0, false),
		prog(20, 20, 0, false),
		prog(10, 7, 2, false),
	}
	for _, p := range cases {
		goLabel := ClassifyTier(p).String()
		jsLabel := jsWordTier(p.TotalCorrect, p.TotalAttempts, p.StreakBonus, p.LearningNewWord)
		if goLabel != jsLabel {
			t.Errorf("Go/JS tier mismatch for %+v: go=%q js=%q", p, goLabel, jsLabel)
		}
	}
}

func TestTierFromBucketKey(t *testing.T) {
	cases := []struct {
		key  string
		want Tier
	}{
		{"new", TierNew},
		{"0-49", TierStruggling},
		{"50-69", TierLearning},
		{"70-84", TierPracticing},
		{"85-100", TierMastered},
		{"", TierNone},
		{"bogus", TierNone},
	}
	for _, c := range cases {
		if got := TierFromBucketKey(c.key); got != c.want {
			t.Errorf("TierFromBucketKey(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestTierFromBucketKey_RoundTripsWithBucketKey(t *testing.T) {
	for _, tr := range []Tier{TierNew, TierStruggling, TierLearning, TierPracticing, TierMastered} {
		key := tr.BucketKey()
		if got := TierFromBucketKey(key); got != tr {
			t.Errorf("TierFromBucketKey(BucketKey(%v)=%q) = %v, want %v", tr, key, got, tr)
		}
	}
}
