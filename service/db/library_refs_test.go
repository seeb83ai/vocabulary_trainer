package db

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

// libraryUserID is the shared library (template) user.
const testLibraryUserID int64 = 1

// seedLibraryWord creates a library zh word with the given glosses (source
// cedict) and returns its id.
func seedLibraryWord(t *testing.T, s *Store, zh, pinyin string, glosses map[string][]string) int64 {
	t.Helper()
	ctx := context.Background()
	sources := map[string][]string{}
	for lang, texts := range glosses {
		for range texts {
			sources[lang] = append(sources[lang], "cedict")
		}
	}
	id, err := s.CreateWord(ctx, testLibraryUserID, models.CreateWordRequest{
		ZhText: zh, Pinyin: pinyin, Translations: glosses, TranslationSources: sources,
	})
	if err != nil {
		t.Fatalf("seed library word %q: %v", zh, err)
	}
	return id
}

// seedReference gives userID a library reference to libraryID: a slim zh
// word row with library_word_id set and no gloss links of its own.
func seedReference(t *testing.T, s *Store, userID, libraryID int64) int64 {
	t.Helper()
	res, err := s.db.Exec(
		`INSERT INTO words (text, language, pinyin, user_id, library_word_id)
		 SELECT text, 'zh', pinyin, ?, id FROM words WHERE id = ?`, userID, libraryID)
	if err != nil {
		t.Fatalf("seed reference: %v", err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.db.Exec(`INSERT INTO sm2_progress (word_id) VALUES (?)`, id); err != nil {
		t.Fatalf("seed reference progress: %v", err)
	}
	return id
}

func newTestUser(t *testing.T, s *Store, email string) int64 {
	t.Helper()
	id, err := s.CreateUser(context.Background(), email, "x", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return id
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestLibraryReference_ShowsLibraryGlosses(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	lib := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat"}, "de": {"essen"}})
	ref := seedReference(t, s, 2, lib)

	wd, err := s.GetWordByID(ctx, 2, ref)
	if err != nil || wd == nil {
		t.Fatalf("GetWordByID: %v, %v", wd, err)
	}
	if got := wd.Translations["en"]; !reflect.DeepEqual(got, []string{"to eat"}) {
		t.Errorf("en = %v, want [to eat]", got)
	}
	if got := wd.Translations["de"]; !reflect.DeepEqual(got, []string{"essen"}) {
		t.Errorf("de = %v, want [essen]", got)
	}
}

func TestLibraryReference_ShowsOnlyLearnerLanguages(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	lib := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat"}, "de": {"essen"}})
	ref := seedReference(t, s, 2, lib)
	st, err := s.GetUserSettings(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	st.SecondaryLang = ""
	if err := s.UpdateUserSettings(ctx, 2, *st); err != nil {
		t.Fatal(err)
	}

	wd, err := s.GetWordByID(ctx, 2, ref)
	if err != nil || wd == nil {
		t.Fatalf("GetWordByID: %v, %v", wd, err)
	}
	if _, ok := wd.Translations["de"]; ok {
		t.Errorf("de glosses shown although secondary language is empty: %v", wd.Translations["de"])
	}
	if got := wd.Translations["en"]; !reflect.DeepEqual(got, []string{"to eat"}) {
		t.Errorf("en = %v, want [to eat]", got)
	}
}

func TestLibraryReference_OverridesAndIsolation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	lib := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat", "to consume"}})
	other := newTestUser(t, s, "other@example.de")
	ref := seedReference(t, s, 2, lib)
	otherRef := seedReference(t, s, other, lib)

	// Learner 2 deletes "to consume" and adds "to munch".
	if _, err := s.db.Exec(`INSERT INTO translation_deletions (user_word_id, translation_word_id)
		SELECT ?, id FROM words WHERE user_id = 1 AND language = 'en' AND text = 'to consume'`, ref); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTranslation(ctx, 2, ref, "en", "to munch"); err != nil {
		t.Fatal(err)
	}

	wd, err := s.GetWordByID(ctx, 2, ref)
	if err != nil || wd == nil {
		t.Fatalf("GetWordByID: %v, %v", wd, err)
	}
	if got := wd.Translations["en"]; !reflect.DeepEqual(got, []string{"to eat", "to munch"}) {
		t.Errorf("learner en = %v, want [to eat to munch]", got)
	}
	if got := wd.TranslationSources["en"]; !reflect.DeepEqual(got, []string{"cedict", "user"}) {
		t.Errorf("learner sources = %v, want [cedict user]", got)
	}

	owd, err := s.GetWordByID(ctx, other, otherRef)
	if err != nil || owd == nil {
		t.Fatalf("GetWordByID other: %v, %v", owd, err)
	}
	if got := owd.Translations["en"]; !reflect.DeepEqual(got, []string{"to consume", "to eat"}) {
		t.Errorf("other learner en = %v, want [to consume to eat]", got)
	}
}

func TestLibraryReference_ReadPathsUseOverlay(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	eat := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat", "to consume"}})
	devour := seedLibraryWord(t, s, "吞", "tūn", map[string][]string{"en": {"to swallow", "to consume"}})
	refEat := seedReference(t, s, 2, eat)
	refDevour := seedReference(t, s, 2, devour)

	// Word list: search by a library gloss finds the reference.
	words, total, err := s.GetWords(ctx, 2, "swallow", 1, 50, "", "", nil, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || words[0].ID != refDevour {
		t.Fatalf("search 'swallow' = %d words (%v), want only the 吞 reference", total, words)
	}
	if got := sortedCopy(words[0].Translations["en"]); !reflect.DeepEqual(got, []string{"to consume", "to swallow"}) {
		t.Errorf("list glosses = %v", got)
	}

	// Quiz answer glosses.
	tw, err := s.GetTranslationsForWord(ctx, refEat, "en")
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, w := range tw {
		texts = append(texts, w.Text)
	}
	if got := sortedCopy(texts); !reflect.DeepEqual(got, []string{"to consume", "to eat"}) {
		t.Errorf("quiz glosses = %v", got)
	}

	// Shared translation between two references.
	shared, err := s.SharesTranslation(ctx, refEat, refDevour, []string{"en"})
	if err != nil || !shared {
		t.Errorf("SharesTranslation = %v, %v; want true", shared, err)
	}

	// Confusion: answering "to swallow" for 吃 points at the 吞 reference
	// once 吞 has been seen.
	if _, err := s.db.Exec(`UPDATE sm2_progress SET first_seen_at = CURRENT_TIMESTAMP WHERE word_id = ?`, refDevour); err != nil {
		t.Fatal(err)
	}
	got, found, err := s.DetectConfusion(ctx, 2, refEat, "to swallow", models.ModeZhToTransl, []string{"en"})
	if err != nil || !found || got != refDevour {
		t.Errorf("DetectConfusion = %d, %v, %v; want %d", got, found, err, refDevour)
	}

	// Missing-language filter: references have no de gloss.
	_, missing, err := s.GetWords(ctx, 2, "", 1, 50, "", "", nil, false, false, "", "", "de")
	if err != nil || missing != 2 {
		t.Errorf("missing de = %d, %v; want 2", missing, err)
	}
}
