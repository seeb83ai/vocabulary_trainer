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

func TestCreateReferences_GivesLearnerLibraryWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	eat := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat"}})
	drink := seedLibraryWord(t, s, "喝", "hē", map[string][]string{"en": {"to drink"}})

	ids, err := s.CreateReferences(ctx, 2, []int64{eat, drink}, []string{"hsk3-1"})
	if err != nil {
		t.Fatalf("CreateReferences: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want 2", ids)
	}
	wd, err := s.GetWordByID(ctx, 2, ids[1])
	if err != nil || wd == nil {
		t.Fatalf("GetWordByID: %v, %v", wd, err)
	}
	if wd.ZhText != "喝" || wd.Pinyin == nil || *wd.Pinyin != "hē" {
		t.Errorf("word = %q / %v, want 喝 / hē", wd.ZhText, wd.Pinyin)
	}
	if !reflect.DeepEqual(wd.Translations["en"], []string{"to drink"}) {
		t.Errorf("glosses = %v", wd.Translations)
	}
	if !reflect.DeepEqual(wd.Tags, []string{"hsk3-1"}) {
		t.Errorf("tags = %v", wd.Tags)
	}
	if wd.TotalAttempts != 0 {
		t.Errorf("new reference is not unseen: %d attempts", wd.TotalAttempts)
	}
	// The import copies no glosses.
	var links int
	s.db.QueryRow(`SELECT COUNT(*) FROM translations t JOIN words w ON w.id = t.zh_word_id WHERE w.user_id = 2`).Scan(&links)
	if links != 0 {
		t.Errorf("learner has %d copied gloss links, want 0", links)
	}
}

func overridesUpdatedAt(t *testing.T, s *Store, id int64) string {
	t.Helper()
	var v *string
	if err := s.db.QueryRow(`SELECT overrides_updated_at FROM words WHERE id = ?`, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v == nil {
		return ""
	}
	return *v
}

func TestUpdateWord_ReferenceEditsBecomeOverrides(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	lib := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat", "to consume"}, "de": {"essen"}})
	other := newTestUser(t, s, "other@example.de")
	ids, err := s.CreateReferences(ctx, 2, []int64{lib}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := ids[0]
	otherIDs, _ := s.CreateReferences(ctx, other, []int64{lib}, nil)

	// The edit form sends what the learner sees, minus "to consume", plus "to munch".
	err = s.UpdateWord(ctx, 2, ref, models.UpdateWordRequest{
		ZhText: "吃", Pinyin: "chī1",
		Translations:       map[string][]string{"en": {"to eat", "to munch"}, "de": {"essen"}},
		TranslationSources: map[string][]string{"en": {"cedict", "user"}, "de": {"cedict"}},
	})
	if err != nil {
		t.Fatalf("UpdateWord: %v", err)
	}
	wd, _ := s.GetWordByID(ctx, 2, ref)
	if got := wd.Translations["en"]; !reflect.DeepEqual(got, []string{"to eat", "to munch"}) {
		t.Errorf("learner en = %v", got)
	}
	if wd.Pinyin == nil || *wd.Pinyin != "chī1" {
		t.Errorf("pinyin = %v, want chī1", wd.Pinyin)
	}
	if overridesUpdatedAt(t, s, ref) == "" {
		t.Error("overrides_updated_at not set")
	}
	owd, _ := s.GetWordByID(ctx, other, otherIDs[0])
	if got := owd.Translations["en"]; !reflect.DeepEqual(got, []string{"to consume", "to eat"}) {
		t.Errorf("other learner en = %v (must not change)", got)
	}
	if got := libraryGlosses(t, s, lib)["en"]; !reflect.DeepEqual(got, []string{"to consume", "to eat"}) {
		t.Errorf("library en = %v (must not change)", got)
	}
	var libraryLinksOnRef int
	s.db.QueryRow(`SELECT COUNT(*) FROM translations WHERE zh_word_id = ? AND source = 'cedict'`, ref).Scan(&libraryLinksOnRef)
	if libraryLinksOnRef != 0 {
		t.Errorf("library glosses were copied onto the reference: %d", libraryLinksOnRef)
	}
}

func TestUpdateWord_ReferenceUnchangedGlossesAreNoOverride(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	lib := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat"}})
	ids, _ := s.CreateReferences(ctx, 2, []int64{lib}, nil)

	err := s.UpdateWord(ctx, 2, ids[0], models.UpdateWordRequest{
		ZhText: "吃", Pinyin: "chī", Tags: []string{"mine"},
		Translations:       map[string][]string{"en": {"to eat"}},
		TranslationSources: map[string][]string{"en": {"cedict"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := overridesUpdatedAt(t, s, ids[0]); got != "" {
		t.Errorf("overrides_updated_at = %q after saving unchanged glosses", got)
	}
	wd, _ := s.GetWordByID(ctx, 2, ids[0])
	if !reflect.DeepEqual(wd.Tags, []string{"mine"}) {
		t.Errorf("tags = %v", wd.Tags)
	}
}

func TestUpdateWord_ReferenceNewTextBecomesOwnEntry(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	lib := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat"}})
	ids, _ := s.CreateReferences(ctx, 2, []int64{lib}, nil)

	err := s.UpdateWord(ctx, 2, ids[0], models.UpdateWordRequest{
		ZhText: "吃饭", Pinyin: "chīfàn",
		Translations: map[string][]string{"en": {"to have a meal"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(ctx, 2, ids[0])
	if wd.ZhText != "吃饭" || !reflect.DeepEqual(wd.Translations["en"], []string{"to have a meal"}) {
		t.Errorf("word = %q %v", wd.ZhText, wd.Translations)
	}
	var libID *int64
	s.db.QueryRow(`SELECT library_word_id FROM words WHERE id = ?`, ids[0]).Scan(&libID)
	if libID != nil {
		t.Errorf("still a reference to %d", *libID)
	}
}

func TestAddTranslation_Reference(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	lib := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat", "to consume"}})
	ids, _ := s.CreateReferences(ctx, 2, []int64{lib}, nil)
	ref := ids[0]
	if err := s.UpdateWord(ctx, 2, ref, models.UpdateWordRequest{ZhText: "吃",
		Translations:       map[string][]string{"en": {"to eat"}},
		TranslationSources: map[string][]string{"en": {"cedict"}}}); err != nil {
		t.Fatal(err)
	}

	// Re-adding a deleted library gloss restores it as a library gloss.
	if err := s.AddTranslation(ctx, 2, ref, "en", "to consume"); err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(ctx, 2, ref)
	if !reflect.DeepEqual(wd.TranslationSources["en"], []string{"cedict", "cedict"}) {
		t.Errorf("sources = %v %v, want the two library glosses", wd.Translations["en"], wd.TranslationSources["en"])
	}
	if got := overridesUpdatedAt(t, s, ref); got != "" {
		t.Errorf("overrides_updated_at = %q with no overrides left", got)
	}

	// A new gloss becomes the learner's own.
	if err := s.AddTranslation(ctx, 2, ref, "en", "to munch"); err != nil {
		t.Fatal(err)
	}
	wd, _ = s.GetWordByID(ctx, 2, ref)
	if !reflect.DeepEqual(wd.Translations["en"], []string{"to consume", "to eat", "to munch"}) {
		t.Errorf("en = %v", wd.Translations["en"])
	}
	if overridesUpdatedAt(t, s, ref) == "" {
		t.Error("overrides_updated_at not set")
	}
}

func TestDeleteWord_ReferenceLeavesTombstone(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	lib := seedLibraryWord(t, s, "吃", "chī", map[string][]string{"en": {"to eat"}})
	ids, _ := s.CreateReferences(ctx, 2, []int64{lib}, nil)

	if err := s.DeleteWord(ctx, 2, ids[0]); err != nil {
		t.Fatal(err)
	}
	stones, err := s.TombstonedLibraryWords(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !stones[lib] {
		t.Errorf("tombstones = %v, want library word %d", stones, lib)
	}
	if libraryGlosses(t, s, lib)["en"] == nil {
		t.Error("library word lost its glosses")
	}

	// Importing it again clears the tombstone.
	if _, err := s.CreateReferences(ctx, 2, []int64{lib}, nil); err != nil {
		t.Fatal(err)
	}
	stones, _ = s.TombstonedLibraryWords(ctx, 2)
	if stones[lib] {
		t.Error("tombstone kept after re-adding the word")
	}
}

func seedLibraryListWord(t *testing.T, s *Store, zh, gloss string, tags ...string) int64 {
	t.Helper()
	id, err := s.CreateWord(context.Background(), testLibraryUserID, models.CreateWordRequest{
		ZhText: zh, Tags: tags,
		Translations:       map[string][]string{"en": {gloss}},
		TranslationSources: map[string][]string{"en": {"cedict"}},
	})
	if err != nil {
		t.Fatalf("seed library list word %q: %v", zh, err)
	}
	return id
}

func finishedImport(t *testing.T, s *Store, userID int64, spec models.ImportJob) {
	t.Helper()
	ctx := context.Background()
	job, err := s.CreateImportJob(ctx, userID, spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishImportJob(ctx, job.ID, ""); err != nil {
		t.Fatal(err)
	}
}

func TestImportedLists_CountsNewAndRemovedWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a := seedLibraryListWord(t, s, "一", "one", "hsk3-1")
	b := seedLibraryListWord(t, s, "二", "two", "hsk3-1")
	seedLibraryListWord(t, s, "三", "three", "hsk3-2")
	finishedImport(t, s, 2, models.ImportJob{Tag: "hsk3-1", ApplyTags: []string{"hsk3-1"}})
	ids, err := s.CreateReferences(ctx, 2, []int64{a, b}, []string{"hsk3-1"})
	if err != nil {
		t.Fatal(err)
	}

	// The learner deletes 二; the library adds 四 to the list later.
	if err := s.DeleteWord(ctx, 2, ids[1]); err != nil {
		t.Fatal(err)
	}
	seedLibraryListWord(t, s, "四", "four", "hsk3-1")

	lists, err := s.ImportedLists(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []models.ImportedList{{Tag: "hsk3-1", AndTags: []string{}, ApplyTags: []string{"hsk3-1"}, New: 1, Removed: 1}}
	if !reflect.DeepEqual(lists, want) {
		t.Errorf("lists = %+v, want %+v", lists, want)
	}
}

// conflictSetup gives learner 2 an edited reference to 吃 in list hsk3-1
// (added "to munch"), edited before a dictionary update.
func conflictSetup(t *testing.T) (*Store, int64) {
	t.Helper()
	s := openTestDB(t)
	ctx := context.Background()
	eat := seedBareLibraryWord(t, s, "吃", "chī")
	if err := s.AddWordTags(ctx, testLibraryUserID, eat, []string{"hsk3-1"}); err != nil {
		t.Fatal(err)
	}
	seedCedict(t, s, "吃", "en", "to eat")
	if _, err := s.RefreshLibrary(ctx); err != nil {
		t.Fatal(err)
	}
	ids, err := s.CreateReferences(ctx, 2, []int64{eat}, []string{"hsk3-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddTranslation(ctx, 2, ids[0], "en", "to munch"); err != nil {
		t.Fatal(err)
	}
	s.db.Exec(`UPDATE words SET overrides_updated_at = '2000-01-01 00:00:00' WHERE id = ?`, ids[0])
	seedCedict(t, s, "吃", "en", "to dine")
	if _, err := s.RefreshLibrary(ctx); err != nil {
		t.Fatal(err)
	}
	return s, ids[0]
}

func TestLibraryConflicts_ListsEditedReferencesTheLibraryChanged(t *testing.T) {
	s, ref := conflictSetup(t)
	ctx := context.Background()

	conflicts, err := s.LibraryConflicts(ctx, 2, "hsk3-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []models.LibraryConflict{{
		WordID: ref, ZhText: "吃",
		Library: map[string][]string{"en": {"to dine", "to eat"}},
		Mine:    map[string][]string{"en": {"to dine", "to eat", "to munch"}},
	}}
	if !reflect.DeepEqual(conflicts, want) {
		t.Errorf("conflicts = %+v, want %+v", conflicts, want)
	}
}

func TestResolveLibraryConflicts(t *testing.T) {
	for _, keep := range []string{"mine", "library"} {
		t.Run(keep, func(t *testing.T) {
			s, ref := conflictSetup(t)
			ctx := context.Background()
			if err := s.ResolveLibraryConflicts(ctx, 2, []int64{ref}, keep); err != nil {
				t.Fatal(err)
			}
			conflicts, _ := s.LibraryConflicts(ctx, 2, "hsk3-1", nil)
			if len(conflicts) != 0 {
				t.Errorf("conflicts after resolve = %+v", conflicts)
			}
			wd, _ := s.GetWordByID(ctx, 2, ref)
			want := []string{"to dine", "to eat", "to munch"}
			if keep == "library" {
				want = []string{"to dine", "to eat"}
			}
			if !reflect.DeepEqual(wd.Translations["en"], want) {
				t.Errorf("en = %v, want %v", wd.Translations["en"], want)
			}
		})
	}
}

func TestImportedLists_CountsConflicts(t *testing.T) {
	s, _ := conflictSetup(t)
	finishedImport(t, s, 2, models.ImportJob{Tag: "hsk3-1", ApplyTags: []string{"hsk3-1"}})
	lists, err := s.ImportedLists(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].Conflicts != 1 {
		t.Errorf("lists = %+v, want 1 conflict", lists)
	}
}

func TestUpdateWord_ReferenceTypedGlossStaysLearnersOwn(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	lib := seedLibraryWord(t, s, "年", "nián", map[string][]string{"en": {"year"}})
	ids, _ := s.CreateReferences(ctx, 2, []int64{lib}, nil)

	// The learner types "year" themselves (source user) next to "annual".
	if err := s.UpdateWord(ctx, 2, ids[0], models.UpdateWordRequest{ZhText: "年",
		Translations:       map[string][]string{"en": {"year", "annual"}},
		TranslationSources: map[string][]string{"en": {"user", "user"}}}); err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(ctx, 2, ids[0])
	if !reflect.DeepEqual(wd.Translations["en"], []string{"annual", "year"}) ||
		!reflect.DeepEqual(wd.TranslationSources["en"], []string{"user", "user"}) {
		t.Errorf("en = %v %v, want both glosses as the learner's own", wd.Translations["en"], wd.TranslationSources["en"])
	}
}

func TestImportedLists_IncludesListsImportedBeforeImportJobs(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a := seedLibraryListWord(t, s, "一", "one", "hsk3-1")
	seedLibraryListWord(t, s, "二", "two", "hsk3-1")
	// Imported long ago: no import job, only the list tag on the word.
	if _, err := s.CreateReferences(ctx, 2, []int64{a}, []string{"hsk3-1", "mine"}); err != nil {
		t.Fatal(err)
	}

	lists, err := s.ImportedLists(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []models.ImportedList{{Tag: "hsk3-1", AndTags: []string{}, ApplyTags: []string{"hsk3-1"}, New: 1}}
	if !reflect.DeepEqual(lists, want) {
		t.Errorf("lists = %+v, want %+v", lists, want)
	}
}
