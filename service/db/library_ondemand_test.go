package db

import (
	"context"
	"reflect"
	"testing"
	"vocabulary_trainer/models"
)

// dictionaryUpdate adds a sense to zh and refreshes the library.
func dictionaryUpdate(t *testing.T, s *Store, zh, lang, def string) {
	t.Helper()
	seedCedict(t, s, zh, lang, def)
	if _, err := s.RefreshLibrary(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCreateWord_DictionaryWordBecomesReference(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedCedict(t, s, "喝", "en", "to drink")

	id, err := s.CreateWord(ctx, 2, models.CreateWordRequest{ZhText: "喝", Pinyin: "hē",
		Translations: map[string][]string{"en": {"to drink"}}, TranslationSources: map[string][]string{"en": {"cedict"}}})
	if err != nil {
		t.Fatal(err)
	}
	dictionaryUpdate(t, s, "喝", "en", "to sip")

	wd, _ := s.GetWordByID(ctx, 2, id)
	if got := wd.Translations["en"]; !reflect.DeepEqual(got, []string{"to drink", "to sip"}) {
		t.Errorf("en = %v, want the dictionary update to reach the word", got)
	}
	if wd.Pinyin == nil || *wd.Pinyin != "hē" {
		t.Errorf("pinyin = %v, want the learner's hē", wd.Pinyin)
	}
}

func TestCreateWord_OwnGlossesOnDictionaryWordStayPrivate(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedCedict(t, s, "喝", "en", "to drink")
	other := newTestUser(t, s, "other@example.de")

	mine, err := s.CreateWord(ctx, 2, models.CreateWordRequest{ZhText: "喝",
		Translations: map[string][]string{"en": {"to sip"}}})
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := s.CreateWord(ctx, other, models.CreateWordRequest{ZhText: "喝",
		Translations: map[string][]string{"en": {"to drink"}}})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(ctx, 2, mine)
	if got := wd.Translations["en"]; !reflect.DeepEqual(got, []string{"to sip"}) {
		t.Errorf("mine = %v, want [to sip]", got)
	}
	owd, _ := s.GetWordByID(ctx, other, theirs)
	if got := owd.Translations["en"]; !reflect.DeepEqual(got, []string{"to drink"}) {
		t.Errorf("theirs = %v, want [to drink]", got)
	}
}

func TestCreateWord_SentenceWithoutDictionaryEntryStaysOwn(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id, err := s.CreateWord(ctx, 2, models.CreateWordRequest{ZhText: "我买牛奶。",
		Translations: map[string][]string{"en": {"I buy milk."}}})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(ctx, 2, id)
	if got := wd.Translations["en"]; !reflect.DeepEqual(got, []string{"I buy milk."}) {
		t.Errorf("en = %v", got)
	}
	var lib int
	s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id = 1 AND text = '我买牛奶。'`).Scan(&lib)
	if lib != 0 {
		t.Error("sentence was added to the library")
	}
}

func TestCreateSubwords_AreLibraryReferences(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedCedict(t, s, "吃饭", "en", "to have a meal")
	seedCedict(t, s, "吃", "en", "to eat")
	seedCedict(t, s, "饭", "en", "rice")

	if _, err := s.CreateWord(ctx, 2, models.CreateWordRequest{ZhText: "吃饭", StartTraining: true,
		Translations: map[string][]string{"en": {"to have a meal"}}}); err != nil {
		t.Fatal(err)
	}
	fan, err := s.GetWordIDByZhText(ctx, 2, "饭")
	if err != nil || fan == 0 {
		t.Fatalf("subword 饭 not created: %d, %v", fan, err)
	}
	dictionaryUpdate(t, s, "饭", "en", "meal")
	wd, _ := s.GetWordByID(ctx, 2, fan)
	if got := wd.Translations["en"]; !reflect.DeepEqual(got, []string{"meal", "rice"}) {
		t.Errorf("subword en = %v, want the dictionary update to reach it", got)
	}
}
