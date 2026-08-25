package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

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

func TestQuizNext_SentenceBlank_ServedWhenEnabledAndEligible(t *testing.T) {
	s := openTestDB(t)
	wo, mai, niunai := seedSentenceScenario(t, s, 2)
	enableSentenceBlank(t, s, 2, true, 100)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/next?langs=en", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.CardType != "sentence" {
		t.Fatalf("want card_type=sentence, got %q (body: %s)", card.CardType, rec.Body)
	}
	if card.WordID != wo && card.WordID != mai && card.WordID != niunai {
		t.Errorf("word_id %d is not one of the seeded component words", card.WordID)
	}
	if !strings.Contains(card.SentenceBlank, "___") {
		t.Errorf("sentence_blank should contain a blank marker, got %q", card.SentenceBlank)
	}
}

func TestQuizNext_SentenceBlank_NeverServedWhenDisabled(t *testing.T) {
	s := openTestDB(t)
	seedSentenceScenario(t, s, 2)
	enableSentenceBlank(t, s, 2, false, 100)
	r := newRouter(s)

	for i := 0; i < 10; i++ {
		rec := do(t, r, "GET", "/api/quiz/next?langs=en", nil)
		var card models.QuizCard
		decodeJSON(t, rec, &card)
		if card.CardType == "sentence" {
			t.Fatal("sentence card must never be served when sentence_blank_enabled=false")
		}
	}
}

func TestQuizNext_SentenceBlank_NeverServedWhenRatioZero(t *testing.T) {
	s := openTestDB(t)
	seedSentenceScenario(t, s, 2)
	enableSentenceBlank(t, s, 2, true, 0)
	r := newRouter(s)

	for i := 0; i < 10; i++ {
		rec := do(t, r, "GET", "/api/quiz/next?langs=en", nil)
		var card models.QuizCard
		decodeJSON(t, rec, &card)
		if card.CardType == "sentence" {
			t.Fatal("sentence card must never be served when sentence_blank_ratio=0")
		}
	}
}

func TestQuizNext_NoPinyinFallsBackMode(t *testing.T) {
	s := openTestDB(t)
	// Word with no pinyin — zh_pinyin_to_en must never be returned
	_, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       "你好",
		Pinyin:       "", // no pinyin
		Translations: map[string][]string{"en": {"hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := newRouter(s)

	for i := 0; i < 30; i++ {
		rec := do(t, r, "GET", "/api/quiz/next", nil)
		var card models.QuizCard
		decodeJSON(t, rec, &card)
		if card.Mode == models.ModeZhPinyinToTransl {
			t.Error("zh_pinyin_to_en should not be returned when pinyin is absent")
		}
	}
}

func TestQuizNext_ModeParam(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	// Give the word some attempts so it is not returned as a new_word introduction.
	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	p.TotalAttempts = 1
	p.TotalCorrect = 1
	p.DueDate = time.Now().UTC().Add(-time.Hour)
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatalf("UpdateSM2Progress: %v", err)
	}

	r := newRouter(s)

	for _, mode := range []string{models.ModeTranslToZh, models.ModeZhToTransl, models.ModeZhPinyinToTransl, models.ModeZhToTranslNoSound} {
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
	validModes := map[string]bool{models.ModeTranslToZh: true, models.ModeZhToTransl: true, models.ModeZhPinyinToTransl: true, models.ModeZhToTranslNoSound: true, models.ModeVoiceToTransl: true}
	if !validModes[card.Mode] {
		t.Errorf("invalid mode param: got unexpected mode %s", card.Mode)
	}
}

// TestQuizNext_ZhToTranslNoSound_SameShapeAsZhToTransl verifies the new mode's
// card is identical in shape to zh_to_transl (Chinese prompt, no pinyin, no
// translations in the response) — it only differs in client-side sound
// availability, never in what's asked.
func TestQuizNext_ZhToTranslNoSound_SameShapeAsZhToTransl(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	p.TotalAttempts = 1
	p.TotalCorrect = 1
	p.DueDate = time.Now().UTC().Add(-time.Hour)
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatalf("UpdateSM2Progress: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/next?mode="+models.ModeZhToTranslNoSound, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.Mode != models.ModeZhToTranslNoSound {
		t.Errorf("want card.Mode=%s, got %s", models.ModeZhToTranslNoSound, card.Mode)
	}
	if card.Prompt != "你好" {
		t.Errorf("want prompt=你好, got %q", card.Prompt)
	}
	if card.Pinyin != nil {
		t.Errorf("want no pinyin hint, got %q", *card.Pinyin)
	}
	if len(card.Translations) != 0 {
		t.Errorf("want no translations in response, got %v", card.Translations)
	}
}

func TestQuizNext_TranslToZh_IncludesZhText(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	p.TotalAttempts = 1
	p.TotalCorrect = 1
	p.DueDate = time.Now().UTC().Add(-time.Hour)
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatalf("UpdateSM2Progress: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/next?mode=transl_to_zh", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.Mode != models.ModeTranslToZh {
		t.Fatalf("want mode=transl_to_zh, got %s", card.Mode)
	}
	if card.ZhText != "你好" {
		t.Errorf("want zh_text=你好, got %q", card.ZhText)
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

	// Acknowledge id1 so it counts as today's introduced word.
	if err := s.AcknowledgeWord(ctx, int64(2), id1); err != nil {
		t.Fatalf("AcknowledgeWord id1: %v", err)
	}

	// Build a router with maxNew=1 (cap is now reached).
	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 1}
	authH, _ := handlers.NewAuthHandlerWithEnv(s, nil, "http://localhost:8080", "", "dev")
	settingsH := handlers.NewSettingsHandler(s, authH.Secret())
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/quiz/next", quizH.Next)
	r.Get("/api/quiz/stats", quizH.Stats)
	r.Get("/api/settings", settingsH.Get)
	r.Patch("/api/settings", settingsH.Patch)

	// Align the per-user setting with the server cap so stats reflects 1.
	type settingsPatch struct {
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
	do(t, r, http.MethodPatch, "/api/settings", settingsPatch{
		PrimaryLang: "en", ProgNew: "transl_to_zh",
		ProgTierStruggling: "transl_to_zh", ProgTierLearning: "zh_pinyin_to_transl",
		ProgTierPracticing: "zh_to_transl", ProgTierMastered: "random",
		NewWordMode0: "transl_to_zh", NewWordMode1: "transl_to_zh", NewWordMode2: "zh_to_transl",
		MaxNewWordsPerDay: 1,
	})

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

func TestQuizNext_ProgressiveNewWord(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello", "hi"})
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/next?mode=progressive", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.Mode != models.ModeNewWord {
		t.Errorf("first progressive card should be new_word, got %s", card.Mode)
	}
	if card.Prompt != "你好" {
		t.Errorf("prompt should be zh text, got %q", card.Prompt)
	}
	if len(card.Translations["en"]) != 2 {
		t.Errorf("en_texts should have 2 entries, got %d", len(card.Translations["en"]))
	}
}

// TestQuizNext_ProgressiveNewWord_PinyinCoversFullText guards against a New
// Word card only showing pinyin for the headword when the zh text carries a
// bracketed annotation (e.g. "过（动词）") and the stored pinyin predates it —
// the card should regenerate a full-text pinyin instead of the stale partial one.
func TestQuizNext_ProgressiveNewWord_PinyinCoversFullText(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "过（动词）", "guò", []string{"pass"})
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/next?mode=progressive", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.Pinyin == nil {
		t.Fatal("want non-nil pinyin")
	}
	want := "guò dòng cí"
	if *card.Pinyin != want {
		t.Errorf("want full-text pinyin %q, got %q", want, *card.Pinyin)
	}
}

// TestQuizNext_ProgressiveNewWord_PinyinAlreadyComplete_NotOverwritten ensures
// the regeneration only kicks in when the stored pinyin is short — a
// hand-curated pinyin that already covers every character (e.g. picking one
// reading of a polyphonic character) must not be silently replaced.
func TestQuizNext_ProgressiveNewWord_PinyinAlreadyComplete_NotOverwritten(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "得（动词）", "dei3 dong4 ci2", []string{"must"})
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/next?mode=progressive", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.Pinyin == nil {
		t.Fatal("want non-nil pinyin")
	}
	want := "dei3 dong4 ci2"
	if *card.Pinyin != want {
		t.Errorf("stored pinyin already covers the text, want it kept as %q, got %q", want, *card.Pinyin)
	}
}

func TestQuizNext_ProgressiveAfterAcknowledge(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	// Acknowledge the word
	rec := do(t, r, "POST", "/api/quiz/acknowledge", map[string]int64{"word_id": id})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("acknowledge: want 204, got %d: %s", rec.Code, rec.Body)
	}

	// Next progressive card should be en_to_zh (total_attempts=1 < 3)
	rec = do(t, r, "GET", "/api/quiz/next?mode=progressive", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.Mode != models.ModeTranslToZh {
		t.Errorf("after acknowledge (0 correct): want en_to_zh, got %s", card.Mode)
	}
}

func TestQuizNext_ProgressiveThresholds(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)
	ctx := context.Background()

	// Acknowledge first
	do(t, r, "POST", "/api/quiz/acknowledge", map[string]int64{"word_id": id})

	// Helper to set progress for progressive-threshold testing.
	// Graduates the word out of LearningNewWord so SelectProgressiveMode is used.
	setProgress := func(correct, attempts int) {
		p, _ := s.GetSM2Progress(ctx, id)
		p.TotalCorrect = correct
		p.TotalAttempts = attempts
		p.LearningNewWord = false                    // graduated: use progressive tier logic
		p.DueDate = time.Now().UTC().Add(-time.Hour) // ensure due
		s.UpdateSM2Progress(ctx, *p)
	}

	tests := []struct {
		correct  int
		attempts int
		wantMode string
	}{
		{0, 1, models.ModeTranslToZh},        // attempts < 3 → en_to_zh
		{1, 10, models.ModeTranslToZh},       // accuracy 10% < 50% → en_to_zh
		{6, 10, models.ModeZhPinyinToTransl}, // accuracy 60% < 70% → zh_pinyin_to_en
		{3, 4, models.ModeZhPinyinToTransl},  // accuracy 75% but attempts < 10 → zh_pinyin_to_en
		{8, 10, models.ModeZhToTransl},       // accuracy 80%, attempts >= 10 → zh_to_en
	}
	for _, tt := range tests {
		setProgress(tt.correct, tt.attempts)
		rec := do(t, r, "GET", "/api/quiz/next?mode=progressive", nil)
		var card models.QuizCard
		decodeJSON(t, rec, &card)
		if card.Mode != tt.wantMode {
			t.Errorf("correct=%d attempts=%d: want mode %s, got %s", tt.correct, tt.attempts, tt.wantMode, card.Mode)
		}
	}

	// accuracy >= 85% and attempts >= 10: random (any valid mode)
	setProgress(9, 10) // also sets LearningNewWord=false
	validModes := map[string]bool{
		models.ModeTranslToZh:        true,
		models.ModeZhToTransl:        true,
		models.ModeZhPinyinToTransl:  true,
		models.ModeZhToTranslNoSound: true,
		models.ModeVoiceToTransl:     true,
	}
	for i := 0; i < 30; i++ {
		p, _ := s.GetSM2Progress(ctx, id)
		p.DueDate = time.Now().UTC().Add(-time.Hour)
		p.LearningNewWord = false
		s.UpdateSM2Progress(ctx, *p)
		rec := do(t, r, "GET", "/api/quiz/next?mode=progressive", nil)
		var card models.QuizCard
		decodeJSON(t, rec, &card)
		if !validModes[card.Mode] {
			t.Errorf("mastered (90%% 10 attempts): got invalid mode %s", card.Mode)
		}
	}
}

func TestQuizNext_ExcludeParam_SkipsRecentWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	r := newRouter(s)

	idA := seedWord(t, s, "一", "", []string{"one"})
	idB := seedWord(t, s, "二", "", []string{"two"})

	// Acknowledge both words so they are immediately due.
	if err := s.AcknowledgeWord(ctx, int64(2), idA); err != nil {
		t.Fatalf("AcknowledgeWord idA: %v", err)
	}
	if err := s.AcknowledgeWord(ctx, int64(2), idB); err != nil {
		t.Fatalf("AcknowledgeWord idB: %v", err)
	}

	// Excluding idA should return idB.
	rec := do(t, r, "GET", fmt.Sprintf("/api/quiz/next?exclude=%d", idA), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.WordID != idB {
		t.Errorf("exclude=%d: want word_id=%d, got word_id=%d", idA, idB, card.WordID)
	}
}

// TestQuizNext_SessionExtension_FlagsNonDueCardAndRespectsSetting reproduces
// issue #186: when the only due-today card is excluded (just answered), the
// server may serve a not-yet-due word instead of repeating it immediately.
// The response must flag this via session_extension so the frontend can keep
// its due-today count accurate, and the new extend_session_with_extra_words
// user setting must be able to turn the behaviour off entirely.
func TestQuizNext_SessionExtension_FlagsNonDueCardAndRespectsSetting(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	r := newRouter(s)

	idA := seedWord(t, s, "一", "", []string{"one"}) // due today, will be excluded
	idB := seedWord(t, s, "二", "", []string{"two"}) // due far in the future

	if err := s.AcknowledgeWord(ctx, int64(2), idA); err != nil {
		t.Fatalf("AcknowledgeWord idA: %v", err)
	}
	if err := s.AcknowledgeWord(ctx, int64(2), idB); err != nil {
		t.Fatalf("AcknowledgeWord idB: %v", err)
	}
	// Push idB's due date 30 days into the future so it is not itself due today.
	if err := s.SkipWord(ctx, int64(2), idB, 30); err != nil {
		t.Fatalf("push idB into the future: %v", err)
	}

	// Default setting (extend_session_with_extra_words=true): excluding idA
	// (the only due-today card) must fall back to idB and flag it as extended.
	rec := do(t, r, "GET", fmt.Sprintf("/api/quiz/next?exclude=%d", idA), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.WordID != idB {
		t.Errorf("want fallback to non-due word_id=%d, got word_id=%d", idB, card.WordID)
	}
	if !card.SessionExtension {
		t.Error("want session_extension=true when a non-due card was served to avoid repetition")
	}

	// Disable the setting: the server must no longer widen beyond today's bound,
	// so it repeats idA instead of serving idB.
	payload := baseSettingsPatch()
	payload["extend_session_with_extra_words"] = false
	if rec := do(t, r, http.MethodPatch, "/api/settings", payload); rec.Code != http.StatusOK {
		t.Fatalf("PATCH /api/settings: want 200, got %d: %s", rec.Code, rec.Body)
	}

	rec = do(t, r, "GET", fmt.Sprintf("/api/quiz/next?exclude=%d", idA), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	card = models.QuizCard{}
	decodeJSON(t, rec, &card)
	if card.WordID != idA {
		t.Errorf("with extension disabled, want repeated due word_id=%d, got word_id=%d", idA, card.WordID)
	}
	if card.SessionExtension {
		t.Error("want session_extension=false when extension is disabled")
	}
}

func TestQuizNext_MnemonicsFalse_SkipsHMMCard(t *testing.T) {
	s := openTestDB(t)
	seedHMMCard(t, s)
	// No vocabulary words — with mnemonics=true the HMM card would be served;
	// with mnemonics=false we should get 404 "no words available".
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/next?mnemonics=false", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["error"] != "no words available" {
		t.Errorf("unexpected error: %q", body["error"])
	}
}

func TestQuizNext_MnemonicsTrue_ServesHMMCard(t *testing.T) {
	s := openTestDB(t)
	seedHMMCard(t, s)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/next", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.CardType != "hmm" {
		t.Errorf("want card_type=hmm, got %q", card.CardType)
	}
}

func TestQuizNext_NewWordWithLangs_PopulatesDeTexts(t *testing.T) {
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

	rec := do(t, r, "GET", "/api/quiz/next?langs=en,de", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.Mode != models.ModeNewWord {
		t.Skipf("card is not new_word (mode=%s); test only applies to first introduction", card.Mode)
	}
	if len(card.Translations["en"]) == 0 {
		t.Error("EnTexts should be set on new_word card when langs includes en")
	}
	if len(card.Translations["de"]) == 0 {
		t.Error("DeTexts should be set on new_word card when langs includes de")
	}
}

func TestQuizNext_ReturnsComponentCard(t *testing.T) {
	s := openTestDB(t)
	if err := s.SeedHanziDecompositionForTest(context.Background(), "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Insert component directly — overdue, no regular words exist.
	past := time.Now().Add(-48 * time.Hour)
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", past)

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/quiz/next?trainComponents=1&mnemonics=false", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card map[string]any
	decodeJSON(t, rec, &card)
	if card["card_type"] != "component" {
		t.Errorf("want card_type=component, got %v", card["card_type"])
	}
	if card["prompt"] != "女" {
		t.Errorf("want prompt=女, got %v", card["prompt"])
	}
}

func TestQuizNext_NewComponentCard_HasIsNewAndDefinitions(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Insert unseen component (first_seen_date IS NULL).
	past := time.Now().Add(-48 * time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/quiz/next?trainComponents=1&mnemonics=false", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card map[string]any
	decodeJSON(t, rec, &card)
	if card["card_type"] != "component" {
		t.Fatalf("want card_type=component, got %v", card["card_type"])
	}
	if isNew, _ := card["is_new"].(bool); !isNew {
		t.Error("want is_new=true for unseen component")
	}
	defs, ok := card["definitions"].(map[string]any)
	if !ok {
		t.Fatalf("want definitions map, got %T", card["definitions"])
	}
	if defs["en"] != "woman" {
		t.Errorf("want definitions[en]=woman, got %v", defs["en"])
	}
}

func TestQuizNext_SeenComponentCard_IsNewFalse(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	past := time.Now().Add(-48 * time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)
	s.SetComponentSeenForTest(ctx, int64(2), "女")

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/quiz/next?trainComponents=1&mnemonics=false", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card map[string]any
	decodeJSON(t, rec, &card)
	if isNew, _ := card["is_new"].(bool); isNew {
		t.Error("want is_new=false for already-seen component")
	}
}

func TestQuizNext_ComponentCard_HasPinyin(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionWithPinyinForTest(ctx, "女", "woman", `["nǚ"]`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/quiz/next?trainComponents=1&mnemonics=false", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card map[string]any
	decodeJSON(t, rec, &card)
	if card["card_type"] != "component" {
		t.Fatalf("want card_type=component, got %v", card["card_type"])
	}
	if card["pinyin"] != "nǚ" {
		t.Errorf("want pinyin=%q, got %v", "nǚ", card["pinyin"])
	}
}

func TestQuizNext_RandomMode_RespectsBucketRestriction(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	r := newRouter(s)

	// Restrict the "new" bucket to zh_to_transl only.
	payload := baseSettingsPatch()
	payload["random_mode_range_transl_to_zh"] = "off"
	payload["random_mode_range_zh_to_transl"] = "new,85-100"
	payload["random_mode_range_zh_pinyin_to_transl"] = "off"
	payload["random_mode_range_zh_to_transl_no_sound"] = "off"
	payload["random_mode_range_voice_to_transl"] = "off"
	if rec := do(t, r, http.MethodPatch, "/api/settings", payload); rec.Code != http.StatusOK {
		t.Fatalf("PATCH /api/settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}
	// Word is now in the "new" bucket (learning_new_word=true).

	for i := 0; i < 20; i++ {
		rec := do(t, r, "GET", "/api/quiz/next?mode=random", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var card models.QuizCard
		decodeJSON(t, rec, &card)
		if card.Mode != models.ModeZhToTransl {
			t.Fatalf("random mode with restrictive config: want %s, got %s", models.ModeZhToTransl, card.Mode)
		}
	}
}

func TestQuizNext_CycleMode_RespectsBucketRestriction(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	r := newRouter(s)

	// Restrict the "new" bucket to zh_to_transl only, and configure a cycle
	// sequence that does not include zh_to_transl at all — forcing the
	// bucket-restricted fallback path.
	payload := baseSettingsPatch()
	payload["random_mode_range_transl_to_zh"] = "off"
	payload["random_mode_range_zh_to_transl"] = "new,85-100"
	payload["random_mode_range_zh_pinyin_to_transl"] = "off"
	payload["random_mode_range_zh_to_transl_no_sound"] = "off"
	payload["random_mode_range_voice_to_transl"] = "off"
	payload["cycle_sequence"] = "transl_to_zh,zh_pinyin_to_transl"
	if rec := do(t, r, http.MethodPatch, "/api/settings", payload); rec.Code != http.StatusOK {
		t.Fatalf("PATCH /api/settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	rec := do(t, r, "GET", "/api/quiz/next?mode=cycle", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.Mode != models.ModeZhToTransl {
		t.Errorf("cycle mode with no bucket-eligible configured step: want fallback %s, got %s", models.ModeZhToTransl, card.Mode)
	}
}

func TestQuizNext_UsesUserPrimaryLang(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	ctx := context.Background()

	// Set primary lang to "de"
	payload := map[string]string{
		"primary_lang": "de", "secondary_lang": "en",
		"prog_new": "transl_to_zh", "prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning": "zh_pinyin_to_transl", "prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered": "random",
		"new_word_mode_0":    "transl_to_zh", "new_word_mode_1": "transl_to_zh",
		"new_word_mode_2": "zh_to_transl",
	}
	do(t, r, http.MethodPatch, "/api/settings", payload)

	// Create a word with only "de" translation
	id, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "狗",
		Pinyin:       "gǒu",
		Translations: map[string][]string{"de": {"Hund"}},
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	// Acknowledge the word
	do(t, r, http.MethodPost, "/api/quiz/acknowledge", map[string]int64{"word_id": id})

	// Request quiz with mode=transl_to_zh (no langs param → should use primary_lang="de")
	rec := do(t, r, http.MethodGet, "/api/quiz/next?mode=transl_to_zh", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	// The prompt should be the German translation
	if card.Prompt != "Hund" {
		t.Errorf("want prompt=Hund (de), got %q", card.Prompt)
	}
}

func TestQuizNext_ComponentCard_IsAlsoWord_True(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed hanzi: %v", err)
	}
	past := time.Now().Add(-48 * time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)
	s.SetComponentSeenForTest(ctx, int64(2), "女")
	_, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:       "女",
		Translations: map[string][]string{"en": {"woman"}},
	})
	if err != nil {
		t.Fatalf("create word: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/quiz/next?trainComponents=1&mnemonics=false", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card map[string]any
	decodeJSON(t, rec, &card)
	if card["card_type"] != "component" {
		t.Fatalf("want card_type=component, got %v", card["card_type"])
	}
	if v, _ := card["is_also_word"].(bool); !v {
		t.Error("want is_also_word=true when component character is also a word")
	}
}

func TestQuizNext_ComponentCard_IsAlsoWord_False(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed hanzi: %v", err)
	}
	past := time.Now().Add(-48 * time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)
	s.SetComponentSeenForTest(ctx, int64(2), "女")
	// No corresponding zh word created.

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/quiz/next?trainComponents=1&mnemonics=false", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card map[string]any
	decodeJSON(t, rec, &card)
	if card["card_type"] != "component" {
		t.Fatalf("want card_type=component, got %v", card["card_type"])
	}
	if v, _ := card["is_also_word"].(bool); v {
		t.Error("want is_also_word=false when component character is not a word")
	}
}

func TestQuizNext_WordCard_IsAlsoComponent_True(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	s.InsertComponentProgressForTest(ctx, int64(2), "关", time.Now())
	seedWord(t, s, "关", "guān", []string{"close"})

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/quiz/next?mode=progressive", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card map[string]any
	decodeJSON(t, rec, &card)
	if v, _ := card["is_also_component"].(bool); !v {
		t.Error("want is_also_component=true when word text is also a component")
	}
}

func TestQuizNext_WordCard_IsAlsoComponent_False(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/quiz/next?mode=progressive", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card map[string]any
	decodeJSON(t, rec, &card)
	if v, _ := card["is_also_component"].(bool); v {
		t.Error("want is_also_component=false when word text is not a component")
	}
}

func TestQuizNext_CycleMode(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/next?mode=cycle", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	// total_attempts=1 after acknowledge → (1-1)%3=0 → zh_pinyin_to_transl
	if card.Mode != models.ModeZhPinyinToTransl {
		t.Errorf("cycle position 0: want %s, got %s", models.ModeZhPinyinToTransl, card.Mode)
	}
}

func TestQuizNext_VoiceToTransl_SameShapeAsZhToTransl(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	p.TotalAttempts = 1
	p.TotalCorrect = 1
	p.DueDate = time.Now().UTC().Add(-time.Hour)
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatalf("UpdateSM2Progress: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/next?mode="+models.ModeVoiceToTransl, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var card models.QuizCard
	decodeJSON(t, rec, &card)
	if card.Mode != models.ModeVoiceToTransl {
		t.Errorf("want card.Mode=%s, got %s", models.ModeVoiceToTransl, card.Mode)
	}
	if card.Prompt != "你好" {
		t.Errorf("want prompt=你好, got %q", card.Prompt)
	}
}
