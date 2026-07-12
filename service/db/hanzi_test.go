package db

import (
	"context"
	"testing"
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
