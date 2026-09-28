package db

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"vocabulary_trainer/models"
)

// wordsSnapshot dumps everything CreateWord writes for userID as sorted
// strings, so two databases can be compared without depending on row IDs.
func wordsSnapshot(t *testing.T, s *Store, userID int64) []string {
	t.Helper()
	var out []string
	queries := map[string]string{
		"word": `SELECT text || '|' || language || '|' || COALESCE(pinyin, '') FROM words WHERE user_id = ?`,
		"link": `SELECT zh.text || '>' || tr.text || '|' || tr.language || '|' || t.source || '|' || COALESCE(t.rank, 'null')
		         FROM translations t
		         JOIN words zh ON zh.id = t.zh_word_id
		         JOIN words tr ON tr.id = t.translation_word_id
		         WHERE zh.user_id = ?`,
		"tag": `SELECT w.text || '#' || tg.name FROM word_tags wt
		        JOIN words w ON w.id = wt.word_id JOIN tags tg ON tg.id = wt.tag_id
		        WHERE w.user_id = ?`,
		"sm2": `SELECT w.text || '|' || w.language || '|' || p.repetitions || '|' || p.total_attempts
		        FROM sm2_progress p JOIN words w ON w.id = p.word_id WHERE w.user_id = ?`,
	}
	for kind, q := range queries {
		rows, err := s.db.Query(q, userID)
		if err != nil {
			t.Fatalf("snapshot %s: %v", kind, err)
		}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				rows.Close()
				t.Fatalf("scan %s: %v", kind, err)
			}
			out = append(out, kind+":"+v)
		}
		rows.Close()
	}
	sort.Strings(out)
	return out
}

func batchFixture() []models.CreateWordRequest {
	return []models.CreateWordRequest{
		{ZhText: "你好", Pinyin: "nǐ hǎo", Translations: map[string][]string{"en": {"hello", "hi"}, "de": {"hallo"}}, Tags: []string{"HSK1", "greetings"}},
		{ZhText: "您好", Pinyin: "nín hǎo", Translations: map[string][]string{"en": {"hello", "good day"}}, Tags: []string{"HSK1"}},
		{ZhText: "谢谢", Pinyin: "xiè xie", Translations: map[string][]string{"en": {"thanks"}}, TranslationSources: map[string][]string{"en": {"cedict"}}},
		{ZhText: "再见", Translations: map[string][]string{"de": {"auf Wiedersehen", " "}}},
	}
}

func TestCreateWordsBatch_MatchesRepeatedCreateWord(t *testing.T) {
	ctx := context.Background()
	single := openTestDB(t)
	batch := openTestDB(t)

	for _, req := range batchFixture() {
		if _, err := single.CreateWord(ctx, 2, req); err != nil {
			t.Fatalf("CreateWord %q: %v", req.ZhText, err)
		}
	}
	ids, err := batch.CreateWordsBatch(ctx, 2, batchFixture())
	if err != nil {
		t.Fatalf("CreateWordsBatch: %v", err)
	}
	if len(ids) != len(batchFixture()) {
		t.Fatalf("got %d ids, want %d", len(ids), len(batchFixture()))
	}

	want, got := wordsSnapshot(t, single, 2), wordsSnapshot(t, batch, 2)
	if fmt.Sprint(want) != fmt.Sprint(got) {
		t.Errorf("batch state differs from repeated CreateWord\nwant: %q\n got: %q", want, got)
	}
}

func benchWords(n int) []models.CreateWordRequest {
	reqs := make([]models.CreateWordRequest, n)
	for i := range reqs {
		reqs[i] = models.CreateWordRequest{
			ZhText: fmt.Sprintf("词%d", i),
			Pinyin: "cí",
			Translations: map[string][]string{
				"en": {fmt.Sprintf("word %d", i), "shared meaning", fmt.Sprintf("to be able to %d", i%50), "thing"},
				"de": {fmt.Sprintf("Wort %d", i), "gemeinsam"},
			},
			TranslationSources: map[string][]string{"en": {"cedict", "cedict", "cedict", "cedict"}, "de": {"cedict", "cedict"}},
			Tags:               []string{"HSK1"},
		}
	}
	return reqs
}

func BenchmarkImport1000Words(b *testing.B) {
	ctx := context.Background()
	b.Run("CreateWordPerWord", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			s := openTestDB(b)
			b.StartTimer()
			for _, req := range benchWords(1000) {
				if _, err := s.CreateWord(ctx, 2, req); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("CreateWordsBatch200", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			s := openTestDB(b)
			b.StartTimer()
			reqs := benchWords(1000)
			for start := 0; start < len(reqs); start += 200 {
				if _, err := s.CreateWordsBatch(ctx, 2, reqs[start:start+200]); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

func TestCreateWordsBatch_ReusesTagRowsAcrossWords(t *testing.T) {
	s := openTestDB(t)
	reqs := []models.CreateWordRequest{
		{ZhText: "甲", Translations: map[string][]string{"en": {"a"}}, Tags: []string{"T1"}},
		{ZhText: "乙", Translations: map[string][]string{"en": {"b"}}, Tags: []string{"T1"}},
		{ZhText: "丙", Translations: map[string][]string{"en": {"c"}}, Tags: []string{"T1"}},
	}
	if _, err := s.CreateWordsBatch(context.Background(), 2, reqs); err != nil {
		t.Fatalf("CreateWordsBatch: %v", err)
	}
	if _, err := s.CreateWordsBatch(context.Background(), 2, []models.CreateWordRequest{
		{ZhText: "丁", Translations: map[string][]string{"en": {"d"}}, Tags: []string{"T1"}},
	}); err != nil {
		t.Fatalf("second CreateWordsBatch: %v", err)
	}

	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tags WHERE name = 'T1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("tag rows for T1 = %d, want 1", n)
	}
	var tagged int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM word_tags wt JOIN tags t ON t.id = wt.tag_id WHERE t.name = 'T1'`).Scan(&tagged); err != nil {
		t.Fatal(err)
	}
	if tagged != 4 {
		t.Errorf("words tagged T1 = %d, want 4", tagged)
	}
}

func TestCreateWordsBatch_ReusesExistingWordsAndTranslations(t *testing.T) {
	ctx := context.Background()
	s := openTestDB(t)
	first, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText: "你好", Pinyin: "nǐ hǎo", Translations: map[string][]string{"en": {"hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	ids, err := s.CreateWordsBatch(ctx, 2, []models.CreateWordRequest{
		{ZhText: "你好", Pinyin: "changed", Translations: map[string][]string{"en": {"hello", "hi"}}},
		{ZhText: "您好", Translations: map[string][]string{"en": {"hello"}}},
	})
	if err != nil {
		t.Fatalf("CreateWordsBatch: %v", err)
	}
	if ids[0] != first {
		t.Errorf("existing word got new id %d, want %d", ids[0], first)
	}

	var helloRows int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE text = 'hello' AND language = 'en' AND user_id = 2`).Scan(&helloRows); err != nil {
		t.Fatal(err)
	}
	if helloRows != 1 {
		t.Errorf("'hello' rows = %d, want the one shared word", helloRows)
	}
	var pinyin string
	if err := s.db.QueryRow(`SELECT pinyin FROM words WHERE id = ?`, first).Scan(&pinyin); err != nil {
		t.Fatal(err)
	}
	if pinyin != "nǐ hǎo" {
		t.Errorf("pinyin = %q, existing word must keep its pinyin", pinyin)
	}
}
