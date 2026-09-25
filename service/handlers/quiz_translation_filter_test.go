package handlers

import (
	"database/sql"
	"reflect"
	"testing"
	"vocabulary_trainer/models"
)

func rankedCand(text string, rank int64) translationCandidate {
	return translationCandidate{Text: text, Source: "cedict", Rank: sql.NullInt64{Int64: rank, Valid: true}}
}

func unrankedCand(text string) translationCandidate {
	return translationCandidate{Text: text, Source: "cedict"}
}

// dictCand is a ranked dictionary gloss at sense/item of its word's
// dictionary entry (issue #474).
func dictCand(text string, rank int64, sense, item int) translationCandidate {
	c := rankedCand(text, rank)
	c.DictPos = &models.DictPosition{Sense: sense, Item: item}
	return c
}

func userCand(text string) translationCandidate {
	return translationCandidate{Text: text, Source: "user", Rank: sql.NullInt64{Int64: 0, Valid: true}}
}

func TestFilterTranslationsForDisplay(t *testing.T) {
	tests := []struct {
		name         string
		candidates   []translationCandidate
		maxShown     int
		hideUnranked bool
		userLast     bool
		wantShown    []string
		wantExtra    []string
	}{
		{
			name:       "fewer than cap shows all, no extra",
			candidates: []translationCandidate{rankedCand("a", 5), rankedCand("b", 50)},
			maxShown:   3,
			wantShown:  []string{"a", "b"},
		},
		{
			name:       "caps to rarest-last top N, rest goes to extra",
			candidates: []translationCandidate{rankedCand("rare", 9000), rankedCand("common", 5), rankedCand("mid", 500)},
			maxShown:   2,
			wantShown:  []string{"common", "mid"},
			wantExtra:  []string{"rare"},
		},
		{
			// User translations are prioritized into the visible set, but still
			// count against the cap — the rest is collapsed into extra rather
			// than dropped or shown unconditionally (issue #431/#432/#433).
			name:       "user translations prioritized but still count against cap",
			candidates: []translationCandidate{userCand("manual1"), userCand("manual2"), rankedCand("common", 5), rankedCand("rare", 9000)},
			maxShown:   1,
			wantShown:  []string{"manual1"},
			wantExtra:  []string{"manual2", "common", "rare"},
		},
		{
			name:         "unranked prioritized but still counts against cap",
			candidates:   []translationCandidate{unrankedCand("mystery"), rankedCand("common", 5), rankedCand("rare", 9000)},
			maxShown:     1,
			hideUnranked: false,
			wantShown:    []string{"mystery"},
			wantExtra:    []string{"common", "rare"},
		},
		{
			name:         "unranked treated as lowest priority when hideUnranked=true",
			candidates:   []translationCandidate{unrankedCand("mystery"), rankedCand("common", 5), rankedCand("rare", 9000)},
			maxShown:     2,
			hideUnranked: true,
			wantShown:    []string{"common", "rare"},
			wantExtra:    []string{"mystery"},
		},
		{
			// Within the prioritized tier, a short entry should be preferred over
			// a long one even though the long one comes first in input order
			// (issue #450).
			name:       "prioritized tier prefers short entry over long entry ahead of it",
			candidates: []translationCandidate{userCand("this is a very long translation phrase"), userCand("short")},
			maxShown:   1,
			wantShown:  []string{"short"},
			wantExtra:  []string{"this is a very long translation phrase"},
		},
		{
			// Within the ranked tier, a short entry should be preferred over a
			// long one even though the long one has a numerically better
			// (rarer-safe) frequency rank (issue #450).
			name:       "ranked tier prefers short entry over long entry with better rank",
			candidates: []translationCandidate{rankedCand("a very long translation phrase here", 5), rankedCand("short", 500)},
			maxShown:   1,
			wantShown:  []string{"short"},
			wantExtra:  []string{"a very long translation phrase here"},
		},
		{
			name:       "user first puts user translations ahead of unranked ones listed before them",
			candidates: []translationCandidate{unrankedCand("mystery"), userCand("mine"), rankedCand("common", 5)},
			maxShown:   3,
			wantShown:  []string{"mine", "mystery", "common"},
		},
		{
			name:       "user last orders ranked, then unranked, then user",
			candidates: []translationCandidate{userCand("mine"), unrankedCand("mystery"), rankedCand("rare", 9000), rankedCand("common", 5)},
			maxShown:   2,
			userLast:   true,
			wantShown:  []string{"common", "rare"},
			wantExtra:  []string{"mystery", "mine"},
		},
		{
			name:         "user last with hideUnranked keeps unranked after ranked and before user",
			candidates:   []translationCandidate{userCand("mine"), unrankedCand("mystery"), rankedCand("common", 5)},
			maxShown:     3,
			hideUnranked: true,
			userLast:     true,
			wantShown:    []string{"common", "mystery", "mine"},
		},
		{
			name:       "user last still prefers short user entries over long ones",
			candidates: []translationCandidate{userCand("this is a very long translation phrase"), userCand("short"), rankedCand("common", 5)},
			maxShown:   3,
			userLast:   true,
			wantShown:  []string{"common", "short", "this is a very long translation phrase"},
		},
		{
			// Issue #474: dictionary glosses follow the dictionary — the first
			// item of every sense, then the second items, and so on — not the
			// word frequency, which put function words like "to" first.
			name: "dictionary glosses ordered by item, then sense, ignoring rank",
			candidates: []translationCandidate{
				dictCand("to", 3, 2, 0), dictCand("at", 20, 2, 1),
				dictCand("pair", 900, 3, 0), dictCand("couple (S)", 800, 3, 1),
				dictCand("opposite", 1500, 0, 0), dictCand("facing (P)", 2000, 0, 1),
				dictCand("correct", 700, 1, 0), dictCand("right (Adj)", 100, 1, 1),
			},
			maxShown:  4,
			wantShown: []string{"opposite", "correct", "to", "pair"},
			wantExtra: []string{"facing (P)", "right (Adj)", "at", "couple (S)"},
		},
		{
			name:       "dictionary glosses come before glosses with no dictionary position",
			candidates: []translationCandidate{unrankedCand("mystery"), rankedCand("common", 5), dictCand("second", 900, 1, 0), dictCand("first", 800, 0, 0)},
			maxShown:   3,
			wantShown:  []string{"first", "second", "mystery"},
			wantExtra:  []string{"common"},
		},
		{
			name:       "user translations still come first, dictionary glosses next",
			candidates: []translationCandidate{dictCand("first", 800, 0, 0), userCand("mine")},
			maxShown:   2,
			wantShown:  []string{"mine", "first"},
		},
		{
			name:       "user last puts dictionary glosses first and user translations last",
			candidates: []translationCandidate{userCand("mine"), rankedCand("common", 5), dictCand("first", 800, 0, 0)},
			maxShown:   3,
			userLast:   true,
			wantShown:  []string{"first", "common", "mine"},
		},
		{
			name: "example sentences go after the other dictionary glosses",
			candidates: []translationCandidate{
				dictCand("Bsp.: 对 -- an", 50, 0, 1), dictCand("gegenüber", 700, 0, 0), dictCand("korrekt", 900, 1, 0),
			},
			maxShown:  2,
			wantShown: []string{"gegenüber", "korrekt"},
			wantExtra: []string{"Bsp.: 对 -- an"},
		},
		{
			name: "dictionary tier still prefers short glosses (issue #450)",
			candidates: []translationCandidate{
				dictCand("a very long first gloss here", 700, 0, 0), dictCand("short", 900, 1, 0),
			},
			maxShown:  1,
			wantShown: []string{"short"},
			wantExtra: []string{"a very long first gloss here"},
		},
		{
			name: "hideUnranked moves unranked dictionary glosses to the end",
			candidates: []translationCandidate{
				func() translationCandidate {
					c := unrankedCand("unranked-first")
					c.DictPos = &models.DictPosition{Sense: 0, Item: 0}
					return c
				}(),
				dictCand("second", 900, 1, 0),
			},
			maxShown:     1,
			hideUnranked: true,
			wantShown:    []string{"second"},
			wantExtra:    []string{"unranked-first"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotShown, gotExtra := filterTranslationsForDisplay(tt.candidates, tt.maxShown, tt.hideUnranked, tt.userLast)
			if !reflect.DeepEqual(gotShown, tt.wantShown) {
				t.Errorf("shown: got %v, want %v", gotShown, tt.wantShown)
			}
			if !reflect.DeepEqual(gotExtra, tt.wantExtra) {
				t.Errorf("extra: got %v, want %v", gotExtra, tt.wantExtra)
			}
		})
	}
}
