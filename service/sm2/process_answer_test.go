package sm2

import (
	"testing"
	"vocabulary_trainer/models"
)

func TestProcessAnswer(t *testing.T) {
	t.Run("correct in learning phase graduates", func(t *testing.T) {
		p := models.SM2Progress{
			LearningNewWord: true,
			Repetitions:     LearningGraduateReps - 1, // one correct rep away from graduating
			TotalAttempts:   2,
			TotalCorrect:    2,
			Easiness:        2.5,
		}
		got := ProcessAnswer(p, true)
		if got.LearningNewWord {
			t.Errorf("expected graduation to clear LearningNewWord")
		}
		// On graduation UpdateLearning resets the counters to 3/3; the attempt
		// increment is skipped.
		if got.TotalAttempts != 3 || got.TotalCorrect != 3 {
			t.Errorf("graduation counters = %d/%d, want 3/3", got.TotalCorrect, got.TotalAttempts)
		}
		if got.Repetitions != 0 {
			t.Errorf("graduation Repetitions = %d, want 0", got.Repetitions)
		}
	})

	t.Run("wrong in learning phase resets streak, increments attempt", func(t *testing.T) {
		p := models.SM2Progress{
			LearningNewWord: true,
			Repetitions:     2,
			TotalAttempts:   5,
			TotalCorrect:    3,
			Easiness:        2.5,
		}
		got := ProcessAnswer(p, false)
		if !got.LearningNewWord {
			t.Errorf("wrong answer should keep word in learning phase")
		}
		if got.Repetitions != 0 {
			t.Errorf("Repetitions = %d, want 0 (reset)", got.Repetitions)
		}
		if got.TotalAttempts != 6 {
			t.Errorf("TotalAttempts = %d, want 6 (incremented)", got.TotalAttempts)
		}
		if got.TotalCorrect != 3 {
			t.Errorf("TotalCorrect = %d, want 3 (unchanged)", got.TotalCorrect)
		}
	})

	t.Run("correct in regular mode advances", func(t *testing.T) {
		p := models.SM2Progress{
			LearningNewWord: false,
			Repetitions:     1,
			IntervalDays:    6,
			TotalAttempts:   10,
			TotalCorrect:    8,
			Easiness:        2.5,
		}
		got := ProcessAnswer(p, true)
		if got.LearningNewWord {
			t.Errorf("regular word must not enter learning phase")
		}
		if got.TotalAttempts != 11 || got.TotalCorrect != 9 {
			t.Errorf("counters = %d/%d, want 9/11", got.TotalCorrect, got.TotalAttempts)
		}
		if got.Repetitions != 2 {
			t.Errorf("Repetitions = %d, want 2", got.Repetitions)
		}
	})

	t.Run("wrong in regular mode resets reps, increments attempt only", func(t *testing.T) {
		p := models.SM2Progress{
			LearningNewWord: false,
			Repetitions:     3,
			IntervalDays:    10,
			TotalAttempts:   10,
			TotalCorrect:    8,
			Easiness:        2.5,
		}
		got := ProcessAnswer(p, false)
		if got.TotalAttempts != 11 {
			t.Errorf("TotalAttempts = %d, want 11", got.TotalAttempts)
		}
		if got.TotalCorrect != 8 {
			t.Errorf("TotalCorrect = %d, want 8 (unchanged)", got.TotalCorrect)
		}
		if got.Repetitions != 0 {
			t.Errorf("Repetitions = %d, want 0 (reset)", got.Repetitions)
		}
	})
}
