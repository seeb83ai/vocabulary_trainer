package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func openTestDB(t *testing.T) *db.Store {
	t.Helper()
	s, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("openTestDB: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newRouter(s *db.Store) http.Handler {
	wordsH := &handlers.WordsHandler{Store: s}
	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 100}
	mismatchH := &handlers.MismatchesHandler{Store: s}

	r := chi.NewRouter()
	r.Get("/api/quiz/next", quizH.Next)
	r.Post("/api/quiz/answer", quizH.Answer)
	r.Get("/api/quiz/stats", quizH.Stats)
	r.Get("/api/mismatches", mismatchH.List)
	r.Route("/api/words", func(r chi.Router) {
		r.Get("/", wordsH.List)
		r.Post("/", wordsH.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", wordsH.GetByID)
			r.Put("/", wordsH.Update)
			r.Delete("/", wordsH.Delete)
			r.Post("/translations", wordsH.AddTranslation)
		})
	})
	return r
}

func do(t *testing.T, r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
	}
}

func seedWord(t *testing.T, s *db.Store, zhText, pinyin string, enTexts []string) int64 {
	t.Helper()
	id, err := s.CreateWord(context.Background(), models.CreateWordRequest{
		ZhText:  zhText,
		Pinyin:  pinyin,
		EnTexts: enTexts,
	})
	if err != nil {
		t.Fatalf("seedWord: %v", err)
	}
	return id
}

// ── GET /api/quiz/next ────────────────────────────────────────────────────────

func TestQuizNext_EmptyDB(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/quiz/next", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["error"] != "no words available" {
		t.Errorf("unexpected error: %q", body["error"])
	}
}

func TestQuizNext_ReturnsCard(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/next", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.WordID <= 0 {
		t.Error("word_id should be positive")
	}
	if card.Mode == "" {
		t.Error("mode should not be empty")
	}
	if card.Prompt == "" {
		t.Error("prompt should not be empty")
	}
}

func TestQuizNext_NoPinyinFallsBackMode(t *testing.T) {
	s := openTestDB(t)
	// Word with no pinyin — zh_pinyin_to_en must never be returned
	_, err := s.CreateWord(context.Background(), models.CreateWordRequest{
		ZhText:  "你好",
		Pinyin:  "", // no pinyin
		EnTexts: []string{"hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := newRouter(s)

	for i := 0; i < 30; i++ {
		rec := do(t, r, "GET", "/api/quiz/next", nil)
		var card models.QuizCard
		decodeJSON(t, rec, &card)
		if card.Mode == models.ModeZhPinyinToEn {
			t.Error("zh_pinyin_to_en should not be returned when pinyin is absent")
		}
	}
}

func TestQuizNext_ModeParam(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	for _, mode := range []string{models.ModeEnToZh, models.ModeZhToEn, models.ModeZhPinyinToEn} {
		rec := do(t, r, "GET", "/api/quiz/next?mode="+mode, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("mode=%s: want 200, got %d: %s", mode, rec.Code, rec.Body)
		}
		var card models.QuizCard
		decodeJSON(t, rec, &card)
		if card.Mode != mode {
			t.Errorf("mode=%s: want card.Mode=%s, got %s", mode, mode, card.Mode)
		}
	}

	// Invalid mode falls back to a valid random mode
	rec := do(t, r, "GET", "/api/quiz/next?mode=invalid", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("invalid mode: want 200, got %d", rec.Code)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	validModes := map[string]bool{models.ModeEnToZh: true, models.ModeZhToEn: true, models.ModeZhPinyinToEn: true}
	if !validModes[card.Mode] {
		t.Errorf("invalid mode param: got unexpected mode %s", card.Mode)
	}
}

func TestQuizNext_DailyNewWordLimitBlocked(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed two words.
	id1 := seedWord(t, s, "一", "", []string{"one"})
	id2 := seedWord(t, s, "二", "", []string{"two"})

	// Push id2 into the future so id1 is always the most-due word.
	p2, err := s.GetSM2Progress(ctx, id2)
	if err != nil || p2 == nil {
		t.Fatalf("GetSM2Progress id2: %v / %v", err, p2)
	}
	p2.DueDate = time.Now().UTC().Add(48 * time.Hour)
	if err := s.UpdateSM2Progress(ctx, *p2); err != nil {
		t.Fatalf("UpdateSM2Progress id2: %v", err)
	}

	// Call GetNextCard once (high limit) to stamp id1 as today's introduced word.
	w, _, err := s.GetNextCard(ctx, nil, 100)
	if err != nil || w == nil || w.ID != id1 {
		t.Fatalf("setup: expected id1=%d to be stamped, got w=%v err=%v", id1, w, err)
	}

	// Build a router with maxNew=1 (cap is now reached).
	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 1}
	r := chi.NewRouter()
	r.Get("/api/quiz/next", quizH.Next)
	r.Get("/api/quiz/stats", quizH.Stats)

	// Only id1 (already introduced) should be returned — id2 is new and the cap is reached.
	rec := do(t, r, "GET", "/api/quiz/next", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.WordID != id1 {
		t.Errorf("expected already-seen word id=%d when daily cap is reached, got id=%d", id1, card.WordID)
	}

	// Stats should reflect new_today=1 and max_new_per_day=1.
	rec = do(t, r, "GET", "/api/quiz/stats", nil)
	var stats map[string]int
	decodeJSON(t, rec, &stats)
	if stats["new_today"] != 1 {
		t.Errorf("new_today: want 1, got %d", stats["new_today"])
	}
	if stats["max_new_per_day"] != 1 {
		t.Errorf("max_new_per_day: want 1, got %d", stats["max_new_per_day"])
	}
}

// ── POST /api/quiz/answer ─────────────────────────────────────────────────────

func TestQuizAnswer_Correct(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeZhToEn,
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
		Mode:   models.ModeZhToEn,
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

func TestQuizAnswer_EnToZh(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id,
		Mode:   models.ModeEnToZh,
		Answer: "你好",
	})
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("answer '你好' for en_to_zh should be correct")
	}
}

func TestQuizAnswer_WordNotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: 9999,
		Mode:   models.ModeZhToEn,
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
		Mode:   models.ModeZhToEn,
		Answer: "wrong",
	})
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.ZhText != "你好" {
		t.Errorf("ZhText: want 你好, got %q", resp.ZhText)
	}
	if len(resp.EnTexts) == 0 {
		t.Error("EnTexts should be populated in response")
	}
}

// ── GET /api/quiz/stats ───────────────────────────────────────────────────────

func TestQuizStats_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var stats map[string]int
	decodeJSON(t, rec, &stats)
	if stats["total"] != 0 {
		t.Errorf("total: want 0, got %d", stats["total"])
	}
}

func TestQuizStats_AfterInsert(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "", []string{"hello"})
	seedWord(t, s, "谢谢", "", []string{"thank you"})
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	var stats map[string]int
	decodeJSON(t, rec, &stats)
	if stats["total"] != 2 {
		t.Errorf("total: want 2, got %d", stats["total"])
	}
}

// ── GET /api/words ────────────────────────────────────────────────────────────

func TestWordsList_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/words?page=1&per_page=20", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp models.WordListResponse
	decodeJSON(t, rec, &resp)
	if resp.Total != 0 || len(resp.Words) != 0 {
		t.Errorf("expected empty list, got total=%d words=%d", resp.Total, len(resp.Words))
	}
}

func TestWordsList_Search(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	seedWord(t, s, "谢谢", "xiè xiè", []string{"thank you"})
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/words?q=thank&page=1&per_page=20", nil)
	var resp models.WordListResponse
	decodeJSON(t, rec, &resp)
	if resp.Total != 1 {
		t.Errorf("total: want 1, got %d", resp.Total)
	}
}

// ── POST /api/words ───────────────────────────────────────────────────────────

func TestWordsCreate_Valid(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:  "再见",
		Pinyin:  "zàijiàn",
		EnTexts: []string{"goodbye"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rec.Code, rec.Body)
	}
	var resp map[string]int64
	decodeJSON(t, rec, &resp)
	if resp["id"] <= 0 {
		t.Error("id should be positive")
	}
}

func TestWordsCreate_MissingZhText(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		EnTexts: []string{"hello"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestWordsCreate_MissingEnTexts(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText: "你好",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestWordsCreate_InvalidJSON(t *testing.T) {
	r := newRouter(openTestDB(t))
	req := httptest.NewRequest("POST", "/api/words", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

// ── GET /api/words/{id} ───────────────────────────────────────────────────────

func TestWordsGetByID_Found(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "GET", fmt.Sprintf("/api/words/%d", id), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var wd models.WordDetail
	decodeJSON(t, rec, &wd)
	if wd.ZhText != "你好" {
		t.Errorf("ZhText: want 你好, got %q", wd.ZhText)
	}
}

func TestWordsGetByID_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/words/9999", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestWordsGetByID_InvalidID(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/words/abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

// ── PUT /api/words/{id} ───────────────────────────────────────────────────────

func TestWordsUpdate_Valid(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "PUT", fmt.Sprintf("/api/words/%d", id), models.UpdateWordRequest{
		ZhText:  "你好吗",
		Pinyin:  "nǐ hǎo ma",
		EnTexts: []string{"how are you"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var wd models.WordDetail
	decodeJSON(t, rec, &wd)
	if wd.ZhText != "你好吗" {
		t.Errorf("ZhText: want 你好吗, got %q", wd.ZhText)
	}
}

func TestWordsUpdate_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "PUT", "/api/words/9999", models.UpdateWordRequest{
		ZhText:  "test",
		EnTexts: []string{"test"},
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestWordsUpdate_MissingZhText(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "PUT", fmt.Sprintf("/api/words/%d", id), models.UpdateWordRequest{
		EnTexts: []string{"hello"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

// ── DELETE /api/words/{id} ────────────────────────────────────────────────────

func TestWordsDelete_Valid(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "DELETE", fmt.Sprintf("/api/words/%d", id), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	// Confirm it's gone
	rec = do(t, r, "GET", fmt.Sprintf("/api/words/%d", id), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("word should be gone after delete, got %d", rec.Code)
	}
}

func TestWordsDelete_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "DELETE", "/api/words/9999", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

// ── POST /api/words/{id}/translations ────────────────────────────────────────

func TestWordsAddTranslation_Valid(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", fmt.Sprintf("/api/words/%d/translations", id),
		map[string]string{"en_text": "hi"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	// Verify it's listed in the word
	rec = do(t, r, "GET", fmt.Sprintf("/api/words/%d", id), nil)
	var wd models.WordDetail
	decodeJSON(t, rec, &wd)
	found := false
	for _, e := range wd.EnTexts {
		if e == "hi" {
			found = true
		}
	}
	if !found {
		t.Errorf("'hi' not found in EnTexts after AddTranslation: %v", wd.EnTexts)
	}
}

func TestWordsAddTranslation_EmptyText(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", fmt.Sprintf("/api/words/%d/translations", id),
		map[string]string{"en_text": ""})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestWordsAddTranslation_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/words/9999/translations",
		map[string]string{"en_text": "hello"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestWordsAddTranslation_Idempotent(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "", []string{"hello"})
	r := newRouter(s)

	body := map[string]string{"en_text": "hi"}
	do(t, r, "POST", fmt.Sprintf("/api/words/%d/translations", id), body)
	rec := do(t, r, "POST", fmt.Sprintf("/api/words/%d/translations", id), body)
	if rec.Code != http.StatusNoContent {
		t.Errorf("second identical add should still return 204, got %d", rec.Code)
	}

	rec = do(t, r, "GET", fmt.Sprintf("/api/words/%d", id), nil)
	var wd models.WordDetail
	decodeJSON(t, rec, &wd)
	count := 0
	for _, e := range wd.EnTexts {
		if e == "hi" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("'hi' should appear exactly once, got %d", count)
	}
}

// ── GET /api/mismatches ───────────────────────────────────────────────────────

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
		"mode":    "zh_to_en",
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
		"mode":    "zh_to_en",
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
		"mode":    "zh_to_en",
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
		"mode":    "en_to_zh",
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
			"mode":    "zh_to_en",
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
