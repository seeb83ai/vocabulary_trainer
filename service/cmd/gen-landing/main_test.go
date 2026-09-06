package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRenderTeasers_ZeroImages_TextOnlyCard(t *testing.T) {
	dir := t.TempDir()
	teaser := filepath.Join(dir, "10-vendor-lock-in")
	if err := os.Mkdir(teaser, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(teaser, "teaser.json"), `{
		"category": "Your data",
		"title": "No vendor lock-in",
		"summary": "Export everything, any time."
	}`)

	html, err := renderTeasers(dir)
	if err != nil {
		t.Fatalf("renderTeasers: %v", err)
	}
	if strings.Contains(html, "<img") {
		t.Error("a teaser with zero images must not render an <img>")
	}
	if strings.Contains(html, "data-carousel") {
		t.Error("a teaser with zero images must not render carousel machinery")
	}
	for _, want := range []string{"Your data", "No vendor lock-in", "Export everything, any time."} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderTeasers_OneImage_Standalone(t *testing.T) {
	dir := t.TempDir()
	teaser := filepath.Join(dir, "20-pinyin")
	if err := os.Mkdir(teaser, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(teaser, "teaser.json"), `{
		"category": "Pinyin drills",
		"title": "Train your ear",
		"summary": "Hear a syllable and identify it.",
		"imageAlt": ["Pinyin listening quiz"]
	}`)
	writeFile(t, filepath.Join(teaser, "01-quiz.png"), "fake-png-bytes")

	html, err := renderTeasers(dir)
	if err != nil {
		t.Fatalf("renderTeasers: %v", err)
	}
	if strings.Count(html, "<img") != 1 {
		t.Errorf("want exactly 1 <img>, got %d\n%s", strings.Count(html, "<img"), html)
	}
	if strings.Contains(html, "carousel-dot") {
		t.Error("a single-image teaser must not render carousel dots")
	}
	if !strings.Contains(html, `src="/landing/teasers/20-pinyin/01-quiz.png"`) {
		t.Error("image src must point at the served static path")
	}
	if !strings.Contains(html, `alt="Pinyin listening quiz"`) {
		t.Error("alt text from teaser.json imageAlt must be used")
	}
}

func TestRenderTeasers_TwoImages_Carousel(t *testing.T) {
	dir := t.TempDir()
	teaser := filepath.Join(dir, "10-training")
	if err := os.Mkdir(teaser, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(teaser, "teaser.json"), `{
		"category": "Training",
		"title": "One card at a time",
		"summary": "Scheduled by SM-2.",
		"imageAlt": ["Question card", "Filter chips"]
	}`)
	writeFile(t, filepath.Join(teaser, "01-question.png"), "a")
	writeFile(t, filepath.Join(teaser, "02-filters.png"), "b")

	html, err := renderTeasers(dir)
	if err != nil {
		t.Fatalf("renderTeasers: %v", err)
	}
	if !strings.Contains(html, "data-carousel") {
		t.Error("2+ images must render carousel machinery")
	}
	if strings.Count(html, "<img") != 2 {
		t.Errorf("want 2 <img>, got %d", strings.Count(html, "<img"))
	}
	if strings.Count(html, "carousel-dot") != 2 {
		t.Errorf("want 2 carousel dots, got %d", strings.Count(html, "carousel-dot"))
	}
	if !strings.Contains(html, `alt="Question card"`) || !strings.Contains(html, `alt="Filter chips"`) {
		t.Error("both alt texts must appear, in image order")
	}
	// First slide must be pre-marked active so it's visible with JS disabled.
	firstImgPos := strings.Index(html, "01-question.png")
	activePos := strings.Index(html, "opacity-100")
	if firstImgPos == -1 || activePos == -1 || activePos > strings.Index(html, "02-filters.png") {
		t.Error("first slide should be the one marked opacity-100 (active)")
	}
}

func TestRenderTeasers_OrdersByDirectoryName(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"20-second", "10-first", "30-third"} {
		d := filepath.Join(dir, name)
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(d, "teaser.json"), `{"category":"C","title":"`+name+`","summary":"s"}`)
	}

	html, err := renderTeasers(dir)
	if err != nil {
		t.Fatalf("renderTeasers: %v", err)
	}
	first := strings.Index(html, "10-first")
	second := strings.Index(html, "20-second")
	third := strings.Index(html, "30-third")
	if !(first < second && second < third) {
		t.Errorf("teasers must be ordered by directory name; positions: %d, %d, %d", first, second, third)
	}
}

func TestRenderTeasers_MissingTeaserJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	teaser := filepath.Join(dir, "10-broken")
	if err := os.Mkdir(teaser, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(teaser, "01-shot.png"), "x")

	_, err := renderTeasers(dir)
	if err == nil {
		t.Fatal("want an error for a teaser folder with no teaser.json")
	}
	if !strings.Contains(err.Error(), "10-broken") {
		t.Errorf("error should name the offending folder, got: %v", err)
	}
}

func TestRenderTeasers_EmitsI18nKeysStrippedOfOrderPrefix(t *testing.T) {
	dir := t.TempDir()
	teaser := filepath.Join(dir, "70-vendor-lock-in")
	if err := os.Mkdir(teaser, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(teaser, "teaser.json"), `{
		"category": "Your data",
		"title": "No vendor lock-in",
		"summary": "Export everything, any time."
	}`)

	html, err := renderTeasers(dir)
	if err != nil {
		t.Fatalf("renderTeasers: %v", err)
	}
	for _, want := range []string{
		`data-i18n="landing.teaser.vendor-lock-in.category"`,
		`data-i18n="landing.teaser.vendor-lock-in.title"`,
		`data-i18n="landing.teaser.vendor-lock-in.summary"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing %q\n%s", want, html)
		}
	}
}

func TestI18nSlug_StripsNumericOrderPrefix(t *testing.T) {
	cases := map[string]string{
		"10-training":      "training",
		"120-self-hosted":  "self-hosted",
		"no-prefix-at-all": "no-prefix-at-all",
		"10":               "10",
	}
	for in, want := range cases {
		if got := i18nSlug(in); got != want {
			t.Errorf("i18nSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSpliceMarkers_ReplacesRegionBetweenMarkers(t *testing.T) {
	doc := "before\n<!-- LANDING-TEASERS:START -->\nold content\n<!-- LANDING-TEASERS:END -->\nafter"
	got, err := spliceMarkers(doc, "new content")
	if err != nil {
		t.Fatalf("spliceMarkers: %v", err)
	}
	want := "before\n<!-- LANDING-TEASERS:START -->\nnew content\n<!-- LANDING-TEASERS:END -->\nafter"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSpliceMarkers_MissingMarkers_ReturnsError(t *testing.T) {
	_, err := spliceMarkers("no markers here", "x")
	if err == nil {
		t.Fatal("want an error when markers are missing")
	}
}

func TestSpliceMarkers_Idempotent(t *testing.T) {
	doc := "before\n<!-- LANDING-TEASERS:START -->\nold\n<!-- LANDING-TEASERS:END -->\nafter"
	once, err := spliceMarkers(doc, "generated")
	if err != nil {
		t.Fatal(err)
	}
	twice, err := spliceMarkers(once, "generated")
	if err != nil {
		t.Fatal(err)
	}
	if once != twice {
		t.Errorf("running splice twice with the same input should be a no-op; got:\n%s\nthen:\n%s", once, twice)
	}
}
