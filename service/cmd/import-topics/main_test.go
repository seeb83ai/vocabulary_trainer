package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	vocabdb "vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

func openStore(t *testing.T) *vocabdb.Store {
	t.Helper()
	// Migrations seed the admin and library users from these.
	t.Setenv("ADMIN_EMAIL", "admin@example.de")
	t.Setenv("ADMIN_PASSWORD", "I am the admin")
	t.Setenv("USER_EMAIL", "me@example.de")
	t.Setenv("USER_PASSWORD", "I learn zh")
	t.Setenv("BCRYPT_COST", "min")
	store, err := vocabdb.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestImportTopics(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)

	// 护照 is already a library word; 签证 and 米饭 are not.
	if _, err := store.CreateWord(ctx, libraryUserID, models.CreateWordRequest{ZhText: "护照", Tags: []string{"hsk2-3"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedCedictEntryForTest(ctx, "签证", "en", "qiān zhèng", "visa"); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	files := map[string]string{
		"travel.csv": "zh,en\n护照,passport\n签证,visa\n",
		"food.csv":   "zh,en\n米饭,cooked rice\n护照,passport\n",
		"_none.csv":  "zh,en\n的,of\n",
		"notes.txt":  "ignored",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := importTopics(ctx, store, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.created != 2 || res.tagged != 4 || res.topics != 2 {
		t.Errorf("first run: %+v, want created=2 tagged=4 topics=2", res)
	}

	words, _, err := store.GetWords(ctx, libraryUserID, "", 1, 0, "", "", nil, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	byZh := map[string]models.WordDetail{}
	for _, w := range words {
		byZh[w.ZhText] = w
	}
	if _, ok := byZh["的"]; ok {
		t.Error("_none.csv words must not be imported")
	}
	for zh, want := range map[string][]string{
		"护照": {"hsk2-3", "topic-food", "topic-travel"},
		"签证": {"topic-travel"},
		"米饭": {"topic-food"},
	} {
		got := slices.Clone(byZh[zh].Tags)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s tags = %v, want %v", zh, got, want)
		}
	}
	if p := byZh["签证"].Pinyin; p == nil || *p != "qiānzhèng" {
		t.Errorf("签证 pinyin = %v, want qiānzhèng (CEDICT, spaces removed)", p)
	}

	tags, err := store.GetImportableSourceTags(ctx, libraryUserID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tg := range tags {
		if tg.Name == "topic-travel" && tg.Description == "" {
			t.Error("topic-travel has no description")
		}
		names = append(names, tg.Name)
	}
	for _, want := range []string{"topic-food", "topic-travel"} {
		if !slices.Contains(names, want) {
			t.Errorf("importable tags %v missing %s", names, want)
		}
	}

	// A second run creates nothing new.
	res, err = importTopics(ctx, store, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.created != 0 || res.tagged != 4 {
		t.Errorf("second run: %+v, want created=0 tagged=4", res)
	}
}

func TestImportTopics_DryRunWritesNothing(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "travel.csv"), []byte("zh,en\n签证,visa\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := importTopics(ctx, store, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.created != 1 {
		t.Errorf("dry run: %+v, want created=1", res)
	}
	words, _, err := store.GetWords(ctx, libraryUserID, "", 1, 0, "", "", nil, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range words {
		if w.ZhText == "签证" {
			t.Error("dry run created a word")
		}
	}
}
