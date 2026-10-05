package db

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"vocabulary_trainer/models"
)

func dictionaryRows(t *testing.T, s *Store, lang string) []string {
	t.Helper()
	rows, err := s.db.Query(`SELECT simplified || '=' || definition || '|' || source FROM cedict_entries WHERE lang = ?`, lang)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		rows.Scan(&v)
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func TestImportDictionaryEntries_ReplaceKeepsUserEntriesAndOtherLanguage(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedCedict(t, s, "吃", "en", "to eat; to consume")
	seedCedict(t, s, "喝", "en", "to drink")
	seedCedict(t, s, "吃", "de", "essen")
	// A translation the old curated library had, kept as a user entry.
	if _, err := s.db.Exec(`INSERT INTO cedict_entries (simplified, lang, pinyin, definition, source) VALUES ('吃', 'en', 'chī', 'to munch', 'user')`); err != nil {
		t.Fatal(err)
	}

	report, err := s.ImportDictionaryEntries(ctx, "en", []models.DictionaryEntry{
		{Simplified: "吃", Pinyin: "chī", Definition: "to eat; to dine"},
		{Simplified: "饭", Pinyin: "fàn", Definition: "rice"},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Inserted != 2 || report.Removed != 2 {
		t.Errorf("report = %+v, want 2 inserted, 2 removed", report)
	}
	want := []string{"吃=to eat; to dine|dict", "吃=to munch|user", "饭=rice|dict"}
	if got := dictionaryRows(t, s, "en"); !reflect.DeepEqual(got, want) {
		t.Errorf("en = %v, want %v", got, want)
	}
	if got := dictionaryRows(t, s, "de"); !reflect.DeepEqual(got, []string{"吃=essen|dict"}) {
		t.Errorf("de = %v, want it unchanged", got)
	}
}

func TestImportDictionaryEntries_AppendKeepsExistingEntries(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedCedict(t, s, "吃", "en", "to eat")
	if _, err := s.ImportDictionaryEntries(ctx, "en", []models.DictionaryEntry{
		{Simplified: "喝", Pinyin: "hē", Definition: "to drink"},
	}, false); err != nil {
		t.Fatal(err)
	}
	if got := dictionaryRows(t, s, "en"); !reflect.DeepEqual(got, []string{"吃=to eat|dict", "喝=to drink|dict"}) {
		t.Errorf("en = %v", got)
	}
}
