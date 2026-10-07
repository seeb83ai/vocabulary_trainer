package sm2

import (
	"testing"
	"time"
	"vocabulary_trainer/models"
)

func TestRecordReview(t *testing.T) {
	now := time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC)
	yesterday := now.Add(-24 * time.Hour)
	reviewWord := func() models.SM2Progress {
		return models.SM2Progress{
			DueDate:           now.Add(-time.Hour),
			LastAttemptAt:     yesterday,
			Lapses:            2,
			ConsecutiveLapses: 1,
		}
	}

	tests := []struct {
		name            string
		p               func() models.SM2Progress
		correct         bool
		wantLapses      int
		wantConsecutive int
		wantCounted     bool
	}{
		{"wrong first answer of the day counts a lapse", reviewWord, false, 3, 2, true},
		{"correct first answer of the day clears the run", reviewWord, true, 2, 0, true},
		{"due later today still counts", func() models.SM2Progress {
			p := reviewWord()
			p.DueDate = time.Date(2026, 10, 7, 23, 0, 0, 0, time.UTC)
			return p
		}, false, 3, 2, true},
		{"never answered before counts", func() models.SM2Progress {
			p := reviewWord()
			p.LastAttemptAt = time.Time{}
			return p
		}, false, 3, 2, true},
		{"second answer of the day does not count", func() models.SM2Progress {
			p := reviewWord()
			p.LastAttemptAt = now.Add(-time.Minute)
			return p
		}, false, 2, 1, false},
		{"word in the New bucket does not count", func() models.SM2Progress {
			p := reviewWord()
			p.LearningNewWord = true
			return p
		}, false, 2, 1, false},
		{"word not due today does not count", func() models.SM2Progress {
			p := reviewWord()
			p.DueDate = time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
			return p
		}, false, 2, 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, counted := RecordReview(tc.p(), tc.correct, now)
			if counted != tc.wantCounted {
				t.Errorf("counted: want %v, got %v", tc.wantCounted, counted)
			}
			if got.Lapses != tc.wantLapses {
				t.Errorf("lapses: want %d, got %d", tc.wantLapses, got.Lapses)
			}
			if got.ConsecutiveLapses != tc.wantConsecutive {
				t.Errorf("consecutive lapses: want %d, got %d", tc.wantConsecutive, got.ConsecutiveLapses)
			}
		})
	}
}
