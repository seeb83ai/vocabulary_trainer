package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestAnswerAmbiguous_TranslToZh_SetsAmbiguousFlag(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// Seed two zh words that share the EN translation "know"
	id1 := seedWord(t, s, "知道", "zhīdào", []string{"know"})
	seedWord(t, s, "认识", "rènshi", []string{"know", "recognize"})

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
	seedWordFull(t, s, int64(2), "面条", "miàntiáo", []string{"noodle"}, []string{"Nudeln / Pasta"}, nil)
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
