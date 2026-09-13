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
		want         []string
	}{
		{
			name:       "fewer than cap shows all",
			candidates: []translationCandidate{rankedCand("a", 5), rankedCand("b", 50)},
			maxShown:   3,
			want:       []string{"a", "b"},
		},
		{
			name:       "caps to rarest-last top N",
			candidates: []translationCandidate{rankedCand("rare", 9000), rankedCand("common", 5), rankedCand("mid", 500)},
			maxShown:   2,
			want:       []string{"common", "mid"},
		},
		{
			name:       "user translations always shown, not counted against cap",
			candidates: []translationCandidate{userCand("manual1"), userCand("manual2"), rankedCand("common", 5), rankedCand("rare", 9000)},
			maxShown:   1,
			want:       []string{"manual1", "manual2", "common"},
		},
		{
			name:         "unranked shown by default (hideUnranked=false) regardless of cap",
			candidates:   []translationCandidate{unrankedCand("mystery"), rankedCand("common", 5), rankedCand("rare", 9000)},
			maxShown:     1,
			hideUnranked: false,
			want:         []string{"mystery", "common"},
		},
		{
			name:         "unranked treated as lowest priority when hideUnranked=true",
			candidates:   []translationCandidate{unrankedCand("mystery"), rankedCand("common", 5), rankedCand("rare", 9000)},
			maxShown:     2,
			hideUnranked: true,
			want:         []string{"common", "rare"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterTranslationsForDisplay(tt.candidates, tt.maxShown, tt.hideUnranked)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
