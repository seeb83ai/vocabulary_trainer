package db

import (
	"context"
	"testing"
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
