package handlers_test

import (
	"net/http"
	"testing"
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
	seedWord(t, s, "书", "shū", []string{"Buch"})

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
	seedWord(t, s, "五", "wǔ", []string{"five"})
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

func TestMismatches_CountIncrementsOnRepeat(t *testing.T) {
	s := openTestDB(t)
	xieID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	seedWord(t, s, "书", "shū", []string{"Buch"})
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
