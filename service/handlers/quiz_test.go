package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

// enableSentenceBlank fetches the user's current settings, flips the
// sentence-blank fields, and saves them back — mirroring how the frontend
// settings page PATCHes the full settings object.
func enableSentenceBlank(t *testing.T, s *db.Store, userID int64, enabled bool, ratio int) {
	t.Helper()
	ctx := context.Background()
	st, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	st.SentenceBlankEnabled = enabled
	st.SentenceBlankRatio = ratio
	if err := s.UpdateUserSettings(ctx, userID, *st); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
}

// seedSentenceScenario seeds three acknowledged component words plus a
// sentence word (我买牛奶, tagged s_test, left unacknowledged) that fully
// segments from them — the shared fixture for sentence-blank handler tests.
func seedSentenceScenario(t *testing.T, s *db.Store, userID int64) (wo, mai, niunai int64) {
	t.Helper()
	ctx := context.Background()
	wo = seedWordFull(t, s, userID, "我", "wǒ", []string{"I"}, nil, nil)
	mai = seedWordFull(t, s, userID, "买", "mǎi", []string{"buy"}, nil, nil)
	niunai = seedWordFull(t, s, userID, "牛奶", "niú nǎi", []string{"milk"}, nil, nil)
	for _, id := range []int64{wo, mai, niunai} {
		if err := s.AcknowledgeWord(ctx, userID, id); err != nil {
			t.Fatalf("AcknowledgeWord %d: %v", id, err)
		}
	}
	seedWordFull(t, s, userID, "我买牛奶", "wǒ mǎi niú nǎi", []string{"I buy milk"}, nil, []string{"s_test"})
	return
}

// makeDifficultWord acknowledges a word (so first_seen_date is set) then writes a
// graduated, low-accuracy progress row so it qualifies for the difficult drill.
func makeDifficultWord(t *testing.T, s *db.Store, r http.Handler, id int64, tc, ta int, ef float64) {
	t.Helper()
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]any{"word_id": id})
	err := s.UpdateSM2Progress(context.Background(), models.SM2Progress{
		WordID:          id,
		Repetitions:     0,
		Easiness:        ef,
		IntervalDays:    1,
		DueDate:         time.Now().UTC().Add(24 * time.Hour),
		TotalCorrect:    tc,
		TotalAttempts:   ta,
		StreakBonus:     0,
		LearningNewWord: false,
	})
	if err != nil {
		t.Fatalf("makeDifficultWord(%d): %v", id, err)
	}
}

func TestQuizSkip_Valid(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)
	ctx := context.Background()

	beforeP, _ := s.GetSM2Progress(ctx, id)

	rec := do(t, r, "POST", "/api/quiz/skip", map[string]int64{"word_id": id})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	afterP, _ := s.GetSM2Progress(ctx, id)
	if afterP.TotalAttempts != beforeP.TotalAttempts {
		t.Error("skip should not change total_attempts")
	}
	if !afterP.DueDate.After(beforeP.DueDate) {
		t.Error("skip should move due_date forward")
	}
}

func TestQuizSkip_DaysOne(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)
	ctx := context.Background()

	beforeP, _ := s.GetSM2Progress(ctx, id)

	rec := do(t, r, "POST", "/api/quiz/skip", map[string]any{"word_id": id, "days": 1})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	afterP, _ := s.GetSM2Progress(ctx, id)
	if afterP.TotalAttempts != beforeP.TotalAttempts {
		t.Error("skip should not change total_attempts")
	}
	delta := afterP.DueDate.Sub(time.Now())
	if delta < 23*time.Hour || delta > 25*time.Hour {
		t.Errorf("days=1 should move due_date ~24h ahead, got delta=%v", delta)
	}
}

func TestQuizSkip_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/quiz/skip", map[string]int64{"word_id": 9999})
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestQuizAcknowledge_Valid(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)
	ctx := context.Background()

	rec := do(t, r, "POST", "/api/quiz/acknowledge", map[string]int64{"word_id": id})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	p, _ := s.GetSM2Progress(ctx, id)
	if p.TotalAttempts != 1 {
		t.Errorf("total_attempts: want 1, got %d", p.TotalAttempts)
	}
	if p.TotalCorrect != 0 {
		t.Errorf("total_correct: want 0, got %d", p.TotalCorrect)
	}
	if !p.LearningNewWord {
		t.Error("acknowledge must set learning_new_word=true so the word enters the learning phase")
	}
}

func TestQuizAcknowledge_Idempotent(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)
	ctx := context.Background()

	// Acknowledge twice — should not increment total_attempts beyond 1
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]int64{"word_id": id})
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]int64{"word_id": id})

	p, _ := s.GetSM2Progress(ctx, id)
	if p.TotalAttempts != 1 {
		t.Errorf("total_attempts after double acknowledge: want 1, got %d", p.TotalAttempts)
	}
}

func TestQuizAcknowledge_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/quiz/acknowledge", map[string]int64{"word_id": 9999})
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestQuizAcknowledge_CreatesComponentProgress(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed 女 as a component (definition only).
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed component: %v", err)
	}
	// Seed 妈 with definition and a decomposition that contains 女.
	if err := s.SeedHanziDecompositionWithDecompForTest(ctx, "妈", "mother", "⿰女马"); err != nil {
		t.Fatalf("seed char: %v", err)
	}

	id := seedWord(t, s, "妈妈", "māmā", []string{"mother"})
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/acknowledge", map[string]int64{"word_id": id})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	_, total, err := s.GetComponentCounts(ctx, int64(2), nil)
	if err != nil {
		t.Fatalf("GetComponentCounts: %v", err)
	}
	if total == 0 {
		t.Error("expected component_progress rows after acknowledge")
	}
}

func TestRecordTime_AccumulatesAndAppearsInDailyStats(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/record-time", map[string]any{"seconds": 60})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, "POST", "/api/quiz/record-time", map[string]any{"seconds": 30})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body.String())
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
	if resp.Days[0].TrainingSeconds != 90 {
		t.Errorf("training_seconds: want 90, got %d", resp.Days[0].TrainingSeconds)
	}
}

func TestRecordTime_RejectsInvalidSeconds(t *testing.T) {
	r := newRouter(openTestDB(t))

	for _, secs := range []int{0, -1, 3601} {
		rec := do(t, r, "POST", "/api/quiz/record-time", map[string]any{"seconds": secs})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("seconds=%d: want 400, got %d", secs, rec.Code)
		}
	}
}

func TestAdvanceHandler_NoWordsAvailable(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// No seen words — advance should return 0 without error.
	rec := do(t, r, "POST", "/api/quiz/advance", map[string]any{"count": 10, "reset_new_cap": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["advanced"].(float64) != 0 {
		t.Errorf("expected advanced=0, got %v", resp["advanced"])
	}
}

func TestAdvanceHandler_AdvancesWords(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	ctx := context.Background()

	// Seed a word, acknowledge it (marks as seen, due_date = now), then skip
	// it forward so it has a future due date.
	wid, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{ZhText: "测试", Translations: map[string][]string{"en": {"test"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeWord(ctx, int64(2), wid); err != nil {
		t.Fatal(err)
	}
	if err := s.SkipWord(ctx, int64(2), wid, 1); err != nil {
		t.Fatal(err)
	}

	rec := do(t, r, "POST", "/api/quiz/advance", map[string]any{"count": 1, "reset_new_cap": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["advanced"].(float64) != 1 {
		t.Errorf("expected advanced=1, got %v", resp["advanced"])
	}
}

func TestAdvanceHandler_ResetCapReflectedInNext(t *testing.T) {
	s := openTestDB(t)
	// Use a handler with MaxNewPerDay=0 so new words are normally blocked.
	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 0}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/quiz/next", quizH.Next)
	r.Post("/api/quiz/advance", quizH.Advance)
	ctx := context.Background()

	// Seed a word (unseen).
	if _, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{ZhText: "新词", Translations: map[string][]string{"en": {"new word"}}}); err != nil {
		t.Fatal(err)
	}

	// With cap=0 and no reset, next should return no words.
	rec := do(t, r, "GET", "/api/quiz/next", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 before cap reset, got %d", rec.Code)
	}

	// Reset cap.
	rec = do(t, r, "POST", "/api/quiz/advance", map[string]any{"count": 0, "reset_new_cap": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Now next should return the unseen word.
	rec = do(t, r, "GET", "/api/quiz/next", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after cap reset, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAcknowledgeRandomHandler_MarksWordsDue(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	ctx := context.Background()

	// Seed 5 unseen words for user 2.
	for i, zh := range []string{"一", "二", "三", "四", "五"} {
		if _, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{ZhText: zh, Translations: map[string][]string{"en": {"word" + string(rune('a'+i))}}}); err != nil {
			t.Fatalf("CreateWord %s: %v", zh, i)
		}
	}

	// due_today should be 0 before.
	stats := do(t, r, "GET", "/api/quiz/stats", nil)
	var s0 map[string]int
	decodeJSON(t, stats, &s0)
	if s0["due_today"] != 0 {
		t.Fatalf("expected due_today=0 before, got %d", s0["due_today"])
	}

	// Acknowledge 3 random words.
	rec := do(t, r, "POST", "/api/quiz/acknowledge-random", map[string]any{"count": 3})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["acknowledged"] != 3 {
		t.Errorf("want acknowledged=3, got %d", resp["acknowledged"])
	}

	// due_today should now be 3.
	stats = do(t, r, "GET", "/api/quiz/stats", nil)
	var s1 map[string]int
	decodeJSON(t, stats, &s1)
	if s1["due_today"] != 3 {
		t.Errorf("want due_today=3, got %d", s1["due_today"])
	}
}

func TestAcknowledgeRandomHandler_CapsAtAvailable(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	ctx := context.Background()

	// Seed only 2 unseen words.
	for _, zh := range []string{"甲", "乙"} {
		if _, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{ZhText: zh, Translations: map[string][]string{"en": {"word"}}}); err != nil {
			t.Fatalf("CreateWord %s: %v", zh, err)
		}
	}

	// Ask for 10, should only acknowledge the 2 available.
	rec := do(t, r, "POST", "/api/quiz/acknowledge-random", map[string]any{"count": 10})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["acknowledged"] != 2 {
		t.Errorf("want acknowledged=2, got %d", resp["acknowledged"])
	}
}

// TestAcknowledgeRandomHandler_RespectsNewWordCap reproduces issue #344: bulk
// onboarding import (which drives acknowledge-random with a large count, e.g.
// the "start training with 20 words" onboarding option) must not bypass the
// same daily new-word pacing cap that governs manually-added words. Only up
// to the user's max_new_words_per_day should be force-acknowledged as
// immediately due; the rest must stay unseen so GetNextCard introduces them
// gradually, exactly like organically-created new words.
func TestAcknowledgeRandomHandler_RespectsNewWordCap(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	ctx := context.Background()

	// Seed 20 unseen words for user 2 — mirroring a bulk HSK1 import.
	for i := 0; i < 20; i++ {
		zh := string(rune('一' + i))
		if _, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
			ZhText:       zh,
			Translations: map[string][]string{"en": {"word"}},
		}); err != nil {
			t.Fatalf("CreateWord %s: %v", zh, err)
		}
	}

	// The default per-user daily new-word cap is 5 (see ensureUserSettings).
	rec := do(t, r, "POST", "/api/quiz/acknowledge-random", map[string]any{"count": 20})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["acknowledged"] != 5 {
		t.Errorf("want acknowledged capped at the daily new-word limit (5), got %d", resp["acknowledged"])
	}

	stats := do(t, r, "GET", "/api/quiz/stats", nil)
	var s1 map[string]int
	decodeJSON(t, stats, &s1)
	if s1["due_today"] != 5 {
		t.Errorf("want due_today=5 (not all 20 flooding the session), got %d", s1["due_today"])
	}
	if s1["new_today"] != 5 {
		t.Errorf("want new_today=5, got %d", s1["new_today"])
	}
}

func TestAcknowledgeRandomHandler_InvalidCount(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "POST", "/api/quiz/acknowledge-random", map[string]any{"count": 0})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for count=0, got %d", rec.Code)
	}
}

func TestAcknowledgeRandomHandler_CreatesComponentProgress(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed 女 as a component (definition only).
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed component: %v", err)
	}
	// Seed 妈 with decomposition containing 女.
	if err := s.SeedHanziDecompositionWithDecompForTest(ctx, "妈", "mother", "⿰女马"); err != nil {
		t.Fatalf("seed char: %v", err)
	}

	if _, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "妈",
		Translations: map[string][]string{"en": {"mother"}},
	}); err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/quiz/acknowledge-random", map[string]any{"count": 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	_, total, err := s.GetComponentCounts(ctx, int64(2), nil)
	if err != nil {
		t.Fatalf("GetComponentCounts: %v", err)
	}
	if total == 0 {
		t.Error("expected component_progress rows after acknowledge-random")
	}
}

func TestQuizLangs_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/quiz/langs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var langs []string
	decodeJSON(t, rec, &langs)
	if len(langs) != 0 {
		t.Errorf("expected empty langs, got %v", langs)
	}
}

func TestQuizLangs_AfterInsertEN(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/langs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var langs []string
	decodeJSON(t, rec, &langs)
	if len(langs) != 1 || langs[0] != "en" {
		t.Errorf("expected [en], got %v", langs)
	}
}

func TestQuizLangs_ENandDE(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	_, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello"}, "de": {"hallo"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/langs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var langs []string
	decodeJSON(t, rec, &langs)
	if len(langs) != 2 {
		t.Fatalf("expected 2 langs, got %v", langs)
	}
	// Sorted: de, en
	if langs[0] != "de" || langs[1] != "en" {
		t.Errorf("expected [de en], got %v", langs)
	}
}

func TestQuizCycleAdvances(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	// Set total_attempts=2 directly so position=(2-1)%3=1 → transl_to_zh.
	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	p.TotalAttempts = 2
	p.TotalCorrect = 1
	p.DueDate = time.Now().UTC().Add(-time.Hour)
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatalf("UpdateSM2Progress: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/next?mode=cycle", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	// total_attempts=2 → (2-1)%3=1 → transl_to_zh
	if card.Mode != models.ModeTranslToZh {
		t.Errorf("cycle position 1: want %s, got %s", models.ModeTranslToZh, card.Mode)
	}
}

func TestQuizCycleWraps(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	// Set total_attempts=4 so position=(4-1)%3=0 in the raw 3-step default
	// sequence. The word stays in the "new" bucket (learning_new_word=true),
	// and the default random-mode-range ladder makes zh_to_transl ineligible
	// there, so the cycle sequence is filtered down to [zh_pinyin_to_transl,
	// transl_to_zh] before indexing — position (4-1)%2=1 → transl_to_zh.
	// AcknowledgeWord first to set first_seen_date (required for GetNextCard).
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}
	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	p.TotalAttempts = 4
	p.TotalCorrect = 1
	p.DueDate = time.Now().UTC().Add(-time.Hour)
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatalf("UpdateSM2Progress: %v", err)
	}

	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/next?mode=cycle", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	// total_attempts=4 → bucket-filtered 2-step sequence → (4-1)%2=1 → transl_to_zh
	if card.Mode != models.ModeTranslToZh {
		t.Errorf("cycle wrapped (bucket-filtered): want %s, got %s", models.ModeTranslToZh, card.Mode)
	}
}

func TestQuizCycleCustomSequence(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	r := newRouter(s)

	// Set a custom 2-step cycle sequence.
	patchPayload := map[string]string{
		"primary_lang":         "en",
		"secondary_lang":       "",
		"prog_new":             "transl_to_zh",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "transl_to_zh",
		"new_word_mode_2":      "zh_to_transl",
		"cycle_sequence":       "transl_to_zh,zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", patchPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	rec = do(t, r, "GET", "/api/quiz/next?mode=cycle", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	// Custom sequence starts with transl_to_zh → position 0 = transl_to_zh
	if card.Mode != models.ModeTranslToZh {
		t.Errorf("custom cycle position 0: want %s, got %s", models.ModeTranslToZh, card.Mode)
	}
}

func TestQuizCycleMode_NoLearningPinyinHint(t *testing.T) {
	// transl_to_zh in cycle mode must NOT expose the learning-phase pinyin hint,
	// even when the word is still in the intro phase (LearningNewWord=true).
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}
	// Word is now LearningNewWord=true, TotalCorrect=0.

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/next?mode=cycle", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	// Step 0 of default cycle is zh_pinyin_to_transl — that's fine to have pinyin.
	// Advance to step 1 (transl_to_zh) by bumping TotalAttempts to 2.
	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	p.TotalAttempts = 2
	p.DueDate = p.DueDate.Add(-time.Hour)
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatalf("UpdateSM2Progress: %v", err)
	}

	rec = do(t, r, "GET", "/api/quiz/next?mode=cycle", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for cycle step 1, got %d: %s", rec.Code, rec.Body.String())
	}
	decodeJSON(t, rec, &card)
	if card.Mode != models.ModeTranslToZh {
		t.Fatalf("cycle step 1: want %s, got %s", models.ModeTranslToZh, card.Mode)
	}
	if card.Pinyin != nil {
		t.Errorf("cycle transl_to_zh step must not include pinyin hint, got %q", *card.Pinyin)
	}
}

func TestQuizCycle_AdvanceOnSuccessOnly(t *testing.T) {
	// When cycle_advance_on_success_only=true, the cycle position is driven by
	// TotalCorrect rather than TotalAttempts. A word with TotalAttempts=3 but
	// TotalCorrect=1 should show position (1-1)%3=0, not (3-1)%3=2.
	s := openTestDB(t)
	ctx := context.Background()
	r := newRouter(s)

	// Enable the setting.
	patchPayload := map[string]interface{}{
		"primary_lang":                  "en",
		"secondary_lang":                "",
		"prog_new":                      "transl_to_zh",
		"prog_tier_struggling":          "transl_to_zh",
		"prog_tier_learning":            "zh_pinyin_to_transl",
		"prog_tier_practicing":          "zh_to_transl",
		"prog_tier_mastered":            "random",
		"new_word_mode_0":               "transl_to_zh",
		"new_word_mode_1":               "transl_to_zh",
		"new_word_mode_2":               "zh_to_transl",
		"cycle_sequence":                "zh_pinyin_to_transl,transl_to_zh,zh_to_transl",
		"cycle_advance_on_success_only": true,
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", patchPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	// Set TotalAttempts=3, TotalCorrect=1: without the flag → position 2; with it → position 0.
	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	p.TotalAttempts = 3
	p.TotalCorrect = 1
	p.DueDate = time.Now().UTC().Add(-time.Hour)
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatalf("UpdateSM2Progress: %v", err)
	}

	rec = do(t, r, "GET", "/api/quiz/next?mode=cycle", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	// TotalCorrect=1 → (1-1)%3=0 → zh_pinyin_to_transl (step 0)
	if card.Mode != models.ModeZhPinyinToTransl {
		t.Errorf("advance_on_success_only: want %s (pos 0), got %s", models.ModeZhPinyinToTransl, card.Mode)
	}
}

func TestSkip_RejectsNewWordWhenHidden(t *testing.T) {
	s := openTestDB(t)
	wordID := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	ctx := context.Background()

	// Mark word as new (learning_new_word=1, first_seen_date=today)
	if err := s.AcknowledgeWord(ctx, 2, wordID); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	r := newRouter(s)

	// First disable skip via settings
	type patchPayload struct {
		PrimaryLang         string `json:"primary_lang"`
		SecondaryLang       string `json:"secondary_lang"`
		ProgNew             string `json:"prog_new"`
		ProgTierStruggling  string `json:"prog_tier_struggling"`
		ProgTierLearning    string `json:"prog_tier_learning"`
		ProgTierPracticing  string `json:"prog_tier_practicing"`
		ProgTierMastered    string `json:"prog_tier_mastered"`
		NewWordMode0        string `json:"new_word_mode_0"`
		NewWordMode1        string `json:"new_word_mode_1"`
		NewWordMode2        string `json:"new_word_mode_2"`
		SkipNewWordsVisible bool   `json:"skip_new_words_visible"`
		MaxNewWordsPerDay   int    `json:"max_new_words_per_day"`
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", patchPayload{
		PrimaryLang:         "en",
		SecondaryLang:       "de",
		ProgNew:             "zh_to_transl",
		ProgTierStruggling:  "transl_to_zh",
		ProgTierLearning:    "zh_pinyin_to_transl",
		ProgTierPracticing:  "zh_to_transl",
		ProgTierMastered:    "random",
		NewWordMode0:        "transl_to_zh",
		NewWordMode1:        "zh_pinyin_to_transl",
		NewWordMode2:        "zh_to_transl",
		SkipNewWordsVisible: false,
		MaxNewWordsPerDay:   5,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch settings: want 200, got %d: %s", rec.Code, rec.Body)
	}

	// Attempt to skip the new word — should be rejected
	rec = do(t, r, http.MethodPost, "/api/quiz/skip", map[string]any{
		"word_id": wordID,
		"days":    7,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 when skipping new word with skip hidden, got %d: %s", rec.Code, rec.Body)
	}
}

// TestQuizAnswer_EllipsisEquivalence covers issue #343: a zh word stored
// with ideographic ellipses ("……") must be accepted when the user types an
// equivalent ellipsis form ("..." or "。。。") while answering a training
// card, in both directions (typing the zh word, and typing its
// translation containing an ellipsis).
func TestQuizAnswer_EllipsisEquivalence(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	id := seedWord(t, s, "虽然……但是……", "suīrán... dànshì...", []string{"although... but..."})

	for _, tc := range []struct {
		name   string
		mode   string
		answer string
	}{
		{"zh word, ASCII dots", "transl_to_zh", "虽然...但是..."},
		{"zh word, fullwidth periods", "transl_to_zh", "虽然。。。但是。。。"},
		{"zh word, exact stored form", "transl_to_zh", "虽然……但是……"},
		{"translation, ideographic ellipsis", "zh_to_transl", "although…but…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
				"word_id": id, "mode": tc.mode, "answer": tc.answer,
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("answer: want 200, got %d: %s", rec.Code, rec.Body.String())
			}
			var resp struct {
				Correct bool `json:"correct"`
			}
			decodeJSON(t, rec, &resp)
			if !resp.Correct {
				t.Errorf("answer %q (mode %s) should be accepted as correct", tc.answer, tc.mode)
			}
		})
	}
}

func TestFlagDifficult_FlagsServesAndClearsOnCorrect(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	id1 := seedWord(t, s, "山", "shān", []string{"mountain"})
	id2 := seedWord(t, s, "河", "hé", []string{"river"})
	makeDifficultWord(t, s, r, id1, 1, 10, 1.3) // ~10% accuracy
	makeDifficultWord(t, s, r, id2, 2, 10, 1.4) // ~20% accuracy

	// Flag the difficult words.
	rec := do(t, r, "POST", "/api/quiz/difficult", map[string]any{"count": 10})
	if rec.Code != http.StatusOK {
		t.Fatalf("flag difficult: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var flagResp struct {
		Flagged int `json:"flagged"`
	}
	decodeJSON(t, rec, &flagResp)
	if flagResp.Flagged != 2 {
		t.Fatalf("expected 2 flagged, got %d", flagResp.Flagged)
	}

	// The drill serves a flagged word despite it being due tomorrow.
	rec = do(t, r, "GET", "/api/quiz/next?difficult=true&mode=zh_to_transl&langs=en", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("difficult next: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.WordID != id1 && card.WordID != id2 {
		t.Fatalf("expected a flagged word, got word_id=%d", card.WordID)
	}

	// stats reports the remaining drill pool.
	rec = do(t, r, "GET", "/api/quiz/stats", nil)
	var stats map[string]int
	decodeJSON(t, rec, &stats)
	if stats["difficult_remaining"] != 2 {
		t.Fatalf("expected difficult_remaining=2, got %d", stats["difficult_remaining"])
	}

	// Answering the served word correctly clears its flag.
	rec = do(t, r, "POST", "/api/quiz/answer", map[string]any{
		"word_id": card.WordID, "mode": "zh_to_transl", "answer": map[int64]string{id1: "mountain", id2: "river"}[card.WordID],
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("answer: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, r, "GET", "/api/quiz/stats", nil)
	decodeJSON(t, rec, &stats)
	if stats["difficult_remaining"] != 1 {
		t.Fatalf("expected difficult_remaining=1 after a correct answer, got %d", stats["difficult_remaining"])
	}
}

func TestFlagDifficult_InvalidCount(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, "POST", "/api/quiz/difficult", map[string]any{"count": 0})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for count=0, got %d", rec.Code)
	}
}

func TestClearDifficult_EmptiesPoolAndDrillReturns404(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	id := seedWord(t, s, "山", "shān", []string{"mountain"})
	makeDifficultWord(t, s, r, id, 1, 10, 1.3)

	do(t, r, "POST", "/api/quiz/difficult", map[string]any{"count": 5})
	rec := do(t, r, "POST", "/api/quiz/difficult/clear", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear difficult: want 200, got %d", rec.Code)
	}
	rec = do(t, r, "GET", "/api/quiz/stats", nil)
	var stats map[string]int
	decodeJSON(t, rec, &stats)
	if stats["difficult_remaining"] != 0 {
		t.Fatalf("expected difficult_remaining=0 after clear, got %d", stats["difficult_remaining"])
	}
	// No flagged words → the drill reports no words available.
	rec = do(t, r, "GET", "/api/quiz/next?difficult=true", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty drill, got %d: %s", rec.Code, rec.Body.String())
	}
}
