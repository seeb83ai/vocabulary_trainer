package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

func TestDetectConfusion_ZhToEn_Found(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	seedWord(t, s, "书", "shū", []string{"Buch"})

	confusedWithID, found, err := s.DetectConfusion(context.Background(), int64(2), zhID, "Buch", "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found")
	}
	if confusedWithID == zhID {
		t.Error("confused_with_id must differ from zh_word_id")
	}
}

// TestDetectConfusion_Behaviour consolidates the core contract: detection fires
// for a shared translation belonging to a *different* entry, returns false for
// the entry's own (correct) translation, and false for an unknown answer.
func TestDetectConfusion_Behaviour(t *testing.T) {
	s := openTestDB(t)
	shoeID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	bookID := seedWord(t, s, "书", "shū", []string{"Buch"})

	// Different entry's translation → confusion with that entry.
	confusedWith, found, err := s.DetectConfusion(context.Background(), int64(2), shoeID, "Buch", "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion for a different entry's translation")
	}
	if confusedWith != bookID {
		t.Errorf("confused_with = %d, want the book entry %d", confusedWith, bookID)
	}

	// The entry's own correct translation → not a confusion.
	if _, found, err := s.DetectConfusion(context.Background(), int64(2), shoeID, "Schuh", "zh_to_transl", []string{"en"}); err != nil {
		t.Fatal(err)
	} else if found {
		t.Error("the correct translation must not be reported as confusion")
	}

	// Unknown answer → not a confusion.
	if _, found, err := s.DetectConfusion(context.Background(), int64(2), shoeID, "Tisch", "zh_to_transl", []string{"en"}); err != nil {
		t.Fatal(err)
	} else if found {
		t.Error("an unknown answer must not be reported as confusion")
	}
}

func TestDetectConfusion_ZhToEn_NoMatch(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})

	_, found, err := s.DetectConfusion(context.Background(), int64(2), zhID, "Tisch", "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected no confusion for unknown word")
	}
}

func TestDetectConfusion_EnToZh_Found(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "书", "shū", []string{"Buch"})
	zhID := seedWord(t, s, "五", "", []string{"five"})

	confusedWithID, found, err := s.DetectConfusion(context.Background(), int64(2), zhID, "书", "transl_to_zh", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found")
	}
	if confusedWithID == zhID {
		t.Error("confused_with_id must differ from zh_word_id")
	}
}

func TestDetectConfusion_SameWord_NotFound(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})

	_, found, err := s.DetectConfusion(context.Background(), int64(2), zhID, "Schuh", "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("should not report confusion when answer matches the tested word")
	}
}

// TestDetectConfusion_ZhToTranslVariants_Found ensures confusion detection also
// fires for the zh_to_transl variant modes (no-sound / voice-only prompt),
// which are scored identically to zh_to_transl in Answer but were previously
// missing from DetectConfusion's mode switch (issue #333).
func TestDetectConfusion_ZhToTranslVariants_Found(t *testing.T) {
	for _, mode := range []string{models.ModeZhToTranslNoSound, models.ModeVoiceToTransl} {
		t.Run(mode, func(t *testing.T) {
			s := openTestDB(t)
			shoeID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
			bookID := seedWord(t, s, "书", "shū", []string{"Buch"})

			confusedWith, found, err := s.DetectConfusion(context.Background(), int64(2), shoeID, "Buch", mode, []string{"en"})
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatalf("mode %q: expected confusion for a different entry's translation", mode)
			}
			if confusedWith != bookID {
				t.Errorf("mode %q: confused_with = %d, want the book entry %d", mode, confusedWith, bookID)
			}
		})
	}
}

func TestUpsertConfusion_IncrementsCount(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), int64(2), idA, idB, "zh_to_transl"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertConfusion(context.Background(), int64(2), idA, idB, "zh_to_transl"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 confusion, got %d", len(items))
	}
	if items[0].Count != 2 {
		t.Errorf("count: want 2, got %d", items[0].Count)
	}
}

func TestGetConfusions_LastSeenUpdated(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	before := time.Now().UTC().Add(-time.Second)
	if err := s.UpsertConfusion(context.Background(), int64(2), idA, idB, "zh_to_transl"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one confusion")
	}
	if items[0].LastSeen.Before(before) {
		t.Errorf("last_seen should be recent, got %v", items[0].LastSeen)
	}
}

func TestDetectConfusion_ZhPinyinToEn_Found(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	seedWord(t, s, "书", "shū", []string{"Buch"})

	confusedWithID, found, err := s.DetectConfusion(context.Background(), int64(2), zhID, "Buch", "zh_pinyin_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("zh_pinyin_to_en should behave like zh_to_en")
	}
	if confusedWithID == zhID {
		t.Error("confused_with_id must differ from zh_word_id")
	}
}

func TestDetectConfusion_InvalidMode_NotFound(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	seedWord(t, s, "书", "shū", []string{"Buch"})

	_, found, err := s.DetectConfusion(context.Background(), int64(2), zhID, "Buch", "invalid_mode", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("invalid mode should never report a confusion")
	}
}

func TestDetectConfusion_EmptyAnswer_NotFound(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})

	_, found, err := s.DetectConfusion(context.Background(), int64(2), zhID, "", "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("empty answer should never match")
	}
}

func TestGetConfusions_PopulatesEnTexts(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), int64(2), idA, idB, "zh_to_transl"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	d := items[0]
	if len(d.ZhTranslations["en"]) == 0 || d.ZhTranslations["en"][0] != "Schuh" {
		t.Errorf("ZhTranslations[en]: want [Schuh], got %v", d.ZhTranslations["en"])
	}
	if len(d.ConfusedWithTranslations["en"]) == 0 || d.ConfusedWithTranslations["en"][0] != "Buch" {
		t.Errorf("ConfusedWithTranslations[en]: want [Buch], got %v", d.ConfusedWithTranslations["en"])
	}
}

// TestResolveConfusionEntity_WordOwnedByDifferentUser_NotFound is a regression
// test for the defense-in-depth user_id guard on resolveConfusionEntity's word
// branch (PR #281 review round 1, finding #3): even though every real caller
// only ever passes a wordID sourced from a confusion_pairs row already
// filtered by user_id, the helper itself must refuse to resolve (and thus
// leak text/pinyin for) a word owned by a different user.
func TestResolveConfusionEntity_WordOwnedByDifferentUser_NotFound(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	otherUsersWordID := seedWord(t, s, "秘密", "mìmì", []string{"secret"}) // owned by user 2

	_, _, _, _, ok, err := s.resolveConfusionEntity(ctx, int64(1), otherUsersWordID, "", []string{"en"})
	if err != nil {
		t.Fatalf("resolveConfusionEntity: %v", err)
	}
	if ok {
		t.Error("resolveConfusionEntity resolved a word owned by a different user; want ok=false")
	}
}

// TestGetConfusions_DropsRowReferencingAnotherUsersWord verifies the same
// guard end-to-end through GetConfusions: a confusion_pairs row for user 1
// that (incorrectly) references words owned by user 2 must be silently
// skipped rather than leaking user 2's word text into user 1's response.
func TestGetConfusions_DropsRowReferencingAnotherUsersWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	idA := seedWord(t, s, "秘密", "mìmì", []string{"secret"})    // owned by user 2
	idB := seedWord(t, s, "危险", "wēixiǎn", []string{"danger"}) // owned by user 2

	if err := s.upsertConfusion(ctx, int64(1), idA, "", idB, "", "zh_to_transl"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(ctx, int64(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("want the cross-user row silently dropped, got %d items: %+v", len(items), items)
	}
}

func TestGetConfusionDetail_ReturnsRow(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), int64(2), idA, idB, "zh_to_transl"); err != nil {
		t.Fatal(err)
	}

	d, err := s.GetConfusionDetail(context.Background(), int64(2), idA, idB, "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("expected a ConfusionDetail, got nil")
	}
	if d.ZhText != "鞋" {
		t.Errorf("ZhText: want 鞋, got %q", d.ZhText)
	}
	if d.ConfusedWithText != "书" {
		t.Errorf("ConfusedWithText: want 书, got %q", d.ConfusedWithText)
	}
	if d.Count != 1 {
		t.Errorf("Count: want 1, got %d", d.Count)
	}
}

func TestGetConfusionDetail_MissingReturnsNil(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	d, err := s.GetConfusionDetail(context.Background(), int64(2), idA, idB, "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if d != nil {
		t.Error("expected nil when no confusion row exists")
	}
}

func TestGetConfusionDetail_ReturnsTranslationsForSelectedLangs(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	idA, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "人",
		Pinyin:       "rén",
		Translations: map[string][]string{"en": {"person"}, "de": {"Person"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	idB, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "点",
		Pinyin:       "diǎn",
		Translations: map[string][]string{"en": {"dot"}, "de": {"Uhr"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertConfusion(ctx, int64(2), idA, idB, "zh_to_transl"); err != nil {
		t.Fatal(err)
	}

	d, err := s.GetConfusionDetail(ctx, int64(2), idA, idB, "zh_to_transl", []string{"en", "de"})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("expected a ConfusionDetail, got nil")
	}

	zhAll := append(d.ZhTranslations["en"], d.ZhTranslations["de"]...)
	wantZh := []string{"person", "Person"}
	if len(zhAll) != len(wantZh) {
		t.Errorf("ZhTranslations: want %v, got %v", wantZh, zhAll)
	}
	cwAll := append(d.ConfusedWithTranslations["en"], d.ConfusedWithTranslations["de"]...)
	wantCW := []string{"dot", "Uhr"}
	if len(cwAll) != len(wantCW) {
		t.Errorf("ConfusedWithTranslations: want %v, got %v", wantCW, cwAll)
	}
}

func TestUpsertConfusion_DifferentModesSeparateRows(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), int64(2), idA, idB, "zh_to_transl"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertConfusion(context.Background(), int64(2), idA, idB, "zh_pinyin_to_transl"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("want 2 rows (one per mode), got %d", len(items))
	}
}

func TestDetectConfusion_ZhToEn_MatchesDeTranslation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// 人 → EN "person", DE "Person"
	targetID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "人",
		Pinyin:       "rén",
		Translations: map[string][]string{"en": {"person"}, "de": {"Person"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 点 → EN "dot", DE "Uhr"
	otherID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "点",
		Pinyin:       "diǎn",
		Translations: map[string][]string{"en": {"dot"}, "de": {"Uhr"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Typing "Uhr" (DE translation of 点) while answering for 人 should detect a confusion.
	confusedWithID, found, err := s.DetectConfusion(ctx, int64(2), targetID, "Uhr", "zh_to_transl", []string{"en", "de"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found for DE answer")
	}
	if confusedWithID != otherID {
		t.Errorf("expected confusedWithID=%d, got %d", otherID, confusedWithID)
	}
}

func TestDetectConfusion_ZhToEn_MatchesEnTranslation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	targetID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "狗",
		Pinyin:       "gǒu",
		Translations: map[string][]string{"en": {"dog"}, "de": {"Hund"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	otherID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "好",
		Pinyin:       "hǎo",
		Translations: map[string][]string{"en": {"good"}, "de": {"gut"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	confusedWithID, found, err := s.DetectConfusion(ctx, int64(2), targetID, "good", "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found for EN answer")
	}
	if confusedWithID != otherID {
		t.Errorf("expected confusedWithID=%d, got %d", otherID, confusedWithID)
	}
}

func TestDetectConfusion_ZhToEn_DeMatchedEvenWhenLangIsEnOnly(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	targetID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "人",
		Pinyin:       "rén",
		Translations: map[string][]string{"en": {"person"}, "de": {"Person"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	otherID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "点",
		Pinyin:       "diǎn",
		Translations: map[string][]string{"en": {"dot"}, "de": {"Uhr"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mismatch detection is language-agnostic: typing "Uhr" (DE translation of 点)
	// should detect a confusion even when langs=["en"] only.
	confusedWithID, found, err := s.DetectConfusion(ctx, int64(2), targetID, "Uhr", "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("expected DE answer to detect confusion even when quiz lang is EN-only")
	}
	if found && confusedWithID != otherID {
		t.Errorf("expected confusedWithID=%d (点), got %d", otherID, confusedWithID)
	}
}

func TestDetectConfusion_UmlautTranslation_Found(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// 练习 → DE "Übung" (umlaut in stored text)
	targetID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "练习",
		Pinyin:       "liànxí",
		Translations: map[string][]string{"de": {"trainieren"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	otherID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "练",
		Pinyin:       "liàn",
		Translations: map[string][]string{"de": {"Übung"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// User types "übung" (lowercase) while answering for 练习; should detect 练 as confusion.
	confusedWithID, found, err := s.DetectConfusion(ctx, int64(2), targetID, "übung", "zh_to_transl", []string{"de"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found for umlaut translation")
	}
	if confusedWithID != otherID {
		t.Errorf("expected confusedWithID=%d, got %d", otherID, confusedWithID)
	}
}

func TestDetectConfusion_SlashVariant_Found(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// 吃 → EN "eat"
	targetID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "吃",
		Pinyin:       "chī",
		Translations: map[string][]string{"en": {"eat"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 食物 → EN "food / eat" (slash-separated variant)
	otherID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "食物",
		Pinyin:       "shíwù",
		Translations: map[string][]string{"en": {"food / nourishment"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// User types "nourishment" while answering for 吃; should detect 食物 as confusion.
	confusedWithID, found, err := s.DetectConfusion(ctx, int64(2), targetID, "nourishment", "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found for slash-variant translation")
	}
	if confusedWithID != otherID {
		t.Errorf("expected confusedWithID=%d, got %d", otherID, confusedWithID)
	}
}

// TestDetectConfusion_ZhToTransl_DeOnlyWord mirrors the user scenario:
// prompt = 天 (zh_to_transl, DE only), user types "Spaziergang" which is the
// DE translation of 走. The quiz lang is "en" (default), because transl_to_zh
// found no EN translations and fell back to zh_to_transl — but mismatch
// detection must still find the DE translation of the other word.
func TestDetectConfusion_ZhToTransl_DeOnlyWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// 天 → DE "Himmel" only (no EN translation)
	tianID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "天",
		Pinyin:       "tiān",
		Translations: map[string][]string{"de": {"Himmel"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 走 → DE "Spaziergang" only (no EN translation)
	zouID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "走",
		Pinyin:       "zǒu",
		Translations: map[string][]string{"de": {"Spaziergang"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// User typed "Spaziergang" but langs=["en"] (default — mode fell back to zh_to_transl).
	// Mismatch detection must search across ALL languages, not just ["en"].
	confusedWithID, found, err := s.DetectConfusion(ctx, int64(2), tianID, "Spaziergang", "zh_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found: Spaziergang is a DE translation of 走, mismatch detection must be language-agnostic")
	}
	if confusedWithID != zouID {
		t.Errorf("expected confusedWithID=%d (走), got %d", zouID, confusedWithID)
	}
}

func TestDetectConfusion_TranslToZh_TranslationOfOtherWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// 天 → DE "Himmel" (the word being quizzed in transl_to_zh mode)
	targetID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "天",
		Pinyin:       "tiān",
		Translations: map[string][]string{"de": {"Himmel"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 走 → DE "Spaziergang" (a different word whose translation the user typed)
	otherID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "走",
		Pinyin:       "zǒu",
		Translations: map[string][]string{"de": {"Spaziergang"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// User types "spaziergang" while in transl_to_zh mode for 天; should detect 走.
	confusedWithID, found, err := s.DetectConfusion(ctx, int64(2), targetID, "spaziergang", "transl_to_zh", []string{"de"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found when user types a translation of another word in transl_to_zh mode")
	}
	if confusedWithID != otherID {
		t.Errorf("expected confusedWithID=%d, got %d", otherID, confusedWithID)
	}
}

func TestDetectConfusion_TranslToZh_SlashVariant(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// 天 → EN "sky"
	targetID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "天",
		Pinyin:       "tiān",
		Translations: map[string][]string{"en": {"sky"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 食物 → EN "food / nourishment"
	otherID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "食物",
		Pinyin:       "shíwù",
		Translations: map[string][]string{"en": {"food / nourishment"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// User types "nourishment" in transl_to_zh mode for 天; should detect 食物.
	confusedWithID, found, err := s.DetectConfusion(ctx, int64(2), targetID, "nourishment", "transl_to_zh", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found for slash-variant translation in transl_to_zh mode")
	}
	if confusedWithID != otherID {
		t.Errorf("expected confusedWithID=%d, got %d", otherID, confusedWithID)
	}
}

func TestGetRecentMismatches_Empty(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	since := time.Now().UTC().AddDate(0, 0, -7)
	items, err := s.GetRecentMismatches(ctx, int64(2), since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestGetRecentMismatches_ReturnsOnlyRecent(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})

	// recent confusion (1 day ago)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen)
		VALUES (2, ?, ?, 'zh_to_transl', 1, datetime('now', '-1 day'))`, id1, id2); err != nil {
		t.Fatal(err)
	}
	// old confusion (10 days ago — outside 7-day window)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen)
		VALUES (2, ?, ?, 'transl_to_zh', 1, datetime('now', '-10 days'))`, id2, id1); err != nil {
		t.Fatal(err)
	}

	since := time.Now().UTC().AddDate(0, 0, -7)
	items, err := s.GetRecentMismatches(ctx, int64(2), since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if items[0].ZhWordID != id1 {
		t.Errorf("expected zh_word_id=%d, got %d", id1, items[0].ZhWordID)
	}
}

func TestGetRecentMismatches_Limit(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})
	id3 := seedWord(t, s, "谢谢", "xiè xie", []string{"thank you"})

	for _, pair := range [][2]int64{{id1, id2}, {id2, id3}, {id3, id1}} {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen)
			VALUES (2, ?, ?, 'zh_to_transl', 1, datetime('now'))`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}

	since := time.Now().UTC().AddDate(0, 0, -7)
	items, err := s.GetRecentMismatches(ctx, int64(2), since, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items (limit), got %d", len(items))
	}
}

func TestGetRecentMismatches_HydratesTranslations(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen)
		VALUES (2, ?, ?, 'zh_to_transl', 1, datetime('now'))`, id1, id2); err != nil {
		t.Fatal(err)
	}

	since := time.Now().UTC().AddDate(0, 0, -7)
	items, err := s.GetRecentMismatches(ctx, int64(2), since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if len(items[0].ZhTranslations["en"]) == 0 {
		t.Error("expected ZhTranslations to be hydrated")
	}
	if len(items[0].ConfusedWithTranslations["en"]) == 0 {
		t.Error("expected ConfusedWithTranslations to be hydrated")
	}
}

func TestMarkConfusionsShownInGame_FiltersSubsequentCalls(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen)
		VALUES (2, ?, ?, 'zh_to_transl', 1, datetime('now'))`, id1, id2); err != nil {
		t.Fatal(err)
	}

	since := time.Now().UTC().AddDate(0, 0, -7)

	// First call returns the pair
	items, err := s.GetRecentMismatches(ctx, int64(2), since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("before mark: expected 1 item, got %d", len(items))
	}

	// Mark as shown
	if err := s.MarkConfusionsShownInGame(ctx, []ConfusionPairKey{{UserID: 2, ZhWordID: id1, ConfusedWithID: id2, Mode: "zh_to_transl"}}); err != nil {
		t.Fatal(err)
	}

	// Second call should return nothing (pair not re-confused since shown)
	items2, err := s.GetRecentMismatches(ctx, int64(2), since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items2) != 0 {
		t.Errorf("after mark: expected 0 items, got %d", len(items2))
	}
}

func TestMarkConfusionsShownInGame_ReappearsAfterNewConfusion(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen)
		VALUES (2, ?, ?, 'zh_to_transl', 1, datetime('now', '-1 hour'))`, id1, id2); err != nil {
		t.Fatal(err)
	}

	// Mark as shown (simulating game shown 30 min ago)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE confusion_pairs SET last_shown_in_game = datetime('now', '-30 minutes')
		WHERE zh_word_id = ? AND confused_with_id = ?`, id1, id2); err != nil {
		t.Fatal(err)
	}

	since := time.Now().UTC().AddDate(0, 0, -7)

	// Not visible yet (last_seen is before last_shown_in_game is not the case here — last_seen is 1 hr ago, shown 30 min ago)
	items, err := s.GetRecentMismatches(ctx, int64(2), since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("before re-confusion: expected 0 items, got %d", len(items))
	}

	// Simulate user confusing them again (last_seen updated to now)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE confusion_pairs SET last_seen = datetime('now'), count = 2
		WHERE zh_word_id = ? AND confused_with_id = ?`, id1, id2); err != nil {
		t.Fatal(err)
	}

	// Now visible again
	items2, err := s.GetRecentMismatches(ctx, int64(2), since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items2) != 1 {
		t.Errorf("after re-confusion: expected 1 item, got %d", len(items2))
	}
}

// TestMarkConfusionsShownInGame_DoesNotAffectOtherUsersRow is a regression test
// for PR #281 review round 2, finding #1: MarkConfusionsShownInGame's UPDATE
// previously had no user_id predicate. zh_component/confused_with_component
// are shared reference-data strings (hanzi characters), not per-user entities
// like word ids, so two different users can independently confuse the exact
// same component pair and end up with two distinct confusion_pairs rows
// (correctly distinguished only by user_id). Marking user 1's row as shown
// must not stamp last_shown_in_game on user 2's row for the same pair.
func TestMarkConfusionsShownInGame_DoesNotAffectOtherUsersRow(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Both users independently confuse the same component pair 扑 vs 打.
	if err := s.UpsertComponentConfusion(ctx, int64(1), "扑", 0, "打", models.ModeZhPinyinToTransl); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertComponentConfusion(ctx, int64(2), "扑", 0, "打", models.ModeZhPinyinToTransl); err != nil {
		t.Fatal(err)
	}

	// User 1's match-game session marks the pair as shown.
	if err := s.MarkConfusionsShownInGame(ctx, []ConfusionPairKey{
		{UserID: 1, ZhComponent: "扑", ConfusedWithComponent: "打", Mode: models.ModeZhPinyinToTransl},
	}); err != nil {
		t.Fatal(err)
	}

	var user1Shown, user2Shown sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT last_shown_in_game FROM confusion_pairs WHERE user_id = 1 AND zh_component = '扑' AND confused_with_component = '打'`,
	).Scan(&user1Shown); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT last_shown_in_game FROM confusion_pairs WHERE user_id = 2 AND zh_component = '扑' AND confused_with_component = '打'`,
	).Scan(&user2Shown); err != nil {
		t.Fatal(err)
	}
	if !user1Shown.Valid {
		t.Error("expected user 1's row to have last_shown_in_game set")
	}
	if user2Shown.Valid {
		t.Errorf("user 2's row was marked shown by user 1's match-game call; want last_shown_in_game still NULL, got %q", user2Shown.String)
	}
}

// TestMarkConfusionsShownInGame_DoesNotAffectOtherModesRow is a regression
// test for PR #281 review round 2, finding #2 (nit): MarkConfusionsShownInGame
// previously had no mode predicate, so marking a pair shown for one mode also
// marked any other-mode row referencing the same word ids as shown. Verifies
// that marking a pair shown under one mode leaves a same-user, same-word-pair
// row under a different mode untouched.
func TestMarkConfusionsShownInGame_DoesNotAffectOtherModesRow(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})

	if err := s.UpsertConfusion(ctx, int64(2), id1, id2, "zh_to_transl"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertConfusion(ctx, int64(2), id1, id2, "transl_to_zh"); err != nil {
		t.Fatal(err)
	}

	if err := s.MarkConfusionsShownInGame(ctx, []ConfusionPairKey{
		{UserID: 2, ZhWordID: id1, ConfusedWithID: id2, Mode: "zh_to_transl"},
	}); err != nil {
		t.Fatal(err)
	}

	var shownZhToTransl, shownTranslToZh sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT last_shown_in_game FROM confusion_pairs WHERE user_id = 2 AND zh_word_id = ? AND confused_with_id = ? AND mode = 'zh_to_transl'`,
		id1, id2,
	).Scan(&shownZhToTransl); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT last_shown_in_game FROM confusion_pairs WHERE user_id = 2 AND zh_word_id = ? AND confused_with_id = ? AND mode = 'transl_to_zh'`,
		id1, id2,
	).Scan(&shownTranslToZh); err != nil {
		t.Fatal(err)
	}
	if !shownZhToTransl.Valid {
		t.Error("expected zh_to_transl row to have last_shown_in_game set")
	}
	if shownTranslToZh.Valid {
		t.Errorf("transl_to_zh row was marked shown by a zh_to_transl-only call; want last_shown_in_game still NULL, got %q", shownTranslToZh.String)
	}
}

// TestDetectConfusion_QueriesDriveFromUsersWords locks in the query shape
// fix: the zh_to_transl and transl_to_zh confusion lookups must drive from
// the user's own zh words (an indexed seek on words(user_id, language) —
// see idx_words_user_language) rather than filtering the whole translations
// table with a "!=" predicate, which can't use an index and previously
// scanned every user's translation rows on every wrong answer. It runs
// EXPLAIN QUERY PLAN against the exact query strings DetectConfusion uses,
// so a future change back to the old shape fails this test.
func TestDetectConfusion_QueriesDriveFromUsersWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	assertUsesWordsUserLanguageIndex := func(t *testing.T, query string, args ...any) {
		t.Helper()
		rows, err := s.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, args...)
		if err != nil {
			t.Fatalf("explain query plan: %v", err)
		}
		defer rows.Close()
		var sawIndexedWords bool
		for rows.Next() {
			var id, parent, notUsed int
			var detail string
			if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
				t.Fatalf("scan explain row: %v", err)
			}
			t.Logf("plan: %s", detail)
			if strings.Contains(detail, "wz") && strings.Contains(detail, "idx_words_user_language") {
				sawIndexedWords = true
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("explain rows: %v", err)
		}
		if !sawIndexedWords {
			t.Error("want the query to seek the user's zh words via idx_words_user_language, got a different plan")
		}
	}

	assertUsesWordsUserLanguageIndex(t, zhToTranslConfusionQuery, int64(2), int64(1))
	assertUsesWordsUserLanguageIndex(t, translToZhConfusionQuery("?"), int64(2), "en", int64(1))
}
