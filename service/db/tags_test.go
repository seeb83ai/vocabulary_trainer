package db

import (
	"context"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

func TestGetAllTags(t *testing.T) {
	s := openTestDB(t)
	seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"B-tag", "A-tag"})
	tags, err := s.GetAllTags(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0] != "A-tag" || tags[1] != "B-tag" {
		t.Errorf("expected [A-tag, B-tag], got %v", tags)
	}
}

func TestGetAllTags_UserIsolation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	// User 2 owns words with tags (user 2 is created by openTestDB)
	seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"user2-tag"})
	// Create user 3 and give them their own word+tag
	user3ID, err := s.CreateUser(ctx, "user3@example.com", "hash", "tok-u3", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWord(ctx, user3ID, models.CreateWordRequest{
		ZhText: "再见", Translations: map[string][]string{"en": {"goodbye"}}, Tags: []string{"user3-tag"},
	}); err != nil {
		t.Fatal(err)
	}
	tags2, err := s.GetAllTags(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(tags2) != 1 || tags2[0] != "user2-tag" {
		t.Errorf("user 2 should only see user2-tag, got %v", tags2)
	}
	tags3, err := s.GetAllTags(ctx, user3ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags3) != 1 || tags3[0] != "user3-tag" {
		t.Errorf("user 3 should only see user3-tag, got %v", tags3)
	}
}

func TestGetTagDetails_Empty(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	tags, err := s.GetTagDetails(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetTagDetails: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

func TestUpsertTagMeta_AndGetTagDetails(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed a word with a tag so the tag appears in GetTagDetails.
	if _, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "测试",
		Translations: map[string][]string{"en": {"test"}},
		Tags:         []string{"hsk1"},
	}); err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	// Initially description is empty, importable defaults to true.
	tags, err := s.GetTagDetails(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetTagDetails: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].Name != "hsk1" {
		t.Errorf("expected name hsk1, got %q", tags[0].Name)
	}
	if tags[0].Description != "" {
		t.Errorf("expected empty description, got %q", tags[0].Description)
	}
	if !tags[0].Importable {
		t.Errorf("expected importable=true by default")
	}

	// Update meta.
	if err := s.UpsertTagMeta(ctx, int64(2), "hsk1", "HSK level 1 vocabulary", false); err != nil {
		t.Fatalf("UpsertTagMeta: %v", err)
	}

	tags, err = s.GetTagDetails(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetTagDetails after upsert: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].Description != "HSK level 1 vocabulary" {
		t.Errorf("expected updated description, got %q", tags[0].Description)
	}
	if tags[0].Importable {
		t.Errorf("expected importable=false after update")
	}
}

func TestGetImportableSourceTags_FiltersImportable(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed two tags for user 1 (source/library user).
	for _, tag := range []string{"hsk1", "hsk2"} {
		if _, err := s.CreateWord(ctx, int64(1), models.CreateWordRequest{
			ZhText:       tag + "字",
			Translations: map[string][]string{"en": {tag + " word"}},
			Tags:         []string{tag},
		}); err != nil {
			t.Fatalf("CreateWord %s: %v", tag, err)
		}
	}

	// Mark hsk2 as not importable.
	if err := s.UpsertTagMeta(ctx, int64(1), "hsk2", "", false); err != nil {
		t.Fatalf("UpsertTagMeta: %v", err)
	}

	tags, err := s.GetImportableSourceTags(ctx, int64(1))
	if err != nil {
		t.Fatalf("GetImportableSourceTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 importable tag, got %d", len(tags))
	}
	if tags[0].Name != "hsk1" {
		t.Errorf("expected hsk1, got %q", tags[0].Name)
	}
}

func TestGetImportableSourceTags_AvailableLangs(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if _, err := s.CreateWord(ctx, int64(1), models.CreateWordRequest{
		ZhText:       "你好",
		Translations: map[string][]string{"en": {"hello"}, "de": {"hallo"}},
		Tags:         []string{"greetings"},
	}); err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	tags, err := s.GetImportableSourceTags(ctx, int64(1))
	if err != nil {
		t.Fatalf("GetImportableSourceTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	langs := map[string]bool{}
	for _, l := range tags[0].AvailableLangs {
		langs[l] = true
	}
	if !langs["en"] {
		t.Error("expected 'en' in available_langs")
	}
	if !langs["de"] {
		t.Error("expected 'de' in available_langs")
	}
}

func TestGetImportableSourceTags_WithDescription(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if _, err := s.CreateWord(ctx, int64(1), models.CreateWordRequest{
		ZhText:       "你好",
		Translations: map[string][]string{"en": {"hello"}},
		Tags:         []string{"greetings"},
	}); err != nil {
		t.Fatalf("CreateWord: %v", err)
	}
	if err := s.UpsertTagMeta(ctx, int64(1), "greetings", "Basic greeting words", true); err != nil {
		t.Fatalf("UpsertTagMeta: %v", err)
	}

	tags, err := s.GetImportableSourceTags(ctx, int64(1))
	if err != nil {
		t.Fatalf("GetImportableSourceTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Description != "Basic greeting words" {
		t.Errorf("expected description 'Basic greeting words', got %+v", tags)
	}
}
