package handlers_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"vocabulary_trainer/models"
)

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

func TestWordsCreate_Valid(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:       "再见",
		Pinyin:       "zàijiàn",
		Translations: map[string][]string{"en": {"goodbye"}},
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
		Translations: map[string][]string{"en": {"hello"}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestWordsCreate_NoTranslations(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText: "你好",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestWordsCreate_DeOnlyValid(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:       "你好",
		Translations: map[string][]string{"de": {"Hallo"}},
	})
	if rec.Code != http.StatusCreated {
		t.Errorf("want 201, got %d: %s", rec.Code, rec.Body.String())
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

func TestWordsCreate_StartTraining(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:        "学习",
		Pinyin:        "xuéxí",
		Translations:  map[string][]string{"en": {"to study"}},
		StartTraining: true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rec.Code, rec.Body)
	}
	var resp map[string]int64
	decodeJSON(t, rec, &resp)

	// Fetch the word and verify it was acknowledged (total_attempts = 1).
	rec2 := do(t, r, "GET", fmt.Sprintf("/api/words/%d", resp["id"]), nil)
	var wd models.WordDetail
	decodeJSON(t, rec2, &wd)
	if wd.TotalAttempts != 1 {
		t.Errorf("want TotalAttempts=1 after start_training, got %d", wd.TotalAttempts)
	}
}

func TestWordsUpdate_StartTraining(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "PUT", fmt.Sprintf("/api/words/%d", id), models.UpdateWordRequest{
		ZhText:        "你好",
		Pinyin:        "nǐ hǎo",
		Translations:  map[string][]string{"en": {"hello"}},
		StartTraining: true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var wd models.WordDetail
	decodeJSON(t, rec, &wd)
	if wd.TotalAttempts != 1 {
		t.Errorf("want TotalAttempts=1 after start_training, got %d", wd.TotalAttempts)
	}
}

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

func TestWordsUpdate_Valid(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "PUT", fmt.Sprintf("/api/words/%d", id), models.UpdateWordRequest{
		ZhText:       "你好吗",
		Pinyin:       "nǐ hǎo ma",
		Translations: map[string][]string{"en": {"how are you"}},
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
		ZhText:       "test",
		Translations: map[string][]string{"en": {"test"}},
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
		Translations: map[string][]string{"en": {"hello"}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

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

func TestWordsAddTranslation_Valid(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", fmt.Sprintf("/api/words/%d/translations", id),
		map[string]string{"text": "hi", "lang": "en"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	// Verify it's listed in the word
	rec = do(t, r, "GET", fmt.Sprintf("/api/words/%d", id), nil)
	var wd models.WordDetail
	decodeJSON(t, rec, &wd)
	found := false
	for _, e := range wd.Translations["en"] {
		if e == "hi" {
			found = true
		}
	}
	if !found {
		t.Errorf("'hi' not found in EnTexts after AddTranslation: %v", wd.Translations["en"])
	}
}

func TestWordsAddTranslation_EmptyText(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", fmt.Sprintf("/api/words/%d/translations", id),
		map[string]string{"text": "", "lang": "en"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestWordsAddTranslation_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/words/9999/translations",
		map[string]string{"text": "hello", "lang": "en"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestWordsAddTranslation_Idempotent(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "", []string{"hello"})
	r := newRouter(s)

	body := map[string]string{"text": "hi", "lang": "en"}
	do(t, r, "POST", fmt.Sprintf("/api/words/%d/translations", id), body)
	rec := do(t, r, "POST", fmt.Sprintf("/api/words/%d/translations", id), body)
	if rec.Code != http.StatusNoContent {
		t.Errorf("second identical add should still return 204, got %d", rec.Code)
	}

	rec = do(t, r, "GET", fmt.Sprintf("/api/words/%d", id), nil)
	var wd models.WordDetail
	decodeJSON(t, rec, &wd)
	count := 0
	for _, e := range wd.Translations["en"] {
		if e == "hi" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("'hi' should appear exactly once, got %d", count)
	}
}

func TestWordsExport_ReturnsAllWords(t *testing.T) {
	s := openTestDB(t)
	for i := 0; i < 5; i++ {
		seedWord(t, s, fmt.Sprintf("词%d", i), "", []string{"word"})
	}
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/words/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var words []models.WordDetail
	decodeJSON(t, rec, &words)
	if len(words) != 5 {
		t.Errorf("want 5 words, got %d", len(words))
	}
}

func TestWordsExport_RespectsFilters(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	seedWord(t, s, "谢谢", "xièxiè", []string{"thank you"})
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/words/export?q=你好", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var words []models.WordDetail
	decodeJSON(t, rec, &words)
	if len(words) != 1 {
		t.Errorf("want 1 word matching search, got %d", len(words))
	}
	if len(words) > 0 && words[0].ZhText != "你好" {
		t.Errorf("want 你好, got %s", words[0].ZhText)
	}
}

func TestMarkReview_SetsFlag(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "POST", fmt.Sprintf("/api/words/%d/review", id), nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	// Confirm via GET /api/words/{id}
	rec2 := do(t, r, "GET", fmt.Sprintf("/api/words/%d", id), nil)
	var wd models.WordDetail
	decodeJSON(t, rec2, &wd)
	if !wd.NeedsReview {
		t.Error("expected needs_review = true after POST /review")
	}
}

func TestMarkReview_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/words/9999/review", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestMarkReview_ClearedOnUpdate(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	do(t, r, "POST", fmt.Sprintf("/api/words/%d/review", id), nil)

	rec := do(t, r, "PUT", fmt.Sprintf("/api/words/%d", id), models.UpdateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: want 200, got %d: %s", rec.Code, rec.Body)
	}

	var wd models.WordDetail
	decodeJSON(t, rec, &wd)
	if wd.NeedsReview {
		t.Error("expected needs_review = false after PUT update")
	}
}

func TestResetProgress_RestoresUnseenState(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "水", "shuǐ", []string{"water"})
	r := newRouter(s)

	if err := s.AcknowledgeWord(context.Background(), 2, id); err != nil {
		t.Fatal(err)
	}

	rec := do(t, r, "POST", fmt.Sprintf("/api/words/%d/reset", id), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var wd models.WordDetail
	decodeJSON(t, rec, &wd)
	if wd.TotalAttempts != 0 {
		t.Errorf("expected total_attempts = 0 after reset, got %d", wd.TotalAttempts)
	}

	rec2 := do(t, r, "GET", fmt.Sprintf("/api/words/%d", id), nil)
	var wd2 models.WordDetail
	decodeJSON(t, rec2, &wd2)
	if wd2.TotalAttempts != 0 {
		t.Errorf("expected total_attempts = 0 on refetch after reset, got %d", wd2.TotalAttempts)
	}
}

func TestResetProgress_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/words/9999/reset", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestWordList_ReviewFilter(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	_ = seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})
	r := newRouter(s)

	do(t, r, "POST", fmt.Sprintf("/api/words/%d/review", id1), nil)

	rec := do(t, r, "GET", "/api/words/?review=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.WordListResponse
	decodeJSON(t, rec, &resp)
	if resp.Total != 1 {
		t.Errorf("review filter: want total=1, got %d", resp.Total)
	}
	if len(resp.Words) != 1 || resp.Words[0].ID != id1 {
		t.Errorf("review filter: expected word %d, got %v", id1, resp.Words)
	}
}

func TestWordList_HideUnseenFilter(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	_ = seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})
	r := newRouter(s)

	// Submit an answer for id1 to mark it as seen (increments total_attempts)
	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id1,
		Mode:   "zh_to_transl",
		Answer: "hello",
	})

	// With hide_unseen=1, only id1 (seen) should appear
	rec := do(t, r, "GET", "/api/words/?hide_unseen=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.WordListResponse
	decodeJSON(t, rec, &resp)
	if resp.Total != 1 {
		t.Errorf("hide_unseen=1: want total=1, got %d", resp.Total)
	}
	if len(resp.Words) != 1 || resp.Words[0].ID != id1 {
		t.Errorf("hide_unseen=1: expected word %d, got %v", id1, resp.Words)
	}

	// Without hide_unseen, both words should appear
	rec2 := do(t, r, "GET", "/api/words/", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec2.Code, rec2.Body)
	}
	var resp2 models.WordListResponse
	decodeJSON(t, rec2, &resp2)
	if resp2.Total != 2 {
		t.Errorf("no hide_unseen param: want total=2, got %d", resp2.Total)
	}
}

func TestWordsCreate_StartTraining_SetsLearningPhase(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	ctx := context.Background()

	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:        "学",
		Translations:  map[string][]string{"en": {"study"}},
		StartTraining: true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rec.Code, rec.Body)
	}
	var resp map[string]int64
	decodeJSON(t, rec, &resp)

	p, err := s.GetSM2Progress(ctx, resp["id"])
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	if !p.LearningNewWord {
		t.Error("start_training=true must set learning_new_word=1 so the word enters the learning phase")
	}
}

func TestWordsCreate_ZhTextTooLong(t *testing.T) {
	r := newRouter(openTestDB(t))
	long201 := ""
	for i := 0; i < 201; i++ {
		long201 += "好"
	}
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:       long201,
		Translations: map[string][]string{"en": {"ok"}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for zh_text > 200 chars, got %d", rec.Code)
	}
}

func TestWordsCreate_TooManyTranslations(t *testing.T) {
	r := newRouter(openTestDB(t))
	texts := make([]string, 21)
	for i := range texts {
		texts[i] = fmt.Sprintf("translation %d", i)
	}
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:       "好",
		Translations: map[string][]string{"en": texts},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for > 20 translations, got %d", rec.Code)
	}
}

func TestWordsCreate_TooManyTags(t *testing.T) {
	r := newRouter(openTestDB(t))
	tags := make([]string, 21)
	for i := range tags {
		tags[i] = fmt.Sprintf("tag%d", i)
	}
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:       "好",
		Translations: map[string][]string{"en": {"ok"}},
		Tags:         tags,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for > 20 tags, got %d", rec.Code)
	}
}

func TestWordsUpdate_SameZhText_NoUniqueError(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	// Re-save with the exact same zh_text — should not return 500.
	rec := do(t, r, "PUT", fmt.Sprintf("/api/words/%d", id), models.UpdateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello", "hi"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var wd models.WordDetail
	decodeJSON(t, rec, &wd)
	if wd.ZhText != "你好" {
		t.Errorf("ZhText: want 你好, got %q", wd.ZhText)
	}
}

func TestWordsList_MissingLangDE(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Word with EN only.
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	// Word with both EN and DE.
	_, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "再见",
		Pinyin:       "zàijiàn",
		Translations: map[string][]string{"en": {"goodbye"}, "de": {"auf Wiedersehen"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/words?page=1&per_page=20&missing_lang=de", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.WordListResponse
	decodeJSON(t, rec, &resp)
	if resp.Total != 1 {
		t.Errorf("missing_lang=de: want 1 result, got %d", resp.Total)
	}
	if len(resp.Words) != 1 || resp.Words[0].ZhText != "你好" {
		t.Errorf("unexpected words: %v", resp.Words)
	}
}

func TestWordsList_MissingLangEmpty_ReturnsAll(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "", []string{"hello"})
	seedWord(t, s, "再见", "", []string{"goodbye"})
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/words?page=1&per_page=20", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp models.WordListResponse
	decodeJSON(t, rec, &resp)
	if resp.Total != 2 {
		t.Errorf("no missing_lang filter: want 2 results, got %d", resp.Total)
	}
}
