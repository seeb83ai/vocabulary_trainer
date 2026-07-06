package handlers

import (
	"time"
	"vocabulary_trainer/models"
)

type cardKind int

const (
	cardWord cardKind = iota
	cardHMM
	cardComponent
)

// componentCard holds the data needed to serialise a component quiz card.
type componentCard struct {
	Character   string
	Pinyin      string
	DueDate     time.Time
	IsNew       bool
	Definitions map[string]string
}

// CardCandidate is one possible next card. Exactly one typed payload is set,
// matching Kind; DueDate is the scheduling key.
type CardCandidate struct {
	Kind         cardKind
	Present      bool
	DueDate      time.Time
	Word         *models.Word
	WordProgress *models.SM2Progress
	HMM          *models.HMMQuizCard
	Component    *componentCard
}

// SelectNextCard returns the present candidate with the earliest DueDate.
//
// Ties are broken deterministically by the candidates' order in the slice
// (earlier wins), so callers pass them in priority order. QuizHandler.Next
// passes [word, hmm, component], which reproduces the historical precedence on
// an exact due-date tie: word beats hmm and component, and hmm beats component.
//
// Returns ok=false when no candidate is present.
func SelectNextCard(candidates []CardCandidate) (CardCandidate, bool) {
	var best CardCandidate
	found := false
	for _, c := range candidates {
		if !c.Present {
			continue
		}
		if !found || c.DueDate.Before(best.DueDate) {
			best = c
			found = true
		}
	}
	return best, found
}
