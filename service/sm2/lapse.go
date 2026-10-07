package sm2

import (
	"time"
	"vocabulary_trainer/models"
)

// RecordReview updates the lapse counters for a main-quiz answer. Only the
// first answer of the day on a word that is due today and has left the New
// bucket counts as a review: a wrong one is a lapse, a correct one ends the
// run of consecutive lapses. Days are UTC, like date('now') in SQLite.
// It returns the updated progress and whether the answer counted.
func RecordReview(p models.SM2Progress, correct bool, now time.Time) (models.SM2Progress, bool) {
	startOfToday := now.UTC().Truncate(24 * time.Hour)
	startOfTomorrow := startOfToday.Add(24 * time.Hour)
	if p.LearningNewWord || !p.DueDate.Before(startOfTomorrow) ||
		(!p.LastAttemptAt.IsZero() && !p.LastAttemptAt.Before(startOfToday)) {
		return p, false
	}
	if correct {
		p.ConsecutiveLapses = 0
	} else {
		p.Lapses++
		p.ConsecutiveLapses++
	}
	return p, true
}
