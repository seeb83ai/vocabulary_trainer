package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

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

func TestDailyStats_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/quiz/daily-stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp models.DailyStatsResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Days) != 0 {
		t.Errorf("expected empty days, got %d", len(resp.Days))
	}
}

func TestDailyStats_PopulatedAfterAnswer(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "猫", "māo", []string{"cat"})
	r := newRouter(s)

	// Submit an answer to trigger daily stat recording
	rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": 1,
		"mode":    "zh_to_transl",
		"answer":  "cat",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("answer: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, "GET", "/api/quiz/daily-stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp models.DailyStatsResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Days) != 1 {
		t.Fatalf("expected 1 day, got %d", len(resp.Days))
	}
	if resp.Days[0].Attempts != 1 {
		t.Errorf("attempts: want 1, got %d", resp.Days[0].Attempts)
	}
	if resp.Days[0].Mistakes != 0 {
		t.Errorf("mistakes: want 0, got %d", resp.Days[0].Mistakes)
	}
	if resp.Days[0].WordsSeen != 0 {
		t.Errorf("words_seen: want 0, got %d", resp.Days[0].WordsSeen)
	}
	// Word was not presented via GetNextCard, so first_seen_date is NULL
	// and all bucket counts should be 0.
	if resp.Days[0].BucketNew != 0 {
		t.Errorf("bucket_new: want 0, got %d", resp.Days[0].BucketNew)
	}
	if resp.Days[0].BucketStruggling != 0 {
		t.Errorf("bucket_struggling: want 0, got %d", resp.Days[0].BucketStruggling)
	}
	if resp.Days[0].BucketMastered != 0 {
		t.Errorf("bucket_mastered: want 0, got %d", resp.Days[0].BucketMastered)
	}
}

func TestDailyStats_BucketCounts(t *testing.T) {
	s := openTestDB(t)
	catID := seedWord(t, s, "猫", "māo", []string{"cat"})
	dogID := seedWord(t, s, "狗", "gǒu", []string{"dog"})
	r := newRouter(s)

	// Acknowledge both words so first_seen_date is set
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": catID})
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": dogID})

	// Answer 猫 correctly once — still learning_new_word=1 → bucket "new"
	rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": catID, "mode": "zh_to_transl", "answer": "cat",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("answer cat: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Answer 狗 wrong once — still learning_new_word=1 → bucket "new"
	rec = do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": dogID, "mode": "zh_to_transl", "answer": "wrong",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("answer dog: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, "GET", "/api/quiz/daily-stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp models.DailyStatsResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Days) != 1 {
		t.Fatalf("expected 1 day, got %d", len(resp.Days))
	}
	day := resp.Days[0]
	// Both words are learning_new_word=1 with first_seen_date set
	if day.BucketNew != 2 {
		t.Errorf("bucket_new: want 2, got %d", day.BucketNew)
	}
	if day.BucketStruggling != 0 {
		t.Errorf("bucket_struggling: want 0, got %d", day.BucketStruggling)
	}
	if day.BucketLearning != 0 {
		t.Errorf("bucket_learning: want 0, got %d", day.BucketLearning)
	}
	if day.BucketPracticing != 0 {
		t.Errorf("bucket_practicing: want 0, got %d", day.BucketPracticing)
	}
	if day.BucketMastered != 0 {
		t.Errorf("bucket_mastered: want 0, got %d", day.BucketMastered)
	}
}

func TestWordStats_Empty(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/word-stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.WordStatsResponse
	decodeJSON(t, rec, &resp)
	if resp.TotalSeen != 0 {
		t.Errorf("total_seen: want 0, got %d", resp.TotalSeen)
	}
}

func TestWordStats_WithData(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// Seed words and answer them to create progress data
	seedWord(t, s, "猫", "māo", []string{"cat"})
	seedWord(t, s, "狗", "gǒu", []string{"dog"})
	seedWord(t, s, "鱼", "yú", []string{"fish"})

	// Acknowledge all words so first_seen_date is set
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": 1})
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": 2})
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": 3})

	// Answer 猫 correctly 3 times
	for i := 0; i < 3; i++ {
		rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
			"word_id": 1, "mode": "zh_to_transl", "answer": "cat",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("answer cat: want 200, got %d", rec.Code)
		}
	}
	// Answer 狗 wrong once, correct once
	do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": 2, "mode": "zh_to_transl", "answer": "wrong",
	})
	do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": 2, "mode": "zh_to_transl", "answer": "dog",
	})
	// Answer 鱼 correct once
	do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": 3, "mode": "zh_to_transl", "answer": "fish",
	})

	rec := do(t, r, "GET", "/api/quiz/word-stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.WordStatsResponse
	decodeJSON(t, rec, &resp)

	// At least 1 word should be seen (acknowledged words have first_seen_date set)
	if resp.TotalSeen < 1 {
		t.Errorf("total_seen: want >= 1, got %d", resp.TotalSeen)
	}

	// Accuracy buckets should have keys
	if _, ok := resp.AccBuckets["85-100"]; !ok {
		t.Error("accuracy_buckets missing '85-100' key")
	}

	// Most practiced should be non-empty
	if len(resp.MostPract) == 0 {
		t.Error("most_practiced should not be empty")
	}
	// Verify en_texts are populated
	for _, w := range resp.MostPract {
		if len(w.Translations["en"]) == 0 {
			t.Errorf("most_practiced word %d missing en_texts", w.WordID)
		}
	}
}

func TestWordStats_AccBucketsFilterByTag(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	tagged := seedWordFull(t, s, int64(2), "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"vip"})
	other := seedWordFull(t, s, int64(2), "再见", "zài jiàn", []string{"bye"}, nil, []string{"other"})

	// Acknowledge both words so they are counted in the accuracy-bucket breakdown.
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": tagged})
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": other})

	// Unfiltered: both words fall in the "new" bucket (still in the learning phase).
	rec := do(t, r, "GET", "/api/quiz/word-stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("unfiltered: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var all models.WordStatsResponse
	decodeJSON(t, rec, &all)
	if all.AccBuckets["new"] != 2 {
		t.Errorf("unfiltered new bucket: want 2, got %d", all.AccBuckets["new"])
	}

	// Tag-filtered: only the "vip"-tagged word is counted.
	rec = do(t, r, "GET", "/api/quiz/word-stats?tags=vip", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("tag-filtered: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var filtered models.WordStatsResponse
	decodeJSON(t, rec, &filtered)
	if filtered.AccBuckets["new"] != 1 {
		t.Errorf("tag-filtered new bucket: want 1, got %d", filtered.AccBuckets["new"])
	}
	if filtered.TotalSeen != all.TotalSeen {
		t.Errorf("total_seen should be unaffected by tag filter: want %d, got %d", all.TotalSeen, filtered.TotalSeen)
	}
}

// TestStatsHasUnseen verifies that has_unseen is 1 even when the daily new-word
// cap is fully consumed, so the success screen can still show the
// "introduce new words today" button.
func TestStatsHasUnseen_CapExhausted(t *testing.T) {
	s := openTestDB(t)

	// Seed 6 words; acknowledge 5 (default cap) leaving 1 unseen.
	words := []int64{
		seedWord(t, s, "一", "", []string{"one"}),
		seedWord(t, s, "二", "", []string{"two"}),
		seedWord(t, s, "三", "", []string{"three"}),
		seedWord(t, s, "四", "", []string{"four"}),
		seedWord(t, s, "五", "", []string{"five"}),
	}
	seedWord(t, s, "六", "", []string{"six"}) // unseen — beyond cap

	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 5}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Post("/api/quiz/acknowledge", quizH.Acknowledge)
	r.Get("/api/quiz/stats", quizH.Stats)

	for _, id := range words {
		rec := do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": id})
		if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
			t.Fatalf("acknowledge %d: want 2xx, got %d: %s", id, rec.Code, rec.Body.String())
		}
	}

	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)

	if resp["new_available"] != 0 {
		t.Errorf("new_available: want 0 (cap exhausted), got %d", resp["new_available"])
	}
	if resp["has_unseen"] != 1 {
		t.Errorf("has_unseen: want 1 (三 is still unseen), got %d", resp["has_unseen"])
	}
}

func TestStatsHandlerNewFields(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)

	for _, key := range []string{"today_attempts", "today_mistakes", "available_to_advance", "new_available", "has_unseen", "hmm_due_today", "words_improved_today"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("stats response missing key %q", key)
		}
	}
}

// The Stats endpoint reports how many words moved up a proficiency bucket
// today relative to the closest prior day's daily_stats snapshot.
func TestStatsHandler_WordsImprovedToday(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	if err := s.SeedDailyStatBucketsForTest(ctx, userID, "date('now', '-1 day')", 0, 2, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedDailyStatBucketsForTest(ctx, userID, "date('now')", 0, 1, 1, 0, 0); err != nil {
		t.Fatal(err)
	}

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["words_improved_today"] != 1 {
		t.Errorf("words_improved_today: want 1, got %d", resp["words_improved_today"])
	}
}

func TestStatsHandler_HmmDueTodayIncluded(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)

	if _, ok := resp["hmm_due_today"]; !ok {
		t.Error("stats response missing key \"hmm_due_today\"")
	}
	// With an empty DB, hmm_due_today should be 0.
	if resp["hmm_due_today"] != 0 {
		t.Errorf("hmm_due_today: want 0, got %d", resp["hmm_due_today"])
	}
}

func TestQuizStats_MnemonicsFalse_HmmDueTodayZero(t *testing.T) {
	s := openTestDB(t)
	seedHMMCard(t, s)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/stats?mnemonics=false", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if got := resp["hmm_due_today"]; got != 0 {
		t.Errorf("hmm_due_today: want 0, got %d", got)
	}
}

func TestQuizStats_MnemonicsTrue_HmmDueTodayNonZero(t *testing.T) {
	s := openTestDB(t)
	seedHMMCard(t, s)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if got := resp["hmm_due_today"]; got == 0 {
		t.Error("hmm_due_today: want >0 after seeding an HMM card, got 0")
	}
}

func TestStatsHandlerNewAvailable(t *testing.T) {
	s := openTestDB(t)
	// Use MaxNewPerDay=0 so new words are blocked by default.
	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 0}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/quiz/stats", quizH.Stats)
	r.Post("/api/quiz/advance", quizH.Advance)
	ctx := context.Background()

	// Seed an unseen word.
	if _, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{ZhText: "未见", Translations: map[string][]string{"en": {"unseen"}}}); err != nil {
		t.Fatal(err)
	}

	// Before cap reset: new_available should be 0 (cap=0 blocks new words).
	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["new_available"] != 0 {
		t.Errorf("new_available before cap reset: got %d, want 0", resp["new_available"])
	}

	// Reset cap.
	rec = do(t, r, "POST", "/api/quiz/advance", map[string]any{"count": 0, "reset_new_cap": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// After cap reset: new_available should be 1.
	rec = do(t, r, "GET", "/api/quiz/stats", nil)
	decodeJSON(t, rec, &resp)
	if resp["new_available"] != 1 {
		t.Errorf("new_available after cap reset: got %d, want 1", resp["new_available"])
	}
}

func TestStatsNewAvailable_WithLearningWords(t *testing.T) {
	s := openTestDB(t)
	// Cap of 5; seed 3 unseen words, acknowledge 1 (puts it in learning phase).
	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 5}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/quiz/stats", quizH.Stats)
	ctx := context.Background()

	ids := make([]int64, 3)
	for i, zh := range []string{"红", "蓝", "绿"} {
		wid, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{ZhText: zh, Translations: map[string][]string{"en": {"color"}}})
		if err != nil {
			t.Fatalf("CreateWord: %v", err)
		}
		ids[i] = wid
	}

	// Acknowledge one word — it enters the learning phase (learning_new_word=1).
	if err := s.AcknowledgeWord(ctx, int64(2), ids[0]); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	// new_available should still reflect the 2 remaining unseen words,
	// not be gated to 0 by the learning word.
	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["new_available"] != 2 {
		t.Errorf("want new_available=2 even with a learning word present, got %d", resp["new_available"])
	}
}

func TestDueDateDistribution_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/quiz/due-date-distribution", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp models.DueDateDistributionResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Dates) != 0 {
		t.Errorf("expected empty dates, got %d", len(resp.Dates))
	}
}

func TestDueDateDistribution_AfterAnswer(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "猫", "māo", []string{"cat"})
	r := newRouter(s)

	// Present the word via /next and then acknowledge to set first_seen_date
	rec := do(t, r, "GET", "/api/quiz/next", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("next: want 200, got %d", rec.Code)
	}

	// Acknowledge (sets first_seen_date) and answer the word
	rec = do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": id})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("acknowledge: want 204, got %d", rec.Code)
	}
	rec = do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": id,
		"mode":    "zh_to_transl",
		"answer":  "cat",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("answer: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, "GET", "/api/quiz/due-date-distribution", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp models.DueDateDistributionResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Dates) == 0 {
		t.Fatal("expected at least one date entry")
	}
	total := 0
	for _, d := range resp.Dates {
		total += d.Count
	}
	if total != 1 {
		t.Errorf("expected total count 1, got %d", total)
	}
}

func TestDueDateDistribution_TagFilter(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Create two words with different tags
	id1, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText: "猫", Pinyin: "māo", Translations: map[string][]string{"en": {"cat"}}, Tags: []string{"animals"},
	})
	if err != nil {
		t.Fatalf("create word 1: %v", err)
	}
	id2, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText: "书", Pinyin: "shū", Translations: map[string][]string{"en": {"book"}}, Tags: []string{"objects"},
	})
	if err != nil {
		t.Fatalf("create word 2: %v", err)
	}

	r := newRouter(s)

	// Present and acknowledge+answer both words so first_seen_date is set
	for _, wid := range []int64{id1, id2} {
		rec := do(t, r, "GET", "/api/quiz/next", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("next for word %d: want 200, got %d", wid, rec.Code)
		}
		rec = do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": wid})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("acknowledge word %d: want 204, got %d", wid, rec.Code)
		}
		rec = do(t, r, "POST", "/api/quiz/answer", map[string]any{
			"word_id": wid,
			"mode":    "zh_to_transl",
			"answer":  "wrong",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("answer word %d: want 200, got %d", wid, rec.Code)
		}
	}

	// Without filter: should see 2 words
	rec := do(t, r, "GET", "/api/quiz/due-date-distribution", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp models.DueDateDistributionResponse
	decodeJSON(t, rec, &resp)
	total := 0
	for _, d := range resp.Dates {
		total += d.Count
	}
	if total != 2 {
		t.Errorf("unfiltered: expected total 2, got %d", total)
	}

	// With animals tag filter: should see 1 word
	rec = do(t, r, "GET", "/api/quiz/due-date-distribution?tags=animals", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var filtered models.DueDateDistributionResponse
	decodeJSON(t, rec, &filtered)
	filteredTotal := 0
	for _, d := range filtered.Dates {
		filteredTotal += d.Count
	}
	if filteredTotal != 1 {
		t.Errorf("filtered by 'animals': expected total 1, got %d", filteredTotal)
	}
}

func TestQuizStats_IncludesComponentCounts(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	past := time.Now().Add(-24 * time.Hour)
	// Insert component directly — this test is about stats, not InitComponentsForWord.
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", past)
	s.SetComponentSeenForTest(context.Background(), int64(2), "女")

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/quiz/stats?trainComponents=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if v, _ := resp["components_total"].(float64); int(v) != 1 {
		t.Errorf("want components_total=1, got %v", resp["components_total"])
	}
	if v, _ := resp["components_due_today"].(float64); int(v) != 1 {
		t.Errorf("want components_due_today=1, got %v", resp["components_due_today"])
	}
}

// TestQuizStats_ComponentCounts_MatchesNonEnglishLang reproduces issues
// #230/#232: a user training in German saw "Due today: 0" while the /next
// endpoint kept serving a due component whose only translation was German.
// GetComponentCounts must honor the same langs the Next handler uses instead
// of hardcoding EN, so due-today reflects what is actually served.
func TestQuizStats_ComponentCounts_MatchesNonEnglishLang(t *testing.T) {
	s := openTestDB(t)
	if _, err := s.ExecForTest(`INSERT INTO hanzi_decomposition (character, definition) VALUES ('女', 'woman')`); err != nil {
		t.Fatalf("seed hanzi_decomposition: %v", err)
	}
	if err := s.SeedHanziTranslationForTest(context.Background(), "女", "de", "Frau"); err != nil {
		t.Fatalf("SeedHanziTranslationForTest: %v", err)
	}
	past := time.Now().Add(-24 * time.Hour)
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", past)
	s.SetComponentSeenForTest(context.Background(), int64(2), "女")

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/quiz/stats?trainComponents=1&langs=de", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if v, _ := resp["components_due_today"].(float64); int(v) != 1 {
		t.Errorf("want components_due_today=1 for a de-only due component when langs=de, got %v", resp["components_due_today"])
	}
}

// TestQuizStats_MaxNewPerDay_ReflectsUserSetting verifies that the stats
// endpoint returns the per-user max_new_words_per_day, not the server default.
func TestQuizStats_MaxNewPerDay_ReflectsUserSetting(t *testing.T) {
	s := openTestDB(t)
	// Server default is 100 (set in newRouter via MaxNewPerDay: 100).
	// Patch the user setting to 3; stats must report 3, not 100.
	r := newRouter(s)

	type patchPayload struct {
		PrimaryLang        string `json:"primary_lang"`
		ProgNew            string `json:"prog_new"`
		ProgTierStruggling string `json:"prog_tier_struggling"`
		ProgTierLearning   string `json:"prog_tier_learning"`
		ProgTierPracticing string `json:"prog_tier_practicing"`
		ProgTierMastered   string `json:"prog_tier_mastered"`
		NewWordMode0       string `json:"new_word_mode_0"`
		NewWordMode1       string `json:"new_word_mode_1"`
		NewWordMode2       string `json:"new_word_mode_2"`
		MaxNewWordsPerDay  int    `json:"max_new_words_per_day"`
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", patchPayload{
		PrimaryLang:        "en",
		ProgNew:            "transl_to_zh",
		ProgTierStruggling: "transl_to_zh",
		ProgTierLearning:   "zh_pinyin_to_transl",
		ProgTierPracticing: "zh_to_transl",
		ProgTierMastered:   "random",
		NewWordMode0:       "transl_to_zh",
		NewWordMode1:       "transl_to_zh",
		NewWordMode2:       "zh_to_transl",
		MaxNewWordsPerDay:  3,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /api/settings: want 200, got %d: %s", rec.Code, rec.Body)
	}

	rec = do(t, r, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/quiz/stats: want 200, got %d", rec.Code)
	}
	var stats map[string]int
	decodeJSON(t, rec, &stats)
	if stats["max_new_per_day"] != 3 {
		t.Errorf("max_new_per_day: want 3 (user setting), got %d", stats["max_new_per_day"])
	}
}
