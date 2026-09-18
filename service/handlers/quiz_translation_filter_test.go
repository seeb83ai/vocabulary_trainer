package handlers

import (
	"database/sql"
	"reflect"
	"testing"
)

func rankedCand(text string, rank int64) translationCandidate {
	return translationCandidate{Text: text, Source: "cedict", Rank: sql.NullInt64{Int64: rank, Valid: true}}
}

func unrankedCand(text string) translationCandidate {
	return translationCandidate{Text: text, Source: "cedict"}
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotShown, gotExtra := filterTranslationsForDisplay(tt.candidates, tt.maxShown, tt.hideUnranked)
			if !reflect.DeepEqual(gotShown, tt.wantShown) {
				t.Errorf("shown: got %v, want %v", gotShown, tt.wantShown)
			}
			if !reflect.DeepEqual(gotExtra, tt.wantExtra) {
				t.Errorf("extra: got %v, want %v", gotExtra, tt.wantExtra)
			}
		})
	}
}
