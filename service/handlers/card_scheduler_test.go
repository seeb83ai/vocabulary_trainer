package handlers

import (
	"testing"
	"time"
)

func cand(kind cardKind, present bool, due time.Time) CardCandidate {
	return CardCandidate{Kind: kind, Present: present, DueDate: due}
}

func TestSelectNextCard_SingleSource(t *testing.T) {
	t.Parallel()
	now := time.Now()
	got, ok := SelectNextCard([]CardCandidate{
		cand(cardWord, false, time.Time{}),
		cand(cardHMM, true, now),
		cand(cardComponent, false, time.Time{}),
	})
	if !ok || got.Kind != cardHMM {
		t.Fatalf("want the only present candidate (hmm), got kind=%d ok=%v", got.Kind, ok)
	}
}

func TestSelectNextCard_EarliestDueWins(t *testing.T) {
	t.Parallel()
	base := time.Now()
	got, ok := SelectNextCard([]CardCandidate{
		cand(cardWord, true, base.Add(10*time.Minute)),
		cand(cardHMM, true, base.Add(1*time.Minute)),
		cand(cardComponent, true, base.Add(5*time.Minute)),
	})
	if !ok || got.Kind != cardHMM {
		t.Fatalf("want earliest-due (hmm), got kind=%d ok=%v", got.Kind, ok)
	}
}

func TestSelectNextCard_TieBreakIsCandidateOrder(t *testing.T) {
	t.Parallel()
	now := time.Now()
	// All due at the same instant: the earliest in slice order wins. The quiz
	// passes [word, hmm, component], so word wins an exact tie.
	got, ok := SelectNextCard([]CardCandidate{
		cand(cardWord, true, now),
		cand(cardHMM, true, now),
		cand(cardComponent, true, now),
	})
	if !ok || got.Kind != cardWord {
		t.Fatalf("tie should go to the earlier candidate (word), got kind=%d", got.Kind)
	}
	// Without a word candidate, hmm beats component on a tie.
	got, ok = SelectNextCard([]CardCandidate{
		cand(cardHMM, true, now),
		cand(cardComponent, true, now),
	})
	if !ok || got.Kind != cardHMM {
		t.Fatalf("tie should go to hmm over component, got kind=%d", got.Kind)
	}
}

func TestSelectNextCard_AllAbsent(t *testing.T) {
	t.Parallel()
	_, ok := SelectNextCard([]CardCandidate{
		cand(cardWord, false, time.Time{}),
		cand(cardHMM, false, time.Time{}),
	})
	if ok {
		t.Fatalf("want ok=false when no candidate is present")
	}
	if _, ok := SelectNextCard(nil); ok {
		t.Fatalf("want ok=false for an empty candidate list")
	}
}
