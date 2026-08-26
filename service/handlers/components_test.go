package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

func TestComponentAnswer_CorrectAnswer(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "女", "woman; female"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Insert component directly — the handler test is about answer checking, not InitComponentsForWord.
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "女",
		"answer":    "woman",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if correct, _ := resp["correct"].(bool); !correct {
		t.Errorf("want correct=true")
	}
}

func TestComponentAnswer_WrongAnswer(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "女", "woman; female"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "女",
		"answer":    "man",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if correct, _ := resp["correct"].(bool); correct {
		t.Errorf("want correct=false")
	}
}

// TestComponentAnswer_WrongAnswer_TriggersMismatch covers issue #280: a wrong
// component answer that happens to be the translation of a different word
// must be reported as a mismatch (confused_with), not just marked wrong.
func TestComponentAnswer_WrongAnswer_TriggersMismatch(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "扑", "to rap, to tap; script; to let go"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "扑", time.Now().Add(-time.Hour))
	wordID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText: "去", Pinyin: "qù", Translations: map[string][]string{"en": {"to go"}},
	})
	if err != nil {
		t.Fatalf("seed word: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "扑",
		"answer":    "To go",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.ComponentAnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Correct {
		t.Fatal("want correct=false")
	}
	if resp.ConfusedWith == nil {
		t.Fatal("want confused_with to be populated")
	}
	if resp.ConfusedWith.ZhKind != models.ConfusionKindComponent || resp.ConfusedWith.ZhComponent != "扑" {
		t.Errorf("zh side: got kind=%s component=%q", resp.ConfusedWith.ZhKind, resp.ConfusedWith.ZhComponent)
	}
	if resp.ConfusedWith.ConfusedWithKind != models.ConfusionKindWord || resp.ConfusedWith.ConfusedWithID != wordID {
		t.Errorf("confused_with side: got kind=%s id=%d (want word id=%d)", resp.ConfusedWith.ConfusedWithKind, resp.ConfusedWith.ConfusedWithID, wordID)
	}
	if resp.ConfusedWith.Mode != models.ModeZhPinyinToTransl {
		t.Errorf("mode: want %s, got %s", models.ModeZhPinyinToTransl, resp.ConfusedWith.Mode)
	}

	// The pair must also now show up on the mismatches page.
	items, err := s.GetConfusions(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 tracked confusion, got %d", len(items))
	}
}

func TestComponentAnswer_WrongAnswer_NoMismatchWhenUnrelated(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman; female"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "女",
		"answer":    "completely unrelated",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.ComponentAnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.ConfusedWith != nil {
		t.Errorf("want confused_with nil, got %+v", resp.ConfusedWith)
	}
}

func TestComponentAnswer_WrongAnswer_IncludesTier(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "女",
		"answer":    "man",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["tier"] != "Struggling" {
		t.Errorf("want tier=Struggling on a wrong first attempt, got %v", resp["tier"])
	}
}

func TestComponentAnswer_AlternativeSemicolon(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "曰", "to speak; to say"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(context.Background(), int64(2), "曰", time.Now().Add(-time.Hour))

	for _, answer := range []string{"to speak", "to say"} {
		r := newRouter(s)
		rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
			"character": "曰",
			"answer":    answer,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("answer %q: want 200, got %d", answer, rec.Code)
		}
		var resp map[string]any
		decodeJSON(t, rec, &resp)
		if correct, _ := resp["correct"].(bool); !correct {
			t.Errorf("answer %q: want correct=true", answer)
		}
	}
}

// TestComponentAnswer_MixedCommaSemicolon verifies that a definition like
// "woman, girl; female" accepts any of the three single-word alternatives,
// not just the semicolon-split halves.
func TestComponentAnswer_MixedCommaSemicolon(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "女", "woman, girl; female"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", time.Now().Add(-time.Hour))

	for _, answer := range []string{"woman", "girl", "female"} {
		r := newRouter(s)
		rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
			"character": "女",
			"answer":    answer,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("answer %q: want 200, got %d", answer, rec.Code)
		}
		var resp map[string]any
		decodeJSON(t, rec, &resp)
		if correct, _ := resp["correct"].(bool); !correct {
			t.Errorf("answer %q: want correct=true", answer)
		}
	}
}

func TestComponentAnswer_NotFound(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "X",
		"answer":    "something",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestComponentAnswer_CorrectAnswersMapReturned(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "女",
		"answer":    "woman",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	answers, ok := resp["correct_answers"].(map[string]any)
	if !ok {
		t.Fatalf("want correct_answers map, got %T: %v", resp["correct_answers"], resp["correct_answers"])
	}
	if answers["en"] != "woman" {
		t.Errorf("want correct_answers[en]=woman, got %v", answers["en"])
	}
}

func TestComponentAnswer_DELangAccepted(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "女", "woman"); err != nil {
		t.Fatalf("seed EN: %v", err)
	}
	if err := s.SeedHanziTranslationForTest(context.Background(), "女", "de", "Frau"); err != nil {
		t.Fatalf("seed DE: %v", err)
	}
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	// Send DE lang — answer in German should be accepted.
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]any{
		"character": "女",
		"answer":    "Frau",
		"langs":     []string{"de"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if correct, _ := resp["correct"].(bool); !correct {
		t.Errorf("want correct=true for DE answer")
	}
	answers, ok := resp["correct_answers"].(map[string]any)
	if !ok {
		t.Fatalf("want correct_answers map, got %T", resp["correct_answers"])
	}
	if answers["de"] != "Frau" {
		t.Errorf("want correct_answers[de]=Frau, got %v", answers["de"])
	}
}

func TestComponentAnswer_TierOnFirstAttempt(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "女",
		"answer":    "woman",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["tier"] != "Struggling" {
		t.Errorf("want tier=Struggling on first attempt, got %v", resp["tier"])
	}
	if _, has := resp["prev_tier"]; has {
		t.Errorf("want no prev_tier on first-ever attempt, got %v", resp["prev_tier"])
	}
}

func TestComponentAnswer_TierChangeOnBoundaryCrossing(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", time.Now().Add(-time.Hour))
	// 9/9 correct = 100% accuracy but under the 10-attempt graduation floor,
	// so this sits in the "Learning" tier just below the Mastered boundary.
	s.SetComponentProgressForTest(context.Background(), int64(2), "女", 9, 9)

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "女",
		"answer":    "woman",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["tier"] != "Mastered" {
		t.Errorf("want tier=Mastered after 10th correct answer, got %v", resp["tier"])
	}
	if resp["prev_tier"] != "Learning" {
		t.Errorf("want prev_tier=Learning, got %v", resp["prev_tier"])
	}
}

func TestComponentStats_ReturnsEmptyDays(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/component/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	days, ok := resp["days"]
	if !ok {
		t.Fatal("want 'days' key in response")
	}
	if days == nil {
		t.Fatal("want non-nil days")
	}
}

func TestComponentDueDateDistribution_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodGet, "/api/component/due-date-distribution", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp models.DueDateDistributionResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Dates) != 0 {
		t.Errorf("expected empty dates, got %d", len(resp.Dates))
	}
}

func TestComponentDueDateDistribution_AfterSeen(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	past := time.Now().Add(-48 * time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)
	s.SetComponentSeenForTest(ctx, int64(2), "女")

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/component/due-date-distribution", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.DueDateDistributionResponse
	decodeJSON(t, rec, &resp)
	total := 0
	for _, d := range resp.Dates {
		total += d.Count
	}
	if total != 1 {
		t.Errorf("expected total count 1, got %d", total)
	}
}

func TestComponentDueDateDistribution_ExcludesUnseen(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	past := time.Now().Add(-48 * time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past) // unseen: first_seen_date IS NULL

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/component/due-date-distribution", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp models.DueDateDistributionResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Dates) != 0 {
		t.Errorf("expected unseen component excluded, got %d dates", len(resp.Dates))
	}
}

func TestComponentSeen_MarksFirstSeenDate(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/seen", map[string]string{"character": "女"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify component now counts toward due_today.
	rec2 := do(t, r, http.MethodGet, "/api/quiz/stats?trainComponents=1", nil)
	var stats map[string]any
	decodeJSON(t, rec2, &stats)
	if v, _ := stats["components_due_today"].(float64); int(v) != 1 {
		t.Errorf("want components_due_today=1 after seen, got %v", stats["components_due_today"])
	}
}

func TestComponentSeen_MissingCharacter(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodPost, "/api/component/seen", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestComponentSkip_DaysOne(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/skip", map[string]any{"character": "女", "days": 1})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	items, _, err := s.GetComponentList(ctx, int64(2), "", 1, 10, false)
	if err != nil {
		t.Fatalf("GetComponentList: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 component, got %d", len(items))
	}
	wantDate := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	if items[0].DueDate != wantDate {
		t.Errorf("days=1: want due_date=%s, got %s", wantDate, items[0].DueDate)
	}
}

func TestComponentSkip_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodPost, "/api/component/skip", map[string]any{"character": "不存在", "days": 1})
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestComponentSkip_MissingCharacter(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodPost, "/api/component/skip", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestComponentList_ReturnsComponents(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman; female"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/components", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if total, _ := resp["total"].(float64); int(total) != 1 {
		t.Errorf("want total=1, got %v", resp["total"])
	}
	items, _ := resp["components"].([]any)
	if len(items) != 1 {
		t.Fatalf("want 1 component, got %d", len(items))
	}
	item := items[0].(map[string]any)
	if item["character"] != "女" {
		t.Errorf("want character=女, got %v", item["character"])
	}
	if item["definition_en"] != "woman; female" {
		t.Errorf("want definition_en='woman; female', got %v", item["definition_en"])
	}
}

func TestComponentList_SearchFilter(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman; female"); err != nil {
		t.Fatalf("seed 女: %v", err)
	}
	if err := s.SeedHanziDecompositionForTest(ctx, "日", "sun; day"); err != nil {
		t.Fatalf("seed 日: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))
	s.InsertComponentProgressForTest(ctx, int64(2), "日", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/components?q=sun", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if total, _ := resp["total"].(float64); int(total) != 1 {
		t.Errorf("want total=1 for search 'sun', got %v", resp["total"])
	}
	items, _ := resp["components"].([]any)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].(map[string]any)["character"] != "日" {
		t.Errorf("want 日 in result, got %v", items[0])
	}
}

func TestComponentCoverage_ReturnsWordIDSets(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionWithDecompForTest(ctx, "明", "bright", "⿰日月"); err != nil {
		t.Fatalf("seed 明: %v", err)
	}
	if err := s.SeedHanziDecompositionForTest(ctx, "日", "sun; day"); err != nil {
		t.Fatalf("seed 日: %v", err)
	}
	if err := s.SeedHanziDecompositionForTest(ctx, "月", "moon; month"); err != nil {
		t.Fatalf("seed 月: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:       "明",
		Translations: map[string][]string{"en": {"bright"}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create word: want 201, got %d: %s", rec.Code, rec.Body)
	}

	rec2 := do(t, r, http.MethodGet, "/api/components/coverage", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec2, &resp)
	if tw, _ := resp["total_words"].(float64); int(tw) != 1 {
		t.Errorf("want total_words=1, got %v", resp["total_words"])
	}
	items, _ := resp["components"].([]any)
	if len(items) != 2 {
		t.Fatalf("want 2 components (日, 月), got %d: %v", len(items), items)
	}
	for _, raw := range items {
		item := raw.(map[string]any)
		wordIDs, _ := item["word_ids"].([]any)
		if len(wordIDs) != 1 {
			t.Errorf("want word_ids of length 1 for %v, got %v", item["character"], item["word_ids"])
		}
	}
}

func TestComponentCoverage_ReturnsTrainedCharacters(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionWithDecompForTest(ctx, "明", "bright", "⿰日月"); err != nil {
		t.Fatalf("seed 明: %v", err)
	}
	if err := s.SeedHanziDecompositionForTest(ctx, "日", "sun; day"); err != nil {
		t.Fatalf("seed 日: %v", err)
	}
	if err := s.SeedHanziDecompositionForTest(ctx, "月", "moon; month"); err != nil {
		t.Fatalf("seed 月: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:        "明",
		Translations:  map[string][]string{"en": {"bright"}},
		StartTraining: true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create word: want 201, got %d: %s", rec.Code, rec.Body)
	}
	// Default threshold is 0 (train every component), and start_training
	// initialises component_progress rows immediately, so both 日 and 月 are
	// already in training from word creation.

	rec2 := do(t, r, http.MethodGet, "/api/components/coverage", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec2, &resp)
	trained, _ := resp["trained_characters"].([]any)
	if len(trained) != 2 {
		t.Fatalf("want 2 trained characters (日, 月), got %d: %v", len(trained), trained)
	}
	got := map[string]bool{}
	for _, c := range trained {
		got[c.(string)] = true
	}
	if !got["日"] || !got["月"] {
		t.Errorf("want trained_characters = [日 月], got %v", trained)
	}
}

func TestComponentCoverage_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodGet, "/api/components/coverage", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	items, _ := resp["components"].([]any)
	if len(items) != 0 {
		t.Errorf("want empty components on fresh DB, got %d", len(items))
	}
}

func TestComponentReview_Sets204(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/components/女/review", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify via reviewOnly filter — only returns flagged components.
	items, total, err := s.GetComponentList(ctx, int64(2), "", 1, 20, true)
	if err != nil {
		t.Fatalf("GetComponentList: %v", err)
	}
	if total != 1 || len(items) == 0 || items[0].Character != "女" {
		t.Errorf("want 女 flagged, got total=%d items=%v", total, items)
	}
}

func TestComponentUpdateTranslation_Sets204(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "水", "water"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, http.MethodPut, "/api/components/水/translation", map[string]string{
		"lang":       "de",
		"definition": "Wasser",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body.String())
	}

	defs, err := s.GetComponentDefinitions(ctx, 2, "水", []string{"de"})
	if err != nil {
		t.Fatalf("GetComponentDefinitions: %v", err)
	}
	if defs["de"] != "Wasser" {
		t.Errorf("want de=Wasser, got %q", defs["de"])
	}
}

func TestComponentUpdateTranslation_MissingLang(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "水", "water"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, http.MethodPut, "/api/components/水/translation", map[string]string{
		"definition": "Wasser",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestComponentList_ReviewFilter(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.SeedHanziDecompositionForTest(ctx, "日", "sun"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	past := time.Now().Add(-time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)
	s.InsertComponentProgressForTest(ctx, int64(2), "日", past)
	if err := s.MarkComponentForReview(int64(2), "女"); err != nil {
		t.Fatalf("MarkComponentForReview: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/components?review=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Components []map[string]any `json:"components"`
		Total      int              `json:"total"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Total != 1 {
		t.Errorf("want total=1 with review filter, got %d", resp.Total)
	}
	if len(resp.Components) != 1 {
		t.Errorf("want 1 component, got %d", len(resp.Components))
	}
	if char, _ := resp.Components[0]["character"].(string); char != "女" {
		t.Errorf("want character=女, got %q", char)
	}
}

func TestComponentGetTranslations_ReturnsMap(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "水", "water"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.StoreComponentTranslation(context.Background(), 2, "水", "en", "water"); err != nil {
		t.Fatalf("store en: %v", err)
	}
	if err := s.StoreComponentTranslation(context.Background(), 2, "水", "de", "Wasser"); err != nil {
		t.Fatalf("store de: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/components/水/translations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if resp["en"] != "water" {
		t.Errorf("want en=water, got %q", resp["en"])
	}
	if resp["de"] != "Wasser" {
		t.Errorf("want de=Wasser, got %q", resp["de"])
	}
}

func TestComponentGetTranslations_EmptyForUnknown(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/components/X/translations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if len(resp) != 0 {
		t.Errorf("want empty map, got %v", resp)
	}
}

func TestComponentGetHMMScene_ReturnsSceneText(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.UpsertComponentHMMScene(ctx, int64(2), "木", "A wooden table"); err != nil {
		t.Fatalf("seed scene: %v", err)
	}
	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/components/木/hmm-scene", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if resp["scene_text"] != "A wooden table" {
		t.Errorf("want scene_text=%q, got %v", "A wooden table", resp["scene_text"])
	}
}

func TestComponentGetHMMScene_EmptyWhenNoScene(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/components/木/hmm-scene", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if resp["scene_text"] != "" {
		t.Errorf("want empty scene_text, got %q", resp["scene_text"])
	}
}

func TestComponentPutHMMScene_SavesAndReturns204(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, http.MethodPut, "/api/components/木/hmm-scene", map[string]string{
		"scene_text": "A wooden table",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body.String())
	}

	text, err := s.GetComponentHMMSceneText(context.Background(), int64(2), "木")
	if err != nil {
		t.Fatalf("GetComponentHMMSceneText: %v", err)
	}
	if text != "A wooden table" {
		t.Errorf("want %q, got %q", "A wooden table", text)
	}
}

func TestComponentPutHMMScene_MissingChar_Returns400(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, http.MethodPut, "/api/components//hmm-scene", map[string]string{
		"scene_text": "some text",
	})
	if rec.Code == http.StatusNoContent {
		t.Fatal("want non-204 for empty char path param")
	}
}

func TestComponentDeleteHMMScene_Removes(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.UpsertComponentHMMScene(ctx, int64(2), "水", "Water flows"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := newRouter(s)
	rec := do(t, r, http.MethodDelete, "/api/components/水/hmm-scene", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body.String())
	}

	text, err := s.GetComponentHMMSceneText(ctx, int64(2), "水")
	if err != nil {
		t.Fatalf("GetComponentHMMSceneText: %v", err)
	}
	if text != "" {
		t.Errorf("want empty after delete, got %q", text)
	}
}

func TestComponentAnswer_IncludesSceneText(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))
	if err := s.UpsertComponentHMMScene(ctx, int64(2), "女", "A woman in a park"); err != nil {
		t.Fatalf("seed scene: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "女",
		"answer":    "woman",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["scene_text"] != "A woman in a park" {
		t.Errorf("want scene_text=%q, got %v", "A woman in a park", resp["scene_text"])
	}
}

func TestComponentAnswer_NoSceneText_OmitsField(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/component/answer", map[string]string{
		"character": "女",
		"answer":    "woman",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if _, ok := resp["scene_text"]; ok {
		t.Errorf("want scene_text omitted when no scene exists, got %v", resp["scene_text"])
	}
}

func TestComponentGetHMMContext_ReturnsContext(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	// Seed character with pinyin so initial/final/tone can be parsed.
	if err := s.SeedHanziDecompositionWithPinyinForTest(ctx, "木", "tree", `["mù"]`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/components/木/hmm/context", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	// Must have the shape expected by loadCompHMMBuilder.
	for _, key := range []string{"initial", "final", "tone", "radicals", "radical_defs", "props"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("response missing key %q; got %v", key, resp)
		}
	}
}

func TestComponentGetHMMContext_MissingChar_Returns400(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodGet, "/api/components//hmm/context", nil)
	if rec.Code == http.StatusOK {
		t.Fatal("want non-200 for empty char path param")
	}
}

func TestComponentGetHMMContext_IncludesExistingScene(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "水", "water"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.UpsertComponentHMMScene(ctx, int64(2), "水", "Water flows down"); err != nil {
		t.Fatalf("seed scene: %v", err)
	}
	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/components/水/hmm/context", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	scene, ok := resp["scene"]
	if !ok {
		t.Fatalf("want scene key in response, got %v", resp)
	}
	sceneMap, ok := scene.(map[string]any)
	if !ok {
		t.Fatalf("want scene to be object, got %T", scene)
	}
	if sceneMap["scene_text"] != "Water flows down" {
		t.Errorf("want scene_text=%q, got %v", "Water flows down", sceneMap["scene_text"])
	}
}

func TestComponentSaveCompScene_SavesActorLocationRoom(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionWithPinyinForTest(ctx, "火", "fire", `["huǒ"]`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := newRouter(s)
	rec := do(t, r, http.MethodPut, "/api/components/火/hmm", map[string]any{
		"scene_text":    "Fire burns bright",
		"actor_name":    "Hugo",
		"location_name": "Harbor",
		"room_name":     "Hall",
		"props":         []any{},
		"decomposition": "",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Scene must be persisted.
	text, err := s.GetComponentHMMSceneText(ctx, int64(2), "火")
	if err != nil {
		t.Fatalf("GetComponentHMMSceneText: %v", err)
	}
	if text != "Fire burns bright" {
		t.Errorf("want scene_text=%q, got %q", "Fire burns bright", text)
	}
}

func TestComponentAcceptCorrect_NoState(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/component/accept-correct", map[string]string{"character": "女"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when no prev_state, got %d: %s", rec.Code, rec.Body)
	}
}

func TestComponentAcceptCorrect_RestoresProgress(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))
	// Mark as seen so it enters quiz rotation
	s.SetComponentSeenForTest(ctx, int64(2), "女")

	r := newRouter(s)
	// Submit a wrong answer — this saves prev_state and applies wrong quality.
	do(t, r, "POST", "/api/component/answer", map[string]string{
		"character": "女",
		"answer":    "man",
	})

	// Now accept as correct.
	rec := do(t, r, "POST", "/api/component/accept-correct", map[string]string{"character": "女"})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if correct, _ := resp["correct"].(bool); !correct {
		t.Error("accept-correct should return correct: true")
	}

	// prev_state should be cleared after accept.
	prev, err := s.GetComponentPrevState(ctx, int64(2), "女")
	if err != nil {
		t.Fatalf("GetComponentPrevState: %v", err)
	}
	if prev != nil {
		t.Errorf("prev_state should be nil after accept-correct, got %+v", prev)
	}

	// The SM-2 state after accept-correct must have interval >= 1 day (not the wrong-penalty value).
	p, _, err := s.GetComponentProgressForTest(ctx, int64(2), "女")
	if err != nil {
		t.Fatalf("GetComponentProgressForTest: %v", err)
	}
	if p.IntervalDays < 1 {
		t.Errorf("interval_days after accept-correct should be >= 1, got %d", p.IntervalDays)
	}
}

func TestComponentAcceptCorrect_MissingCharacter(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/component/accept-correct", map[string]string{"character": ""})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for empty character, got %d", rec.Code)
	}
}
