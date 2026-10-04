package db

import (
	"context"
	"reflect"
	"testing"
	"vocabulary_trainer/models"
)

// seedBareLibraryWord creates a library zh word with no glosses, as the HSK
// and topic import tools leave it.
func seedBareLibraryWord(t *testing.T, s *Store, zh, pinyin string) int64 {
	t.Helper()
	id, err := s.CreateWord(context.Background(), testLibraryUserID, models.CreateWordRequest{ZhText: zh, Pinyin: pinyin})
	if err != nil {
		t.Fatalf("seed library word %q: %v", zh, err)
	}
	return id
}

func seedCedict(t *testing.T, s *Store, zh, lang, def string) {
	t.Helper()
	if err := s.SeedCedictEntryForTest(context.Background(), zh, lang, "x", def); err != nil {
		t.Fatalf("seed cedict %q: %v", zh, err)
	}
}

func libraryGlosses(t *testing.T, s *Store, id int64) map[string][]string {
	t.Helper()
	wd, err := s.GetWordByID(context.Background(), testLibraryUserID, id)
	if err != nil || wd == nil {
		t.Fatalf("GetWordByID(%d): %v, %v", id, wd, err)
	}
	return wd.Translations
}

func TestRefreshLibrary_FillsGlossesFromDictionarySenses(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	eat := seedBareLibraryWord(t, s, "吃", "chī")
	seedCedict(t, s, "吃", "en", "to eat; to consume")
	seedCedict(t, s, "吃", "de", "essen, fressen")

	if _, err := s.RefreshLibrary(ctx); err != nil {
		t.Fatalf("RefreshLibrary: %v", err)
	}

	got := libraryGlosses(t, s, eat)
	want := map[string][]string{"en": {"to consume", "to eat"}, "de": {"essen", "fressen"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("library glosses = %v, want %v", got, want)
	}
	wd, _ := s.GetWordByID(ctx, testLibraryUserID, eat)
	for _, src := range wd.TranslationSources["en"] {
		if src != "cedict" {
			t.Errorf("source = %q, want cedict", src)
		}
	}
}

func glossIDs(t *testing.T, s *Store, zhID int64, lang string) map[string]int64 {
	t.Helper()
	words, err := s.GetTranslationsForWord(context.Background(), zhID, lang)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, w := range words {
		out[w.Text] = w.ID
	}
	return out
}

func libraryUpdatedAt(t *testing.T, s *Store, id int64) string {
	t.Helper()
	var v *string
	if err := s.db.QueryRow(`SELECT library_updated_at FROM words WHERE id = ?`, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v == nil {
		return ""
	}
	return *v
}

func TestRefreshLibrary_SecondRunChangesNothing(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	eat := seedBareLibraryWord(t, s, "吃", "chī")
	seedCedict(t, s, "吃", "en", "to eat; to consume")
	if _, err := s.RefreshLibrary(ctx); err != nil {
		t.Fatal(err)
	}
	before := glossIDs(t, s, eat, "en")

	report, err := s.RefreshLibrary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Added != 0 || report.Dropped != 0 || report.Changed != 0 {
		t.Errorf("second run report = %+v, want no changes", report)
	}
	if after := glossIDs(t, s, eat, "en"); !reflect.DeepEqual(before, after) {
		t.Errorf("gloss ids changed: %v -> %v", before, after)
	}
	if got := libraryUpdatedAt(t, s, eat); got != "" {
		t.Errorf("library_updated_at = %q after first fill, want empty", got)
	}
}

func TestRefreshLibrary_DictionaryChangeKeepsIDsAndMarksWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	eat := seedBareLibraryWord(t, s, "吃", "chī")
	drink := seedBareLibraryWord(t, s, "喝", "hē")
	seedCedict(t, s, "吃", "en", "to eat; to consume")
	seedCedict(t, s, "喝", "en", "to drink")
	if _, err := s.RefreshLibrary(ctx); err != nil {
		t.Fatal(err)
	}
	before := glossIDs(t, s, eat, "en")

	// New dictionary version: "to consume" replaced by "to dine".
	if _, err := s.db.Exec(`UPDATE cedict_entries SET definition = 'to eat; to dine' WHERE simplified = '吃'`); err != nil {
		t.Fatal(err)
	}
	report, err := s.RefreshLibrary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Added != 1 || report.Dropped != 1 || report.Changed != 1 {
		t.Errorf("report = %+v, want 1 added, 1 dropped, 1 changed", report)
	}
	after := glossIDs(t, s, eat, "en")
	if after["to eat"] != before["to eat"] {
		t.Errorf("unchanged gloss got a new id: %d -> %d", before["to eat"], after["to eat"])
	}
	if _, ok := after["to consume"]; ok {
		t.Errorf("dropped gloss still linked: %v", after)
	}
	if _, ok := after["to dine"]; !ok {
		t.Errorf("new gloss missing: %v", after)
	}
	if libraryUpdatedAt(t, s, eat) == "" {
		t.Error("changed word has no library_updated_at")
	}
	if libraryUpdatedAt(t, s, drink) != "" {
		t.Error("unchanged word got library_updated_at")
	}
	var orphans int
	s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id = 1 AND text = 'to consume'`).Scan(&orphans)
	if orphans != 0 {
		t.Errorf("orphan gloss word kept")
	}
}

func TestRefreshLibrary_WordLeavingDictionaryIsFlaggedNotDeleted(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	eat := seedBareLibraryWord(t, s, "吃", "chī")
	seedCedict(t, s, "吃", "en", "to eat")
	if _, err := s.RefreshLibrary(ctx); err != nil {
		t.Fatal(err)
	}
	ref := seedReference(t, s, 2, eat)

	s.db.Exec(`DELETE FROM cedict_entries WHERE simplified = '吃'`)
	report, err := s.RefreshLibrary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Missing != 1 {
		t.Errorf("report = %+v, want 1 missing", report)
	}
	wd, err := s.GetWordByID(ctx, 2, ref)
	if err != nil || wd == nil {
		t.Fatalf("reference gone: %v, %v", wd, err)
	}
	if !reflect.DeepEqual(wd.Translations["en"], []string{"to eat"}) {
		t.Errorf("reference glosses = %v, want the last known [to eat]", wd.Translations)
	}
	var removed int
	s.db.QueryRow(`SELECT library_removed FROM words WHERE id = ?`, eat).Scan(&removed)
	if removed != 1 {
		t.Error("library word not flagged removed")
	}

	seedCedict(t, s, "吃", "en", "to eat")
	if _, err := s.RefreshLibrary(ctx); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow(`SELECT library_removed FROM words WHERE id = ?`, eat).Scan(&removed)
	if removed != 0 {
		t.Error("flag not cleared when the word is back")
	}
}

func TestLibraryNeedsPrefill(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedBareLibraryWord(t, s, "吃", "chī")

	if need, err := s.LibraryNeedsPrefill(ctx); err != nil || need {
		t.Errorf("without dictionary: need = %v, %v; want false", need, err)
	}
	seedCedict(t, s, "吃", "en", "to eat")
	if need, err := s.LibraryNeedsPrefill(ctx); err != nil || !need {
		t.Errorf("with dictionary, empty library: need = %v, %v; want true", need, err)
	}
	if _, err := s.RefreshLibrary(ctx); err != nil {
		t.Fatal(err)
	}
	if need, err := s.LibraryNeedsPrefill(ctx); err != nil || need {
		t.Errorf("after prefill: need = %v, %v; want false", need, err)
	}
}

func TestLibraryWordsHaveNoProgress(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	eat := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat"}})
	seedCedict(t, s, "喝", "en", "to drink")
	drink := seedBareLibraryWord(t, s, "喝", "hē")
	if _, err := s.RefreshLibrary(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM sm2_progress p JOIN words w ON w.id = p.word_id WHERE w.user_id = 1`).Scan(&n)
	if n != 0 {
		t.Errorf("library has %d progress rows (words %d, %d), want 0", n, eat, drink)
	}
}
