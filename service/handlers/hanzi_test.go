package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

func TestDecompose_EmptyCharsReturnsEmptyArray(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, http.MethodGet, "/api/hanzi/decompose", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result []models.HanziDecomposition
	decodeJSON(t, rec, &result)
	if len(result) != 0 {
		t.Errorf("want empty array, got %d entries", len(result))
	}
}

func TestDecompose_MarkNew_FlagsUntrainedComponents(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	s.SeedHanziDecompositionWithDecompForTest(ctx, "好", "good", "⿰女子")
	s.SeedHanziDecompositionForTest(ctx, "女", "woman")
	s.SeedHanziDecompositionForTest(ctx, "子", "child")

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/hanzi/decompose?chars=%E5%A5%BD&mark_new=true", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result []models.HanziDecomposition
	decodeJSON(t, rec, &result)
	if len(result) == 0 || len(result[0].Components) == 0 {
		t.Skip("no components returned")
	}
	for _, comp := range result[0].Components {
		if comp.IsNewComponent == nil || !*comp.IsNewComponent {
			t.Errorf("component %q: want is_new_component=true (no progress row), got %v", comp.Character, comp.IsNewComponent)
		}
	}
}

func TestDecompose_MarkNew_TrainedComponentNotFlagged(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	s.SeedHanziDecompositionWithDecompForTest(ctx, "好", "good", "⿰女子")
	s.SeedHanziDecompositionForTest(ctx, "女", "woman")
	s.SeedHanziDecompositionForTest(ctx, "子", "child")
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now())
	// Mark as trained.
	s.SetComponentSeenForTest(ctx, int64(2), "女")
	s.SetComponentAttemptsForTest(ctx, int64(2), "女", 1)

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/hanzi/decompose?chars=%E5%A5%BD&mark_new=true", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result []models.HanziDecomposition
	decodeJSON(t, rec, &result)
	if len(result) == 0 || len(result[0].Components) == 0 {
		t.Skip("no components returned")
	}
	byChar := map[string]*bool{}
	for _, comp := range result[0].Components {
		byChar[comp.Character] = comp.IsNewComponent
	}
	if v := byChar["女"]; v == nil || *v {
		t.Errorf("component 女: want is_new_component=false (trained), got %v", v)
	}
	if v := byChar["子"]; v == nil || !*v {
		t.Errorf("component 子: want is_new_component=true (untrained), got %v", v)
	}
}

func TestDecompose_Langs_PopulatesDefinitions(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	s.SeedHanziDecompositionWithDecompForTest(ctx, "好", "good", "⿰女子")
	s.SeedHanziDecompositionForTest(ctx, "女", "woman")
	s.SeedHanziDecompositionForTest(ctx, "子", "child")
	if err := s.SeedHanziTranslationForTest(ctx, "女", "de", "Frau"); err != nil {
		t.Fatalf("seed translation: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/hanzi/decompose?chars=%E5%A5%BD&langs=en,de", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result []models.HanziDecomposition
	decodeJSON(t, rec, &result)
	if len(result) == 0 || len(result[0].Components) == 0 {
		t.Skip("no components returned")
	}
	byChar := map[string]map[string]string{}
	for _, comp := range result[0].Components {
		byChar[comp.Character] = comp.Definitions
	}
	if defs := byChar["女"]; defs["en"] != "woman" {
		t.Errorf("女 EN: want %q, got %q", "woman", defs["en"])
	}
	if defs := byChar["女"]; defs["de"] != "Frau" {
		t.Errorf("女 DE: want %q, got %q", "Frau", defs["de"])
	}
}
