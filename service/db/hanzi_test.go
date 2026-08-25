package db

import (
	"context"
	"testing"
	"time"
)

// TestGetHanziDecomposition_IsSemantic_Pictophonetic verifies that for a
// pictophonetic character the semantic component gets is_semantic=true and
// the phonetic-only component gets is_semantic=false.
func TestGetHanziDecomposition_IsSemantic_Pictophonetic(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	ety := `{"type":"pictophonetic","phonetic":"马","semantic":"女","hint":"mother"}`
	seedHanziFull(t, s, "妈", "mother", "⿰女马", ety, "女", "")
	seedHanziFull(t, s, "女", "woman; female", "", "", "", "")
	seedHanziFull(t, s, "马", "horse", "", "", "", "")

	results, err := s.GetHanziDecomposition(ctx, []rune("妈"))
	if err != nil {
		t.Fatalf("GetHanziDecomposition: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}

	parent := results[0]
	if parent.IsSemantic != nil {
		t.Errorf("top-level char should not have IsSemantic set, got %v", *parent.IsSemantic)
	}
	if len(parent.Components) != 2 {
		t.Fatalf("want 2 components, got %d", len(parent.Components))
	}

	byChar := map[string]bool{}
	for _, c := range parent.Components {
		if c.IsSemantic == nil {
			t.Errorf("component %q missing IsSemantic", c.Character)
			continue
		}
		byChar[c.Character] = *c.IsSemantic
	}

	if !byChar["女"] {
		t.Errorf("want 女 (semantic) to have is_semantic=true")
	}
	if byChar["马"] {
		t.Errorf("want 马 (phonetic-only) to have is_semantic=false")
	}
}

// TestGetHanziDecomposition_IsSemantic_Ideographic verifies that all
// components of an ideographic character get is_semantic=true.
func TestGetHanziDecomposition_IsSemantic_Ideographic(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	ety := `{"type":"ideographic","hint":"sun + moon = bright"}`
	seedHanziFull(t, s, "明", "bright", "⿰日月", ety, "日", "")
	seedHanziFull(t, s, "日", "sun; day", "", "", "", "")
	seedHanziFull(t, s, "月", "moon; month", "", "", "", "")

	results, err := s.GetHanziDecomposition(ctx, []rune("明"))
	if err != nil {
		t.Fatalf("GetHanziDecomposition: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}

	for _, c := range results[0].Components {
		if c.IsSemantic == nil {
			t.Errorf("component %q missing IsSemantic", c.Character)
			continue
		}
		if !*c.IsSemantic {
			t.Errorf("want component %q is_semantic=true for ideographic char, got false", c.Character)
		}
	}
}

// TestGetHanziDecomposition_IsSemantic_PinyinFallback verifies that without
// etymology, the pinyin similarity fallback marks the matching component
// as phonetic (is_semantic=false).
func TestGetHanziDecomposition_IsSemantic_PinyinFallback(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Parent 请 (qǐng) with no etymology. Component 青 (qīng) shares final →
	// phonetic. Component 讠 (yán) has different final → semantic.
	seedHanziFull(t, s, "请", "please; request", "⿰讠青", "", "讠", `["qǐng"]`)
	seedHanziFull(t, s, "青", "blue; green", "", "", "", `["qīng"]`)
	seedHanziFull(t, s, "讠", "speech radical", "", "", "", `["yán"]`)

	results, err := s.GetHanziDecomposition(ctx, []rune("请"))
	if err != nil {
		t.Fatalf("GetHanziDecomposition: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}

	byChar := map[string]bool{}
	for _, c := range results[0].Components {
		if c.IsSemantic == nil {
			t.Errorf("component %q missing IsSemantic", c.Character)
			continue
		}
		byChar[c.Character] = *c.IsSemantic
	}

	if byChar["青"] {
		t.Errorf("want 青 (pinyin-similar) to have is_semantic=false")
	}
	if !byChar["讠"] {
		t.Errorf("want 讠 (different pinyin) to have is_semantic=true")
	}
}

// TestGetHanziDecomposition_IsSemantic_TopLevelUnset verifies that top-level
// queried characters never have IsSemantic populated (it is only set on
// entries that are components of some parent).
func TestGetHanziDecomposition_IsSemantic_TopLevelUnset(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	seedHanziFull(t, s, "女", "woman; female", "", "", "", "")

	results, err := s.GetHanziDecomposition(ctx, []rune("女"))
	if err != nil {
		t.Fatalf("GetHanziDecomposition: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].IsSemantic != nil {
		t.Errorf("top-level char should not have IsSemantic set, got %v", *results[0].IsSemantic)
	}
}

func TestAnnotateComponentDefinitions_PopulatesENAndDE(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	s.SeedHanziDecompositionWithDecompForTest(ctx, "好", "good", "⿰女子")
	s.SeedHanziDecompositionForTest(ctx, "女", "woman")
	s.SeedHanziDecompositionForTest(ctx, "子", "child")
	if err := s.SeedHanziTranslationForTest(ctx, "女", "de", "Frau"); err != nil {
		t.Fatalf("seed DE translation: %v", err)
	}

	results, err := s.GetHanziDecomposition(ctx, []rune("好"))
	if err != nil {
		t.Fatalf("GetHanziDecomposition: %v", err)
	}
	if len(results) == 0 || len(results[0].Components) == 0 {
		t.Skip("no components — decomposition not seeded correctly")
	}

	if err := s.AnnotateComponentDefinitions(ctx, 2, results, []string{"en", "de"}); err != nil {
		t.Fatalf("AnnotateComponentDefinitions: %v", err)
	}

	byChar := map[string]map[string]string{}
	for _, comp := range results[0].Components {
		byChar[comp.Character] = comp.Definitions
	}
	if defs := byChar["女"]; defs["en"] != "woman" {
		t.Errorf("女 EN: want %q, got %q", "woman", defs["en"])
	}
	if defs := byChar["女"]; defs["de"] != "Frau" {
		t.Errorf("女 DE: want %q, got %q", "Frau", defs["de"])
	}
	if defs := byChar["子"]; defs["en"] != "child" {
		t.Errorf("子 EN: want %q, got %q", "child", defs["en"])
	}
}

func TestAnnotateComponentDefinitions_NoLangsIsNoop(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	s.SeedHanziDecompositionWithDecompForTest(ctx, "好", "good", "⿰女子")
	s.SeedHanziDecompositionForTest(ctx, "女", "woman")
	s.SeedHanziDecompositionForTest(ctx, "子", "child")

	results, err := s.GetHanziDecomposition(ctx, []rune("好"))
	if err != nil {
		t.Fatalf("GetHanziDecomposition: %v", err)
	}
	if err := s.AnnotateComponentDefinitions(ctx, 2, results, nil); err != nil {
		t.Fatalf("AnnotateComponentDefinitions: %v", err)
	}
	for _, comp := range results[0].Components {
		if comp.Definitions != nil {
			t.Errorf("component %q: expected nil Definitions with no langs, got %v", comp.Character, comp.Definitions)
		}
	}
}

func TestAnnotateNewComponents_MarksNewAndExisting(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// Seed decomposition data: 好 = 女 + 子
	s.SeedHanziDecompositionWithDecompForTest(ctx, "好", "good", "⿰女子")
	s.SeedHanziDecompositionForTest(ctx, "女", "woman")
	s.SeedHanziDecompositionForTest(ctx, "子", "child")

	results, err := s.GetHanziDecomposition(ctx, []rune("好"))
	if err != nil {
		t.Fatalf("GetHanziDecomposition: %v", err)
	}
	if len(results) == 0 || len(results[0].Components) == 0 {
		t.Skip("no components found — decomposition not seeded correctly")
	}

	// Before any component_progress row: both should be new.
	if err := s.AnnotateNewComponents(ctx, userID, results); err != nil {
		t.Fatalf("AnnotateNewComponents (before insert): %v", err)
	}
	for _, comp := range results[0].Components {
		if comp.IsNewComponent == nil || !*comp.IsNewComponent {
			t.Errorf("component %q: want is_new_component=true before progress row, got %v", comp.Character, comp.IsNewComponent)
		}
	}

	// Insert a progress row for 女 with total_attempts=0 (exists but never trained).
	s.InsertComponentProgressForTest(ctx, userID, "女", time.Now())

	results2, err := s.GetHanziDecomposition(ctx, []rune("好"))
	if err != nil {
		t.Fatalf("GetHanziDecomposition (2nd): %v", err)
	}
	if err := s.AnnotateNewComponents(ctx, userID, results2); err != nil {
		t.Fatalf("AnnotateNewComponents (untrained row): %v", err)
	}
	for _, comp := range results2[0].Components {
		if comp.IsNewComponent == nil || !*comp.IsNewComponent {
			t.Errorf("component %q: want is_new_component=true when total_attempts=0, got %v", comp.Character, comp.IsNewComponent)
		}
	}

	// Mark 女 as trained (total_attempts > 0).
	if _, err := s.db.ExecContext(ctx,
		`UPDATE component_progress SET total_attempts = 1 WHERE user_id = ? AND character = '女'`, userID,
	); err != nil {
		t.Fatalf("update total_attempts: %v", err)
	}

	results3, err := s.GetHanziDecomposition(ctx, []rune("好"))
	if err != nil {
		t.Fatalf("GetHanziDecomposition (3rd): %v", err)
	}
	if err := s.AnnotateNewComponents(ctx, userID, results3); err != nil {
		t.Fatalf("AnnotateNewComponents (trained row): %v", err)
	}
	byChar := map[string]*bool{}
	for _, comp := range results3[0].Components {
		byChar[comp.Character] = comp.IsNewComponent
	}
	if v := byChar["女"]; v == nil || *v {
		t.Errorf("component 女: want is_new_component=false after total_attempts=1, got %v", v)
	}
	if v := byChar["子"]; v == nil || !*v {
		t.Errorf("component 子: want is_new_component=true (no progress row), got %v", v)
	}
}
