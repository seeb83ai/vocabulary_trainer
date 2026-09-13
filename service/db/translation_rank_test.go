package db

import (
	"context"
	"testing"
)

func TestComputeTranslationRank(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO word_frequency_lang (word, lang, rank) VALUES
		 ('hello', 'en', 5), ('world', 'en', 50), ('rare', 'en', 7000)
		 ON CONFLICT(word, lang) DO UPDATE SET rank = excluded.rank`,
	); err != nil {
		t.Fatalf("seed word_frequency_lang: %v", err)
	}

	tests := []struct {
		name      string
		lang      string
		text      string
		wantValid bool
		wantRank  int64
	}{
		{"single common word", "en", "hello", true, 5},
		{"phrase uses rarest word's rank", "en", "hello world", true, 50},
		{"very rare word dominates", "en", "hello rare", true, 7000},
		{"unmatched words ignored when others match", "en", "hello xyzzy", true, 5},
		{"fully unmatched phrase is unranked", "en", "xyzzy plugh", false, 0},
		{"case-insensitive lookup", "en", "HELLO", true, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := computeTranslationRank(ctx, tx, tt.lang, tt.text)
			if err != nil {
				t.Fatalf("computeTranslationRank: %v", err)
			}
			if got.Valid != tt.wantValid {
				t.Fatalf("Valid = %v, want %v", got.Valid, tt.wantValid)
			}
			if tt.wantValid && got.Int64 != tt.wantRank {
				t.Errorf("rank = %d, want %d", got.Int64, tt.wantRank)
			}
		})
	}
}
