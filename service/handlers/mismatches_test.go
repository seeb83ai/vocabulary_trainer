package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"vocabulary_trainer/models"
)

func TestMismatches_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/mismatches", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var items []map[string]any
	decodeJSON(t, rec, &items)
	if len(items) != 0 {
		t.Errorf("expected empty list, got %d items", len(items))
	}
}

func TestMismatches_RecordedOnWrongAnswer(t *testing.T) {
	s := openTestDB(t)
	xieID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	bookID := seedWord(t, s, "书", "shū", []string{"Buch"})
	markWordTrained(t, s, bookID)

	r := newRouter(s)

	// Answer 鞋 with "Buch" (which belongs to 书)
	rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": xieID,
		"mode":    "zh_to_transl",
		"answer":  "Buch",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("answer: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["correct"] != false {
		t.Error("expected incorrect answer")
	}
	if resp["confused_with"] == nil {
		t.Error("expected confused_with to be populated")
	}

	// Mismatches list should now have one entry
	rec2 := do(t, r, "GET", "/api/mismatches", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("mismatches: want 200, got %d", rec2.Code)
	}
	var items []map[string]any
	decodeJSON(t, rec2, &items)
	if len(items) != 1 {
		t.Fatalf("want 1 mismatch, got %d", len(items))
	}
	if items[0]["count"].(float64) != 1 {
		t.Errorf("count: want 1, got %v", items[0]["count"])
	}
}

func TestMismatches_NoConfusionWhenAnswerUnknown(t *testing.T) {
	s := openTestDB(t)
	xieID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	r := newRouter(s)

	// "Tisch" is not in the vocabulary — wrong but not a known confusion
	rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": xieID,
		"mode":    "zh_to_transl",
		"answer":  "Tisch",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["correct"] != false {
		t.Error("expected incorrect answer")
	}
	if resp["confused_with"] != nil {
		t.Error("confused_with should be absent when answer is not a known word")
	}

	// No confusion row recorded
	rec2 := do(t, r, "GET", "/api/mismatches", nil)
	var items []map[string]any
	decodeJSON(t, rec2, &items)
	if len(items) != 0 {
		t.Errorf("want 0 mismatches, got %d", len(items))
	}
}

func TestMismatches_NoConfusionOnCorrectAnswer(t *testing.T) {
	s := openTestDB(t)
	xieID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": xieID,
		"mode":    "zh_to_transl",
		"answer":  "Schuh",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["correct"] != true {
		t.Error("expected correct answer")
	}
	if resp["confused_with"] != nil {
		t.Error("confused_with must not be set on correct answers")
	}

	rec2 := do(t, r, "GET", "/api/mismatches", nil)
	var items []map[string]any
	decodeJSON(t, rec2, &items)
	if len(items) != 0 {
		t.Errorf("correct answer should record no confusion, got %d", len(items))
	}
}

func TestMismatches_EnToZh_Recorded(t *testing.T) {
	s := openTestDB(t)
	buchwID := seedWord(t, s, "书", "shū", []string{"Buch"})
	fiveID := seedWord(t, s, "五", "wǔ", []string{"five"})
	markWordTrained(t, s, fiveID)
	r := newRouter(s)

	// Given prompt "Buch" (en_to_zh), user types "五" instead of "书"
	rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": buchwID,
		"mode":    "transl_to_zh",
		"answer":  "五",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["correct"] != false {
		t.Error("expected incorrect answer")
	}
	if resp["confused_with"] == nil {
		t.Error("expected confused_with to be set")
	}

	rec2 := do(t, r, "GET", "/api/mismatches", nil)
	var items []map[string]any
	decodeJSON(t, rec2, &items)
	if len(items) != 1 {
		t.Fatalf("want 1 mismatch, got %d", len(items))
	}
}

// TestMismatches_ConfusedWithTranslations_CapsAndCollapses is a regression
// test: the "belongs to" mismatch box on the answer-result screen rendered
// the confused-with word's full, unfiltered translation list, ignoring
// max_translations_shown entirely (reported against issue #431/#432/#433's
// fix — the same cap must apply here too).
func TestMismatches_ConfusedWithTranslations_CapsAndCollapses(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	xieID := seedWord(t, s, "鞋", "xié", []string{"shoe"})
	bookTranslations := []string{"book", "volume", "text", "letter", "document", "register"}
	bookID := seedWord(t, s, "书", "shū", bookTranslations)
	markWordTrained(t, s, bookID)

	st, err := s.GetUserSettings(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	st.TranslationRankingEnabled = true
	st.MaxTranslationsShown = 4
	if err := s.UpdateUserSettings(ctx, int64(2), *st); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": xieID, "mode": "zh_to_transl", "answer": bookTranslations[0],
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("answer: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Correct {
		t.Fatal("expected incorrect answer")
	}
	if resp.ConfusedWith == nil {
		t.Fatal("expected confused_with to be populated")
	}
	shown := resp.ConfusedWith.ConfusedWithTranslations["en"]
	if len(shown) != 4 {
		t.Errorf("want exactly 4 confused-with translations shown (the configured cap), got %d: %v", len(shown), shown)
	}
	extra := resp.ConfusedWith.ConfusedWithTranslationsExtra["en"]
	if len(extra) != 2 {
		t.Errorf("want the remaining 2 confused-with translations collapsed into extra, got %d: %v", len(extra), extra)
	}
}

func TestMismatches_CountIncrementsOnRepeat(t *testing.T) {
	s := openTestDB(t)
	xieID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	bookID := seedWord(t, s, "书", "shū", []string{"Buch"})
	markWordTrained(t, s, bookID)
	r := newRouter(s)

	for i := 0; i < 3; i++ {
		rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
			"word_id": xieID,
			"mode":    "zh_to_transl",
			"answer":  "Buch",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("iteration %d: want 200, got %d", i, rec.Code)
		}
	}

	rec := do(t, r, "GET", "/api/mismatches", nil)
	var items []map[string]any
	decodeJSON(t, rec, &items)
	if len(items) != 1 {
		t.Fatalf("want 1 mismatch row, got %d", len(items))
	}
	if items[0]["count"].(float64) != 3 {
		t.Errorf("count: want 3, got %v", items[0]["count"])
	}
}
