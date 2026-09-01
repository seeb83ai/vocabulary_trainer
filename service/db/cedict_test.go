package db

import (
	"context"
	"testing"

	"vocabulary_trainer/models"
)

func TestSegmentZhText_SplitsSingleCharAndMultiCharTokens(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedCedictEntryForTest(ctx, "足球", "en", "zú qiú", "football"); err != nil {
		t.Fatal(err)
	}

	tokens, ok, err := segmentZhText(ctx, s.db, "踢足球")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("want ok=true")
	}
	if len(tokens) != 2 {
		t.Fatalf("want 2 tokens, got %d: %+v", len(tokens), tokens)
	}
	if tokens[0].Text != "踢" || !tokens[0].IsSingle {
		t.Errorf("token[0] = %+v, want single-char 踢", tokens[0])
	}
	if tokens[1].Text != "足球" || tokens[1].IsSingle || tokens[1].DefinitionEN != "football" {
		t.Errorf("token[1] = %+v, want multi-char 足球/football", tokens[1])
	}
}

func TestSegmentZhText_PrefersLongestMatchFirst(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedCedictEntryForTest(ctx, "中华人", "en", "zhōng huá rén", "spurious 3-char match"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedCedictEntryForTest(ctx, "中华", "en", "zhōng huá", "China (adj)"); err != nil {
		t.Fatal(err)
	}

	tokens, ok, err := segmentZhText(ctx, s.db, "中华人")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("want ok=true")
	}
	if len(tokens) != 1 || tokens[0].Text != "中华人" {
		t.Errorf("want the 3-char match to win, got %+v", tokens)
	}
}

func TestSegmentZhText_FallsBackToSingleCharsWhenNoCedictData(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	tokens, ok, err := segmentZhText(ctx, s.db, "踢足球")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("want ok=true even with no cedict data")
	}
	if len(tokens) != 3 {
		t.Fatalf("want 3 single-char tokens, got %d: %+v", len(tokens), tokens)
	}
	for _, tok := range tokens {
		if !tok.IsSingle {
			t.Errorf("token %+v should be IsSingle with no cedict data", tok)
		}
	}
}

func TestSegmentZhText_GuardRejectsTooLong(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	_, ok, err := segmentZhText(ctx, s.db, "一二三四五六七八九")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("want ok=false for a 9-rune input")
	}
}

func TestSegmentZhText_GuardRejectsSentencePunctuation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	_, ok, err := segmentZhText(ctx, s.db, "你好吗？")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("want ok=false for input containing sentence punctuation")
	}
}

func TestSegmentZhText_EmptyInput(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	_, ok, err := segmentZhText(ctx, s.db, "")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("want ok=false for empty input")
	}
}

func TestSegmentZhText_BothLangDefinitions(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedCedictEntryForTest(ctx, "足球", "en", "zú qiú", "football"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedCedictEntryForTest(ctx, "足球", "de", "zú qiú", "Fußball"); err != nil {
		t.Fatal(err)
	}

	tokens, ok, err := segmentZhText(ctx, s.db, "足球")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(tokens) != 1 {
		t.Fatalf("want a single matched token, got ok=%v tokens=%+v", ok, tokens)
	}
	if tokens[0].DefinitionEN != "football" || tokens[0].DefinitionDE != "Fußball" {
		t.Errorf("want both definitions populated, got %+v", tokens[0])
	}
}

func TestLookupDictionary_ExactMatchByLang(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedCedictEntryForTest(ctx, "足球", "en", "zú qiú", "football"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedCedictEntryForTest(ctx, "足球", "de", "zú qiú", "Fußball"); err != nil {
		t.Fatal(err)
	}

	en, err := s.LookupDictionary(ctx, "足球", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(en) != 1 || en[0] != "football" {
		t.Errorf("en lookup = %+v, want [football]", en)
	}

	de, err := s.LookupDictionary(ctx, "足球", "de")
	if err != nil {
		t.Fatal(err)
	}
	if len(de) != 1 || de[0] != "Fußball" {
		t.Errorf("de lookup = %+v, want [Fußball]", de)
	}

	none, err := s.LookupDictionary(ctx, "不存在", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("want empty slice for no match, got %+v", none)
	}
}

func TestCreateSubwordsForWord_TwoCharParentCreatesCharSubwords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	// Seed cedict for both the parent word and its constituent chars.
	if err := s.SeedCedictEntryForTest(ctx, "炒饭", "en", "chǎo fàn", "fried rice"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedCedictEntryForTest(ctx, "炒", "en", "chǎo", "to stir-fry"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedCedictEntryForTest(ctx, "饭", "en", "fàn", "cooked rice"); err != nil {
		t.Fatal(err)
	}

	zhID, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:       "炒饭",
		Translations: map[string][]string{"en": {"fried rice"}},
		Tags:         []string{"HSK1"},
		StartTraining: true,
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}
	_ = zhID

	for _, ch := range []string{"炒", "饭"} {
		exists, err := s.IsZhWordForUser(ctx, 2, ch)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("want %q auto-created as subword of 炒饭", ch)
		}
	}
}

func TestCreateSubwordsForWord_SplitsSemicolonDefinitions(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedCedictEntryForTest(ctx, "炒饭", "en", "chǎo fàn", "fried rice"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedCedictEntryForTest(ctx, "炒", "en", "chǎo", "to sauté; to stir-fry; to fire (sb)"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:        "炒饭",
		Translations:  map[string][]string{"en": {"fried rice"}},
		StartTraining: true,
	}); err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	// 炒 should exist and its translations should be the split senses, not the raw string.
	exists, err := s.IsZhWordForUser(ctx, 2, "炒")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("want 炒 auto-created as subword")
	}
	// Check that "to sauté; to stir-fry; to fire (sb)" was NOT stored as one word.
	var rawCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM words WHERE text = ? AND user_id = ?`,
		"to sauté; to stir-fry; to fire (sb)", int64(2),
	).Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if rawCount > 0 {
		t.Error("raw semicolon-joined definition was stored as a word — want it split into parts")
	}
	// At least "to sauté" and "to stir-fry" should exist as individual translation words.
	for _, sense := range []string{"to sauté", "to stir-fry", "to fire (sb)"} {
		var cnt int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM words WHERE text = ? AND user_id = ?`, sense, int64(2),
		).Scan(&cnt); err != nil {
			t.Fatal(err)
		}
		if cnt == 0 {
			t.Errorf("want sense %q stored as individual translation word", sense)
		}
	}
}

func TestLookupDictionary_SplitsSemicolons(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	// Single row with multiple senses separated by "; "
	if err := s.SeedCedictEntryForTest(ctx, "过", "en", "guò", "to cross; to go over; to pass (time)"); err != nil {
		t.Fatal(err)
	}
	// Second row that is already a single sense (no semicolon)
	if err := s.SeedCedictEntryForTest(ctx, "过", "en", "guò", "experienced action marker"); err != nil {
		t.Fatal(err)
	}

	got, err := s.LookupDictionary(ctx, "过", "en")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"to cross", "to go over", "to pass (time)", "experienced action marker"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}
