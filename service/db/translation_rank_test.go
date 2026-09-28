package db

import (
	"context"
	"database/sql"
	"testing"
	"vocabulary_trainer/models"
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

func seedGlossRankFixture(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO word_frequency_lang (word, lang, rank) VALUES
		 ('hello', 'en', 5), ('world', 'en', 50), ('rare', 'en', 7000), ('hallo', 'de', 9)
		 ON CONFLICT(word, lang) DO UPDATE SET rank = excluded.rank`); err != nil {
		t.Fatalf("seed word_frequency_lang: %v", err)
	}
	for _, e := range []struct{ zh, lang, def string }{
		{"你好", "en", "hello; hello world"},
		{"稀", "en", "rare; xyzzy plugh"},
		{"你好", "de", "hallo, guten Tag (Gruß, formell)"},
	} {
		if err := s.SeedCedictEntryForTest(ctx, e.zh, e.lang, "", e.def); err != nil {
			t.Fatalf("seed cedict entry: %v", err)
		}
	}
}

func TestRebuildGlossRank_StoresRankPerSense(t *testing.T) {
	s := openTestDB(t)
	seedGlossRankFixture(t, s)
	ctx := context.Background()

	if err := RebuildGlossRank(s.db); err != nil {
		t.Fatalf("RebuildGlossRank: %v", err)
	}

	tests := []struct{ lang, gloss string }{
		{"en", "hello"},
		{"en", "hello world"},
		{"en", "rare"},
		{"en", "xyzzy plugh"},
		{"de", "hallo"},
		{"de", "guten Tag (Gruß, formell)"},
	}
	for _, tt := range tests {
		t.Run(tt.lang+"/"+tt.gloss, func(t *testing.T) {
			var got sql.NullInt64
			if err := s.db.QueryRow(
				`SELECT rank FROM gloss_rank WHERE lang = ? AND gloss = ?`, tt.lang, tt.gloss).Scan(&got); err != nil {
				t.Fatalf("select gloss_rank: %v", err)
			}
			tx, err := s.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatalf("begin tx: %v", err)
			}
			defer tx.Rollback()
			want, err := computeTranslationRank(ctx, tx, tt.lang, tt.gloss)
			if err != nil {
				t.Fatalf("computeTranslationRank: %v", err)
			}
			if got != want {
				t.Errorf("gloss_rank = %+v, computeTranslationRank = %+v", got, want)
			}
		})
	}
}

func linkRank(t *testing.T, s *Store, zhID int64, text string) sql.NullInt64 {
	t.Helper()
	var rank sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT t.rank FROM translations t JOIN words w ON w.id = t.translation_word_id
		 WHERE t.zh_word_id = ? AND w.text = ?`, zhID, text).Scan(&rank); err != nil {
		t.Fatalf("select translation rank %q: %v", text, err)
	}
	return rank
}

func TestLinkTranslation_UsesGlossRankCache(t *testing.T) {
	s := openTestDB(t)
	seedGlossRankFixture(t, s)
	if _, err := s.db.Exec(`INSERT INTO gloss_rank (lang, gloss, rank) VALUES ('en', 'hello', 4242)
		ON CONFLICT(lang, gloss) DO UPDATE SET rank = excluded.rank`); err != nil {
		t.Fatalf("plant gloss_rank: %v", err)
	}

	id, err := s.CreateWord(context.Background(), 2, models.CreateWordRequest{
		ZhText:             "你好",
		Translations:       map[string][]string{"en": {"hello"}},
		TranslationSources: map[string][]string{"en": {"cedict"}},
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	if got := linkRank(t, s, id, "hello"); !got.Valid || got.Int64 != 4242 {
		t.Errorf("rank = %+v, want the cached 4242", got)
	}
}

func TestLinkTranslation_FallsBackWhenGlossNotCached(t *testing.T) {
	s := openTestDB(t)
	seedGlossRankFixture(t, s)
	if _, err := s.db.Exec(`DELETE FROM gloss_rank WHERE lang = 'en' AND gloss = 'hello world'`); err != nil {
		t.Fatalf("clear gloss_rank: %v", err)
	}

	id, err := s.CreateWord(context.Background(), 2, models.CreateWordRequest{
		ZhText:             "你好",
		Translations:       map[string][]string{"en": {"hello world"}},
		TranslationSources: map[string][]string{"en": {"cedict"}},
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	if got := linkRank(t, s, id, "hello world"); !got.Valid || got.Int64 != 50 {
		t.Errorf("rank = %+v, want computed 50", got)
	}
}

func TestLinkTranslation_CachedNullRankStaysUnranked(t *testing.T) {
	s := openTestDB(t)
	seedGlossRankFixture(t, s)
	if _, err := s.db.Exec(`INSERT INTO gloss_rank (lang, gloss, rank) VALUES ('en', 'hello', NULL)
		ON CONFLICT(lang, gloss) DO UPDATE SET rank = NULL`); err != nil {
		t.Fatalf("plant gloss_rank: %v", err)
	}

	id, err := s.CreateWord(context.Background(), 2, models.CreateWordRequest{
		ZhText:             "你好",
		Translations:       map[string][]string{"en": {"hello"}},
		TranslationSources: map[string][]string{"en": {"cedict"}},
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	if got := linkRank(t, s, id, "hello"); got.Valid {
		t.Errorf("rank = %+v, want NULL (cached as unranked)", got)
	}
}
