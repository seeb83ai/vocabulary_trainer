package db

import (
	"context"
	"reflect"
	"testing"
)

type copiedGloss struct{ lang, text, source string }

// seedCopy creates an own zh word with copied gloss links, as list imports
// did before library references.
func seedCopy(t *testing.T, s *Store, userID int64, zh, pinyin string, glosses ...copiedGloss) int64 {
	t.Helper()
	res, err := s.db.Exec(`INSERT INTO words (text, language, pinyin, user_id) VALUES (?, 'zh', ?, ?)`, zh, pinyin, userID)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	s.db.Exec(`INSERT INTO sm2_progress (word_id, total_attempts, total_correct) VALUES (?, 5, 4)`, id)
	for _, g := range glosses {
		s.db.Exec(`INSERT OR IGNORE INTO words (text, language, user_id) VALUES (?, ?, ?)`, g.text, g.lang, userID)
		if _, err := s.db.Exec(`INSERT INTO translations (translation_word_id, zh_word_id, source, rank)
			SELECT id, ?, ?, 0 FROM words WHERE text = ? AND language = ? AND user_id = ?`, id, g.source, g.text, g.lang, userID); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func markActive(t *testing.T, s *Store, userID int64) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO daily_stats (user_id, date, attempts) VALUES (?, date('now'), 3)`, userID); err != nil {
		t.Fatal(err)
	}
}

func glossesWithSources(t *testing.T, s *Store, userID, id int64) map[string][]string {
	t.Helper()
	wd, err := s.GetWordByID(context.Background(), userID, id)
	if err != nil || wd == nil {
		t.Fatalf("GetWordByID(%d): %v, %v", id, wd, err)
	}
	out := map[string][]string{}
	for lang, texts := range wd.Translations {
		for i, text := range texts {
			out[lang] = append(out[lang], text+"|"+wd.TranslationSources[lang][i])
		}
	}
	return out
}

func isReference(t *testing.T, s *Store, id int64) bool {
	t.Helper()
	var lib *int64
	s.db.QueryRow(`SELECT library_word_id FROM words WHERE id = ?`, id).Scan(&lib)
	return lib != nil
}

func TestConvertToLibraryReferences_ActiveUserSeesTheSameGlosses(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedCedict(t, s, "吃", "en", "to eat; to consume")
	seedCedict(t, s, "吃", "de", "essen")
	markActive(t, s, 2)
	eat := seedCopy(t, s, 2, "吃", "chī!", copiedGloss{"en", "to eat", "cedict"}, copiedGloss{"en", "to munch", "user"})
	sentence := seedCopy(t, s, 2, "我买牛奶。", "", copiedGloss{"en", "I buy milk.", "user"})
	if err := s.AddWordTags(ctx, 2, eat, []string{"hsk3-1"}); err != nil {
		t.Fatal(err)
	}
	before := glossesWithSources(t, s, 2, eat)

	if _, err := s.ConvertToLibraryReferences(ctx); err != nil {
		t.Fatalf("convert: %v", err)
	}

	if !isReference(t, s, eat) {
		t.Fatal("吃 is not a library reference")
	}
	if after := glossesWithSources(t, s, 2, eat); !reflect.DeepEqual(after, before) {
		t.Errorf("glosses changed: %v -> %v", before, after)
	}
	wd, _ := s.GetWordByID(ctx, 2, eat)
	if wd.Pinyin == nil || *wd.Pinyin != "chī!" || wd.TotalAttempts != 5 || !reflect.DeepEqual(wd.Tags, []string{"hsk3-1"}) {
		t.Errorf("word = pinyin %v attempts %d tags %v, want them kept", wd.Pinyin, wd.TotalAttempts, wd.Tags)
	}
	if isReference(t, s, sentence) {
		t.Error("sentence without dictionary entry became a reference")
	}
	// Later dictionary updates reach the converted word.
	dictionaryUpdate(t, s, "吃", "en", "to dine")
	if got := glossesWithSources(t, s, 2, eat)["en"]; !reflect.DeepEqual(got, []string{"to dine|cedict", "to eat|cedict", "to munch|user"}) {
		t.Errorf("after update en = %v", got)
	}
}

func TestConvertToLibraryReferences_DormantUserGetsTheLibrary(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedCedict(t, s, "吃", "en", "to eat; to consume")
	eat := seedCopy(t, s, 2, "吃", "chī", copiedGloss{"en", "to eat", "user"}, copiedGloss{"en", "to munch", "user"})

	if _, err := s.ConvertToLibraryReferences(ctx); err != nil {
		t.Fatalf("convert: %v", err)
	}
	if got := glossesWithSources(t, s, 2, eat)["en"]; !reflect.DeepEqual(got, []string{"to consume|cedict", "to eat|cedict", "to munch|user"}) {
		t.Errorf("en = %v, want the library glosses plus the own extra", got)
	}
}

func TestConvertToLibraryReferences_LanguagesFollowTheCopiedGlosses(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedCedict(t, s, "吃", "en", "to eat")
	seedCedict(t, s, "吃", "de", "essen")
	enOnly := newTestUser(t, s, "en-only@example.de")
	eat := seedCopy(t, s, enOnly, "吃", "chī", copiedGloss{"en", "to eat", "cedict"})

	if _, err := s.ConvertToLibraryReferences(ctx); err != nil {
		t.Fatalf("convert: %v", err)
	}
	if got := glossesWithSources(t, s, enOnly, eat); !reflect.DeepEqual(got, map[string][]string{"en": {"to eat|cedict"}}) {
		t.Errorf("glosses = %v, want EN only", got)
	}
	st, _ := s.GetUserSettings(ctx, enOnly)
	if st.SecondaryLang != "" {
		t.Errorf("secondary language = %q, want empty", st.SecondaryLang)
	}
}

func TestConvertToLibraryReferences_CleansCopiesAndRunsOnce(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedCedict(t, s, "吃", "en", "to eat")
	seedCopy(t, s, 2, "吃", "chī", copiedGloss{"en", "to eat", "cedict"})

	pending, err := s.LibraryConversionPending(ctx)
	if err != nil || !pending {
		t.Fatalf("pending = %v, %v; want true", pending, err)
	}
	if _, err := s.ConvertToLibraryReferences(ctx); err != nil {
		t.Fatal(err)
	}
	var glossWords int
	s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id = 2 AND language != 'zh'`).Scan(&glossWords)
	if glossWords != 0 {
		t.Errorf("%d copied gloss words left, want 0", glossWords)
	}
	if pending, _ := s.LibraryConversionPending(ctx); pending {
		t.Error("still pending after the conversion")
	}
}

func TestCompareConvertedGlosses(t *testing.T) {
	before := map[int64]map[string][]string{1: {"en": {"to eat|cedict", "to munch|user"}}}
	same := map[int64]map[string][]string{1: {"en": {"to eat|cedict", "to munch|user"}}}
	more := map[int64]map[string][]string{1: {"en": {"to consume|cedict", "to eat|cedict", "to munch|user"}}}
	less := map[int64]map[string][]string{1: {"en": {"to eat|cedict"}}}

	if err := compareConvertedGlosses(before, same, true); err != nil {
		t.Errorf("faithful, same: %v", err)
	}
	if err := compareConvertedGlosses(before, more, true); err == nil {
		t.Error("faithful, more: want an error")
	}
	if err := compareConvertedGlosses(before, more, false); err != nil {
		t.Errorf("superset, more: %v", err)
	}
	if err := compareConvertedGlosses(before, less, false); err == nil {
		t.Error("superset, less: want an error")
	}
}
