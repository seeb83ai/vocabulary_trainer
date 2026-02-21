package sm2

import (
	"math"
	"math/rand"
	"strings"
	"time"
	"vocabulary_trainer/models"
)

const (
	QualityCorrect = 4
	QualityWrong   = 0
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
		intervalDays = 1
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
func CheckAnswer(userAnswer string, accepted []string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAnswer))
	for _, a := range accepted {
		if strings.ToLower(strings.TrimSpace(a)) == ua {
			return true
		}
	}
	return false
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
