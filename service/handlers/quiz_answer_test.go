package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

func TestQuizAnswer_Correct(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
		Answer: "hello",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("answer 'hello' should be correct")
	}
	if resp.TotalAttempts != 1 {
		t.Errorf("total_attempts: want 1, got %d", resp.TotalAttempts)
	}
	if resp.TotalCorrect != 1 {
		t.Errorf("total_correct: want 1, got %d", resp.TotalCorrect)
	}
}

func TestQuizAnswer_Wrong(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
		Answer: "wrong",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Correct {
		t.Error("answer 'wrong' should not be correct")
	}
	if resp.TotalCorrect != 0 {
		t.Errorf("total_correct: want 0, got %d", resp.TotalCorrect)
	}
}

func TestQuizAnswer_Wrong_IncludesTier(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
		Answer: "wrong",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Tier != "New" {
		t.Errorf("tier: want 'New' on a wrong answer for a fresh learning-phase word, got %q", resp.Tier)
	}
}

func TestQuizAnswer_EnToZh(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeTranslToZh,
		Answer: "你好",
	})
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("answer '你好' for en_to_zh should be correct")
	}
}

// Regression for issue #309/#311: a word stored with a fullwidth-paren POS
// annotation ("还（动词）") must accept the bare character as correct in
// transl_to_zh mode, just like an ASCII-paren annotation already does.
func TestQuizAnswer_TranslToZh_FullwidthParensAnnotationStripped(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "还（动词）", "huán", []string{"Return"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeTranslToZh,
		Answer: "还",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("'还' should be accepted as correct for '还（动词）' — fullwidth parens mark an optional segment")
	}
}

func TestQuizAnswer_TranslToZh_WrongKnownWordIncludesPinyin(t *testing.T) {
	s := openTestDB(t)
	correctID := seedWord(t, s, "看书", "kàn shū", []string{"to read"})
	_ = seedWord(t, s, "看数", "kàn shù", []string{"to count"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: correctID,
		Mode:   models.ModeTranslToZh,
		Answer: "看数",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Correct {
		t.Fatal("'看数' should be wrong for 看书")
	}
	if resp.UserAnswerPinyin == nil {
		t.Fatal("expected user_answer_pinyin to be set when wrong answer is in vocab")
	}
	if *resp.UserAnswerPinyin != "kàn shù" {
		t.Errorf("want user_answer_pinyin=%q, got %q", "kàn shù", *resp.UserAnswerPinyin)
	}
}

func TestQuizAnswer_TranslToZh_WrongUnknownWordNoPinyin(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "看书", "kàn shū", []string{"to read"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeTranslToZh,
		Answer: "不存在",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.UserAnswerPinyin != nil {
		t.Errorf("expected user_answer_pinyin nil for unknown word, got %q", *resp.UserAnswerPinyin)
	}
}

func TestQuizAnswer_ZhToTransl_NoUserAnswerPinyin(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "看书", "kàn shū", []string{"to read"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
		Answer: "wrong",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.UserAnswerPinyin != nil {
		t.Errorf("user_answer_pinyin should be absent for zh_to_transl, got %q", *resp.UserAnswerPinyin)
	}
}

// TestQuizAnswer_ZhToTranslNoSound_GradesLikeZhToTransl verifies the new mode
// is graded identically to zh_to_transl (the typed answer is checked against
// the word's translations) — it only affects sound availability, not grading.
func TestQuizAnswer_ZhToTranslNoSound_GradesLikeZhToTransl(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTranslNoSound,
		Answer: "hello",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("want correct=true for a matching translation")
	}
}

func TestQuizAnswer_WordNotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: 9999,
		Mode:   models.ModeZhToTransl,
		Answer: "hello",
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestQuizAnswer_InvalidMode(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   "invalid_mode",
		Answer: "hello",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestQuizAnswer_InvalidJSON(t *testing.T) {
	r := newRouter(openTestDB(t))
	req := httptest.NewRequest("POST", "/api/quiz/answer", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestQuizAnswer_ResponseContainsZhAndEN(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
		Answer: "wrong",
	})
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.ZhText != "你好" {
		t.Errorf("ZhText: want 你好, got %q", resp.ZhText)
	}
	if len(resp.Translations["en"]) == 0 {
		t.Error("Translations[en] should be populated in response")
	}
}

func TestQuizAnswer_MultiLang_DEAccepted(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello"}, "de": {"hallo"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := newRouter(s)

	// Answer with German when langs includes "de" — should be correct.
	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
		Answer: "hallo",
		Langs:  []string{"en", "de"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("German answer 'hallo' should be accepted when de is in langs")
	}
}

func TestQuizAnswer_MultiLang_ResponseContainsDeTexts(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "再见",
		Pinyin:       "zàijiàn",
		Translations: map[string][]string{"en": {"goodbye"}, "de": {"auf Wiedersehen"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
		Answer: "wrong",
		Langs:  []string{"en", "de"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Translations["de"]) == 0 {
		t.Error("DeTexts should be populated in response when word has DE translations")
	}
	if resp.Translations["de"][0] != "auf Wiedersehen" {
		t.Errorf("DeTexts[0]: want 'auf Wiedersehen', got %q", resp.Translations["de"][0])
	}
}

func TestQuizAnswer_DefaultLang_EnOnly(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello"}, "de": {"hallo"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := newRouter(s)

	// Answer with German when langs not specified (defaults to ["en"]) — should be wrong.
	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
		Answer: "hallo",
		// Langs omitted → defaults to ["en"]
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Correct {
		t.Error("German answer 'hallo' should NOT be accepted when langs defaults to [en]")
	}
}

func TestQuizAnswer_CommaJoinedTranslation_PartAccepted(t *testing.T) {
	// Regression for #189: a translation stored as a single comma-joined row
	// (e.g. "topic, item") must accept each comma-separated part on its own,
	// the same way slash-separated alternatives already do.
	s := openTestDB(t)
	ctx := context.Background()
	id, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "题",
		Pinyin:       "tí",
		Translations: map[string][]string{"en": {"topic, item"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
		Answer: "item",
		Langs:  []string{"en"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("'item' should be accepted as a valid part of the comma-joined translation 'topic, item'")
	}
}

func TestAnswerWrongStoresPrevState(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	// Confirm initial EF before submitting any answer.
	before, err := s.GetSM2Progress(ctx, id)
	if err != nil || before == nil {
		t.Fatalf("GetSM2Progress before answer: %v / %v", err, before)
	}
	initialEF := before.Easiness

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
		Answer: "wrong",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	prev, err := s.GetSM2PrevState(ctx, id)
	if err != nil {
		t.Fatalf("GetSM2PrevState: %v", err)
	}
	if prev == nil {
		t.Fatal("expected prev_state to be set after wrong answer, got nil")
	}
	if prev.Easiness != initialEF {
		t.Errorf("prev_state EF: want %v (pre-answer), got %v", initialEF, prev.Easiness)
	}
}

func TestAnswerCorrectClearsPrevState(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	// First submit wrong to set prev_state.
	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhToTransl, Answer: "wrong",
	})

	// Then submit correct.
	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhToTransl, Answer: "hello",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	prev, err := s.GetSM2PrevState(ctx, id)
	if err != nil {
		t.Fatalf("GetSM2PrevState: %v", err)
	}
	if prev != nil {
		t.Errorf("expected prev_state to be cleared after correct answer, got %+v", prev)
	}
}

func TestAnswer_FirstTryCorrect_IncrementsKnownCorrectCount(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhToTransl, Answer: "hello",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	got, err := s.GetSM2Progress(ctx, id)
	if err != nil || got == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, got)
	}
	if got.KnownCorrectCount != 1 {
		t.Errorf("known_correct_count after first-try-correct answer: want 1, got %d", got.KnownCorrectCount)
	}
}

func TestAnswer_WrongThenCorrect_DoesNotIncrementKnownCorrectCount(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	// Wrong first.
	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhToTransl, Answer: "wrong",
	})
	// Then correct.
	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhToTransl, Answer: "hello",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	got, err := s.GetSM2Progress(ctx, id)
	if err != nil || got == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, got)
	}
	if got.KnownCorrectCount != 0 {
		t.Errorf("known_correct_count after wrong-then-correct: want 0, got %d", got.KnownCorrectCount)
	}
}

func TestAnswer_WrongUnderKnownOnly_SetsCyclePinMode(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, http.MethodPatch, "/api/settings", map[string]interface{}{
		"primary_lang":                "en",
		"secondary_lang":              "",
		"prog_new":                    "transl_to_zh",
		"prog_tier_struggling":        "transl_to_zh",
		"prog_tier_learning":          "zh_pinyin_to_transl",
		"prog_tier_practicing":        "zh_to_transl",
		"prog_tier_mastered":          "random",
		"new_word_mode_0":             "transl_to_zh",
		"new_word_mode_1":             "transl_to_zh",
		"new_word_mode_2":             "zh_to_transl",
		"cycle_advance_on_known_only": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhPinyinToTransl, Answer: "wrong",
	})

	got, err := s.GetCyclePinMode(ctx, id)
	if err != nil {
		t.Fatalf("GetCyclePinMode: %v", err)
	}
	if got != models.ModeZhPinyinToTransl {
		t.Errorf("GetCyclePinMode after wrong answer: want %s, got %q", models.ModeZhPinyinToTransl, got)
	}
}

func TestAnswer_CorrectAnswer_ClearsCyclePinMode(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, http.MethodPatch, "/api/settings", map[string]interface{}{
		"primary_lang":                "en",
		"secondary_lang":              "",
		"prog_new":                    "transl_to_zh",
		"prog_tier_struggling":        "transl_to_zh",
		"prog_tier_learning":          "zh_pinyin_to_transl",
		"prog_tier_practicing":        "zh_to_transl",
		"prog_tier_mastered":          "random",
		"new_word_mode_0":             "transl_to_zh",
		"new_word_mode_1":             "transl_to_zh",
		"new_word_mode_2":             "zh_to_transl",
		"cycle_advance_on_known_only": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Wrong first, pinning the mode.
	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhPinyinToTransl, Answer: "wrong",
	})
	if got, _ := s.GetCyclePinMode(ctx, id); got == "" {
		t.Fatal("expected cycle_pin_mode to be set after wrong answer")
	}

	// Correct (not first try) resolves the encounter.
	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhPinyinToTransl, Answer: "hello",
	})

	got, err := s.GetCyclePinMode(ctx, id)
	if err != nil {
		t.Fatalf("GetCyclePinMode: %v", err)
	}
	if got != "" {
		t.Errorf("expected cycle_pin_mode cleared after correct answer, got %q", got)
	}
}

func TestAcceptCorrect_ClearsCyclePinMode(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, http.MethodPatch, "/api/settings", map[string]interface{}{
		"primary_lang":                "en",
		"secondary_lang":              "",
		"prog_new":                    "transl_to_zh",
		"prog_tier_struggling":        "transl_to_zh",
		"prog_tier_learning":          "zh_pinyin_to_transl",
		"prog_tier_practicing":        "zh_to_transl",
		"prog_tier_mastered":          "random",
		"new_word_mode_0":             "transl_to_zh",
		"new_word_mode_1":             "transl_to_zh",
		"new_word_mode_2":             "zh_to_transl",
		"cycle_advance_on_known_only": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhPinyinToTransl, Answer: "wrong",
	})
	if got, _ := s.GetCyclePinMode(ctx, id); got == "" {
		t.Fatal("expected cycle_pin_mode to be set after wrong answer")
	}

	rec = do(t, r, "POST", "/api/quiz/accept-correct", models.AcceptCorrectRequest{WordID: id})
	if rec.Code != http.StatusOK {
		t.Fatalf("accept-correct: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	got, err := s.GetCyclePinMode(ctx, id)
	if err != nil {
		t.Fatalf("GetCyclePinMode: %v", err)
	}
	if got != "" {
		t.Errorf("expected cycle_pin_mode cleared after accept-correct, got %q", got)
	}
}

func TestAnswerAmbiguous_TranslToZh_SetsAmbiguousFlag(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// Seed two zh words that share the EN translation "know"
	id1 := seedWord(t, s, "知道", "zhīdào", []string{"know"})
	id2 := seedWord(t, s, "认识", "rènshi", []string{"know", "recognize"})
	markWordTrained(t, s, id2)

	// Acknowledge id1 so it is available in the quiz
	if err := s.AcknowledgeWord(context.Background(), int64(2), id1); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	// Submit 认识 as the answer when the quiz word is 知道 (transl_to_zh)
	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id1,
		Mode:   models.ModeTranslToZh,
		Answer: "认识",
		Langs:  []string{"en"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Correct {
		t.Error("expected correct=false")
	}
	if !resp.Ambiguous {
		t.Error("expected ambiguous=true when typed word shares a translation with the quiz word")
	}
	// Issue #617: sentence cards have no prompt, so the result names the
	// shared meaning itself.
	if resp.AmbiguousMeaning != "know" {
		t.Errorf("ambiguous_meaning = %q; want %q", resp.AmbiguousMeaning, "know")
	}
	if resp.ConfusedWith == nil {
		t.Error("expected confused_with to be populated")
	}
}

func TestAnswerAmbiguous_NonSharedTranslation_NotAmbiguous(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// Seed two zh words with distinct translations
	id1 := seedWord(t, s, "书", "shū", []string{"book"})
	seedWord(t, s, "鱼", "yú", []string{"fish"})
	if err := s.AcknowledgeWord(context.Background(), int64(2), id1); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	// Typing the other zh word — it does NOT share "book" — should be just a confusion
	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id1,
		Mode:   models.ModeTranslToZh,
		Answer: "鱼",
		Langs:  []string{"en"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Ambiguous {
		t.Error("expected ambiguous=false when typed word does not share a translation")
	}
	if resp.AmbiguousMeaning != "" {
		t.Errorf("ambiguous_meaning = %q; want empty", resp.AmbiguousMeaning)
	}
}

func TestAnswerAmbiguous_ZhToTranslMode_NotAmbiguous(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	id1 := seedWord(t, s, "知道", "zhīdào", []string{"know"})
	if err := s.AcknowledgeWord(context.Background(), int64(2), id1); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	// In zh_to_transl mode ambiguity detection should NOT fire
	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id1,
		Mode:   models.ModeZhToTransl,
		Answer: "wrong",
		Langs:  []string{"en"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Ambiguous {
		t.Error("expected ambiguous=false for zh_to_transl mode")
	}
}

// TestAnswerAmbiguous_MultiGlossTranslation_SetsAmbiguousFlag regresses issue
// #188: 面 ("noodles"/"Nudeln") and 面条 ("noodle"/"Nudeln / Pasta") share the
// "Nudeln" meaning, but 面条's DE translation is a single multi-gloss entry
// rather than a separate "Nudeln" row. This must still be detected as
// ambiguous rather than falling through to a plain wrong answer.
func TestAnswerAmbiguous_MultiGlossTranslation_SetsAmbiguousFlag(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	id1 := seedWordFull(t, s, int64(2), "面", "miàn", []string{"noodles"}, []string{"Nudeln"}, nil)
	id2 := seedWordFull(t, s, int64(2), "面条", "miàntiáo", []string{"noodle"}, []string{"Nudeln / Pasta"}, nil)
	markWordTrained(t, s, id2)
	if err := s.AcknowledgeWord(context.Background(), int64(2), id1); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id1,
		Mode:   models.ModeTranslToZh,
		Answer: "面条",
		Langs:  []string{"en", "de"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Correct {
		t.Error("expected correct=false")
	}
	if !resp.Ambiguous {
		t.Error("expected ambiguous=true: 面条's 'Nudeln / Pasta' shares the 'Nudeln' meaning with 面")
	}
	if resp.AmbiguousMeaning != "Nudeln" {
		t.Errorf("ambiguous_meaning = %q; want %q (the quiz word's own text)", resp.AmbiguousMeaning, "Nudeln")
	}
}

func TestAcceptCorrectNoState(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/accept-correct", models.AcceptCorrectRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when no prev_state, got %d: %s", rec.Code, rec.Body)
	}
}

// A wrong answer and its "Accept as correct" count as one attempt without a
// mistake in today's session (issue #583).
func TestAcceptCorrect_CountsCardOnce(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhToTransl, Answer: "helo",
	})
	rec := do(t, r, "POST", "/api/quiz/accept-correct", models.AcceptCorrectRequest{
		WordID: id, Mode: models.ModeZhToTransl,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	var stats map[string]int
	decodeJSON(t, do(t, r, "GET", "/api/quiz/stats", nil), &stats)
	if stats["today_attempts"] != 1 || stats["today_mistakes"] != 0 {
		t.Errorf("today attempts/mistakes = %d/%d, want 1/0", stats["today_attempts"], stats["today_mistakes"])
	}
}

func TestAcceptCorrectRestoresProgress(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	// Seed a graduated SM-2 state (rep=3, EF=2.5, interval=1 day).
	graduated := models.SM2Progress{
		WordID:          id,
		Repetitions:     3,
		Easiness:        2.5,
		IntervalDays:    1,
		DueDate:         time.Now().UTC(),
		TotalCorrect:    3,
		TotalAttempts:   3,
		LearningNewWord: false,
	}
	if err := s.UpdateSM2Progress(ctx, graduated); err != nil {
		t.Fatalf("seed progress: %v", err)
	}
	initialEF := graduated.Easiness

	// Submit wrong answer — decrements EF and resets repetitions/interval.
	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhToTransl, Answer: "wrong",
	})

	// Accept as correct — restores pre-wrong state and applies correct quality.
	rec := do(t, r, "POST", "/api/quiz/accept-correct", models.AcceptCorrectRequest{
		WordID: id,
		Mode:   models.ModeZhToTransl,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("accept-correct should return correct: true")
	}
	if resp.TotalCorrect != 4 {
		t.Errorf("TotalCorrect: want 4 (pre-answer 3 + 1), got %d", resp.TotalCorrect)
	}
	if resp.TotalAttempts != 4 {
		t.Errorf("TotalAttempts: want 4 (pre-answer 3 + 1), got %d", resp.TotalAttempts)
	}

	after, _ := s.GetSM2Progress(ctx, id)

	// EF after accept-correct should be >= initial (correct quality on sm2.Update bumps it).
	if after.Easiness < initialEF {
		t.Errorf("EF after accept-correct (%v) should be >= initial EF (%v)", after.Easiness, initialEF)
	}

	// prev_state should be cleared.
	prev, _ := s.GetSM2PrevState(ctx, id)
	if prev != nil {
		t.Errorf("prev_state should be nil after accept-correct, got %+v", prev)
	}

	// Due date should be at least 1 day away (not the 3-minute wrong penalty).
	if time.Until(after.DueDate) < 20*time.Hour {
		t.Errorf("due date after accept-correct should be >= 1 day, got %v from now", time.Until(after.DueDate))
	}
}

func TestAcceptCorrectInvalidWordID(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/quiz/accept-correct", models.AcceptCorrectRequest{
		WordID: 0,
		Mode:   models.ModeZhToTransl,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for word_id=0, got %d", rec.Code)
	}
}

func TestQuizAnswer_VoiceToTransl_GradesLikeZhToTransl(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeVoiceToTransl,
		Answer: "hello",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("want correct=true for a matching translation")
	}
}

// seedReviewWord seeds a word that left the New bucket, is due now and was
// last answered yesterday, so its next answer is a review that can lapse.
func seedReviewWord(t *testing.T, s *db.Store, zhText, en string, consecutiveLapses int) int64 {
	t.Helper()
	id := seedWord(t, s, zhText, "", []string{en})
	if _, err := s.ExecForTest(`UPDATE sm2_progress SET learning_new_word = 0, repetitions = 2, interval_days = 3,
		first_seen_at = datetime('now', '-10 days'), last_attempt_at = datetime('now', '-1 day'),
		due_date = datetime('now', '-1 hour'), lapses = ?, consecutive_lapses = ? WHERE word_id = ?`,
		consecutiveLapses, consecutiveLapses, id); err != nil {
		t.Fatalf("seedReviewWord: %v", err)
	}
	return id
}

func TestAnswer_WrongReviewCountsLapse(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	id := seedReviewWord(t, s, "记住", "remember", 1)

	rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{"word_id": id, "mode": "zh_to_transl", "answer": "wrong"})
	if rec.Code != 200 {
		t.Fatalf("answer: %d %s", rec.Code, rec.Body)
	}
	// A retry on the same day is not a review and must not count again.
	do(t, r, "POST", "/api/quiz/answer", map[string]any{"word_id": id, "mode": "zh_to_transl", "answer": "wrong"})

	p, _ := s.GetSM2Progress(context.Background(), id)
	if p.Lapses != 2 || p.ConsecutiveLapses != 2 {
		t.Errorf("lapses: want 2/2, got %d/%d", p.Lapses, p.ConsecutiveLapses)
	}
}

func TestAnswer_DrillDoesNotCountLapse(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	id := seedReviewWord(t, s, "记住", "remember", 1)

	rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{"word_id": id, "mode": "zh_to_transl", "answer": "wrong", "drill": true})
	if rec.Code != 200 {
		t.Fatalf("answer: %d %s", rec.Code, rec.Body)
	}
	p, _ := s.GetSM2Progress(context.Background(), id)
	if p.Lapses != 1 || p.ConsecutiveLapses != 1 {
		t.Errorf("lapses: want 1/1 (unchanged), got %d/%d", p.Lapses, p.ConsecutiveLapses)
	}
}

func TestAcceptCorrect_UndoesLapse(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	id := seedReviewWord(t, s, "记住", "remember", 2)

	do(t, r, "POST", "/api/quiz/answer", map[string]any{"word_id": id, "mode": "zh_to_transl", "answer": "remembr"})
	rec := do(t, r, "POST", "/api/quiz/accept-correct", map[string]any{"word_id": id, "mode": "zh_to_transl"})
	if rec.Code != 200 {
		t.Fatalf("accept-correct: %d %s", rec.Code, rec.Body)
	}
	// The accepted answer is a correct first answer: the lapse is undone and
	// the run of consecutive lapses ends.
	p, _ := s.GetSM2Progress(context.Background(), id)
	if p.Lapses != 2 || p.ConsecutiveLapses != 0 {
		t.Errorf("lapses: want 2/0, got %d/%d", p.Lapses, p.ConsecutiveLapses)
	}
}

func TestAnswer_LeechAtThresholdAndMultiples(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	// Default threshold is 5.
	tests := []struct {
		before    int
		correct   bool
		wantLeech int
	}{
		{3, false, 0},  // 4th lapse in a row
		{4, false, 5},  // 5th: the warning shows
		{5, false, 0},  // 6th: no repeat
		{9, false, 10}, // 10th: shows again
		{4, true, 0},   // correct answer: no warning
	}
	for i, tc := range tests {
		id := seedReviewWord(t, s, string(rune('甲'+i)), "remember", tc.before)
		answer := "wrong"
		if tc.correct {
			answer = "remember"
		}
		rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{"word_id": id, "mode": "zh_to_transl", "answer": answer})
		var resp struct {
			LeechLapses int `json:"leech_lapses"`
		}
		decodeJSON(t, rec, &resp)
		if resp.LeechLapses != tc.wantLeech {
			t.Errorf("before=%d correct=%v: leech_lapses want %d, got %d", tc.before, tc.correct, tc.wantLeech, resp.LeechLapses)
		}
	}
}

func TestAnswer_LeechOffWhenThresholdZero(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	if rec := patchSettingsFromCurrent(t, r, func(st map[string]any) { st["leech_threshold"] = 0 }); rec.Code != 200 {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}
	id := seedReviewWord(t, s, "记住", "remember", 4)
	rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{"word_id": id, "mode": "zh_to_transl", "answer": "wrong"})
	var resp struct {
		LeechLapses int `json:"leech_lapses"`
	}
	decodeJSON(t, rec, &resp)
	if resp.LeechLapses != 0 {
		t.Errorf("leech_lapses with threshold 0: want 0, got %d", resp.LeechLapses)
	}
}

func TestAcceptCorrect_VoiceToTransl(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeVoiceToTransl, Answer: "helo",
	})
	rec := do(t, r, "POST", "/api/quiz/accept-correct", models.AcceptCorrectRequest{
		WordID: id,
		Mode:   models.ModeVoiceToTransl,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for voice_to_transl accept-correct, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if len(resp.CorrectAnswers) != 1 || resp.CorrectAnswers[0] != "hello" {
		t.Errorf("CorrectAnswers: want [hello], got %v", resp.CorrectAnswers)
	}
}

// TestQuizAnswer_VoiceToZh covers issue #571: the answer is the hanzi; a
// wrong word with the same pinyin and tones (from the user's words or the
// dictionary) is flagged as a homophone, other wrong words are not.
func TestQuizAnswer_VoiceToZh(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "他", "tā", []string{"he"})
	seedWord(t, s, "她", "tā", []string{"she"})
	seedWord(t, s, "大", "dà", []string{"big"})
	if _, err := s.ExecForTest(`INSERT INTO cedict_entries (simplified, pinyin, definition, lang) VALUES ('它', 'tā', 'it', 'en')`); err != nil {
		t.Fatal(err)
	}
	r := newRouter(s)

	cases := []struct {
		answer             string
		correct, homophone bool
	}{
		{"他", true, false},
		{"她", false, true}, // user word, same pinyin
		{"它", false, true}, // dictionary only, same pinyin
		{"大", false, false},
		{"踏", false, false}, // unknown word
	}
	for _, c := range cases {
		rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
			WordID: id, Mode: models.ModeVoiceToZh, Answer: c.answer, Langs: []string{"en"},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d: %s", c.answer, rec.Code, rec.Body)
		}
		var resp models.AnswerResponse
		decodeJSON(t, rec, &resp)
		if resp.Correct != c.correct || resp.Homophone != c.homophone {
			t.Errorf("%s: want correct=%v homophone=%v, got correct=%v homophone=%v",
				c.answer, c.correct, c.homophone, resp.Correct, resp.Homophone)
		}
		if len(resp.CorrectAnswers) != 1 || resp.CorrectAnswers[0] != "他" {
			t.Errorf("%s: want correct_answers [他], got %v", c.answer, resp.CorrectAnswers)
		}
	}
}

// TestQuizAnswer_VoiceOneDiffHomophone covers issue #628: on both voice cards,
// an answer that differs from the word in one word (voice_to_transl) or one
// character (voice_to_zh) with the same pinyin is a homophone, also when the
// typed text is not a word of the user.
func TestQuizAnswer_VoiceOneDiffHomophone(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "她有孩子", "tā yǒu háizi", []string{"she has a child"})
	seedWord(t, s, "他", "tā", []string{"he"})
	seedWord(t, s, "她", "tā", []string{"she"})
	seedWord(t, s, "你", "nǐ", []string{"you"})
	if _, err := s.ExecForTest(`INSERT INTO cedict_entries (simplified, pinyin, definition, lang) VALUES ('它', 'tā', 'it', 'en')`); err != nil {
		t.Fatal(err)
	}
	r := newRouter(s)

	cases := []struct {
		mode, answer string
		homophone    bool
	}{
		{models.ModeVoiceToTransl, "He has a child", true},
		{models.ModeVoiceToTransl, "you has a child", false}, // other pinyin
		{models.ModeVoiceToTransl, "it has a child", false},  // "it" is no word of the user
		{models.ModeVoiceToTransl, "he has a dog", false},    // two words differ
		{models.ModeVoiceToZh, "他有孩子", true},
		{models.ModeVoiceToZh, "它有孩子", true}, // 它 from the dictionary
		{models.ModeVoiceToZh, "你有孩子", false},
		{models.ModeVoiceToZh, "他有孩们", false}, // two characters differ
	}
	for _, c := range cases {
		rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
			WordID: id, Mode: c.mode, Answer: c.answer, Langs: []string{"en"},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d: %s", c.answer, rec.Code, rec.Body)
		}
		var resp models.AnswerResponse
		decodeJSON(t, rec, &resp)
		if resp.Correct || resp.Homophone != c.homophone {
			t.Errorf("%s %q: want correct=false homophone=%v, got correct=%v homophone=%v",
				c.mode, c.answer, c.homophone, resp.Correct, resp.Homophone)
		}
	}
}

// TestQuizAnswer_VoiceToZhHomophoneSentence covers issue #630: on a
// voice_to_zh card for a sentence with a trailing question mark, an answer
// without the mark that differs in one character with the same pinyin is a
// homophone.
func TestQuizAnswer_VoiceToZhHomophoneSentence(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "她喜欢什么？", "tā xǐhuan shénme", []string{"what does she like?"})
	seedWord(t, s, "他", "tā", []string{"he"})
	seedWord(t, s, "她", "tā", []string{"she"})
	seedWord(t, s, "你", "nǐ", []string{"you"})
	r := newRouter(s)

	cases := []struct {
		answer    string
		homophone bool
	}{
		{"他喜欢什么", true},
		{"他喜欢什么？", true},
		{"你喜欢什么", false},
	}
	for _, c := range cases {
		rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
			WordID: id, Mode: models.ModeVoiceToZh, Answer: c.answer, Langs: []string{"en"},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d: %s", c.answer, rec.Code, rec.Body)
		}
		var resp models.AnswerResponse
		decodeJSON(t, rec, &resp)
		if resp.Correct || resp.Homophone != c.homophone {
			t.Errorf("%q: want correct=false homophone=%v, got correct=%v homophone=%v",
				c.answer, c.homophone, resp.Correct, resp.Homophone)
		}
	}
}

// TestQuizAnswer_VoiceToTranslHomophone covers issue #624: on a voice_to_transl
// card, a meaning of another word with the same pinyin and tones is flagged as
// a homophone; a meaning of a word with other pinyin is not.
func TestQuizAnswer_VoiceToTranslHomophone(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "角", "jiǎo", []string{"corner"})
	markWordTrained(t, s, seedWord(t, s, "脚", "jiǎo", []string{"foot"}))
	markWordTrained(t, s, seedWord(t, s, "叫", "jiào", []string{"call"}))
	r := newRouter(s)

	cases := []struct {
		answer             string
		correct, homophone bool
	}{
		{"corner", true, false},
		{"foot", false, true},
		{"call", false, false}, // other tone
		{"table", false, false},
	}
	for _, c := range cases {
		rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
			WordID: id, Mode: models.ModeVoiceToTransl, Answer: c.answer, Langs: []string{"en"},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d: %s", c.answer, rec.Code, rec.Body)
		}
		var resp models.AnswerResponse
		decodeJSON(t, rec, &resp)
		if resp.Correct != c.correct || resp.Homophone != c.homophone {
			t.Errorf("%s: want correct=%v homophone=%v, got correct=%v homophone=%v",
				c.answer, c.correct, c.homophone, resp.Correct, resp.Homophone)
		}
	}
}

func TestQuizAcceptCorrect_VoiceToZh(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "他", "tā", []string{"he"})
	seedWord(t, s, "她", "tā", []string{"she"})
	r := newRouter(s)

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{WordID: id, Mode: models.ModeVoiceToZh, Answer: "她"})
	rec := do(t, r, "POST", "/api/quiz/accept-correct", models.AcceptCorrectRequest{WordID: id, Mode: models.ModeVoiceToZh})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if len(resp.CorrectAnswers) != 1 || resp.CorrectAnswers[0] != "他" {
		t.Errorf("CorrectAnswers: want [他], got %v", resp.CorrectAnswers)
	}
}
