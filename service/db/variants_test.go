package db

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// storedVariants returns the sorted variants stored for the word with this
// text, language and user.
func storedVariants(t *testing.T, s *Store, text, lang string, userID int64) []string {
	t.Helper()
	rows, err := s.db.Query(`
		SELECT v.variant FROM translation_variants v
		JOIN words w ON w.id = v.word_id
		WHERE w.text = ? AND w.language = ? AND w.user_id = ?
		ORDER BY v.variant`, text, lang, userID)
	if err != nil {
		t.Fatalf("query variants of %q: %v", text, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func TestTranslationVariants_StoredOnCreateWord(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "狗", "gǒu", []string{"Dog / hound"})

	if got, want := storedVariants(t, s, "Dog / hound", "en", 2), []string{"dog", "dog / hound", "hound"}; !reflect.DeepEqual(got, want) {
		t.Errorf("variants = %q, want %q", got, want)
	}
	if got := storedVariants(t, s, "狗", "zh", 2); got != nil {
		t.Errorf("zh word has variants %q, want none", got)
	}
}

func TestTranslationVariants_StoredByLibraryRefresh(t *testing.T) {
	s := openTestDB(t)
	seedBareLibraryWord(t, s, "月", "yuè")
	seedCedict(t, s, "月", "de", "Mond (S)")
	if _, err := s.RefreshLibrary(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got, want := storedVariants(t, s, "Mond (S)", "de", testLibraryUserID), []string{"mond", "mond (s"}; !reflect.DeepEqual(got, want) {
		t.Errorf("variants = %q, want %q", got, want)
	}
}

// TestTranslationVariants_FilledByMigrate covers translations that a CLI tool
// or a migration inserts with plain SQL: Migrate (run at every start) adds
// their variants.
func TestTranslationVariants_FilledByMigrate(t *testing.T) {
	s := openTestDB(t)
	if _, err := s.db.Exec(`INSERT INTO words (text, language, user_id) VALUES ('Hund (S)', 'de', 2)`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(s.db); err != nil {
		t.Fatal(err)
	}

	if got, want := storedVariants(t, s, "Hund (S)", "de", 2), []string{"hund", "hund (s"}; !reflect.DeepEqual(got, want) {
		t.Errorf("variants = %q, want %q", got, want)
	}
}

func TestTranslationVariants_DeletedWithWord(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "狗", "gǒu", []string{"dog"})
	if _, err := s.db.Exec(`DELETE FROM words WHERE text = 'dog' AND user_id = 2`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM translation_variants WHERE variant = 'dog'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d variant rows left after the word was deleted, want 0", n)
	}
}

// TestTranslationVariants_RulesUnchanged guards the stored variants: they are
// computed with sm2.ExpandVariants when a word is inserted. If this test fails,
// the answer rules changed, and the stored variants of existing words are
// out of date. Add a migration that runs
// "DELETE FROM translation_variants" and then FillTranslationVariants, and
// update the expected values here.
func TestTranslationVariants_RulesUnchanged(t *testing.T) {
	cases := map[string][]string{
		"dog / hound":             {"dog", "dog / hound", "hound"},
		"Mond (S)":                {"mond", "mond (s"},
		"to put (sb, sth) in":     {"to put (sb, sth) in", "to put in"},
		"lesen, vorlesen (V)":     {"lesen", "lesen, vorlesen", "lesen, vorlesen (v", "vorlesen", "vorlesen (v"},
		"Don’t worry":             {"don't worry"},
		"Wie heißt sie？":          {"wie heißt sie"},
		"well…then":               {"well then"},
		"（Anzahl） mal":            {"mal", "（anzahl） mal"},
		"  SPACES   inside  ":     {"spaces inside"},
		"Übung (S), Training":     {"training", "übung", "übung (s", "übung (s), training", "übung , training"},
		"to (be) eat":             {"to (be) eat", "to eat"},
		"Bsp.: 我是中國人。 -- Ich bin": {"bsp : 我是中國人 -- ich bin"},
	}
	for text, want := range cases {
		got := sm2.ExpandVariants(text)
		slices.Sort(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ExpandVariants(%q) = %q, want %q", text, got, want)
		}
	}
}

// TestTranslationPinyins_LibraryReference checks that a translation of a
// library word counts for the user's reference to it.
func TestTranslationPinyins_LibraryReference(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	lib := seedLibraryWord(t, s, "她", "tā", map[string][]string{"en": {"she"}})
	seedReference(t, s, 2, lib)
	other := newTestUser(t, s, "other@example.com")
	if _, err := s.CreateWord(ctx, other, models.CreateWordRequest{
		ZhText: "鬼", Pinyin: "guǐ", Translations: map[string][]string{"en": {"she"}},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.TranslationPinyins(ctx, 2, []string{"she"})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string][]string{"she": {"tā"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("TranslationPinyins = %q, want %q", got, want)
	}
}
