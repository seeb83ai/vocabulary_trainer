package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

// TestMain sets migration credential env vars once for the entire test binary
// so all in-memory DBs get consistent user seeds regardless of the host environment.
func TestMain(m *testing.M) {
	os.Setenv("ADMIN_EMAIL", "admin@example.de")
	os.Setenv("ADMIN_PASSWORD", "I am the admin")
	os.Setenv("USER_EMAIL", "me@example.de")
	os.Setenv("USER_PASSWORD", "I learn zh")
	os.Exit(m.Run())
}

// ── helpers ───────────────────────────────────────────────────────────────────

func openTestDB(t *testing.T) *db.Store {
	t.Helper()
	s, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("openTestDB: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	clearHMMLibrary(t, s)
	return s
}

// clearHMMLibrary blanks all HMM library names so no entries qualify for the
// mnemonic quiz.  Migration v13 pre-seeds several named entries; this resets
// them so word-quiz tests are not disturbed by interleaved HMM cards.
func clearHMMLibrary(t *testing.T, s *db.Store) {
	t.Helper()
	ctx := context.Background()
	actors, err := s.GetHMMActors(ctx, int64(2))
	if err != nil {
		t.Fatalf("clearHMMLibrary GetHMMActors: %v", err)
	}
	for _, a := range actors {
		if a.ActorName != "" {
			if err := s.UpdateHMMActor(ctx, int64(2), a.Initial, ""); err != nil {
				t.Fatalf("clearHMMLibrary UpdateHMMActor %s: %v", a.Initial, err)
			}
		}
	}
	for tone := 1; tone <= 5; tone++ {
		if err := s.UpdateHMMToneRoom(ctx, int64(2), tone, ""); err != nil {
			t.Fatalf("clearHMMLibrary tone %d: %v", tone, err)
		}
	}
	props, err := s.GetHMMProps(ctx, int64(2))
	if err != nil {
		t.Fatalf("clearHMMLibrary GetHMMProps: %v", err)
	}
	for _, p := range props {
		if err := s.DeleteHMMProp(ctx, int64(2), p.Radical); err != nil {
			t.Fatalf("clearHMMLibrary DeleteHMMProp %s: %v", p.Radical, err)
		}
	}
}

func newRouter(s *db.Store) http.Handler {
	return newRouterWithUserID(s, 2)
}

func newRouterWithUserID(s *db.Store, userID int64) http.Handler {
	wordsH := &handlers.WordsHandler{Store: s}
	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 100}
	mismatchH := &handlers.MismatchesHandler{Store: s}
	importH := &handlers.ImportHandler{Store: s}
	tagsH := &handlers.TagsHandler{Store: s}
	authH, _ := handlers.NewAuthHandler(s, nil, "http://localhost:8080", "")
	settingsH := handlers.NewSettingsHandler(s, authH.Secret())
	translateH := &handlers.TranslateHandler{Store: s, APIKey: "test-key", TargetLang: "EN", SettingsHandler: settingsH}
	componentH := &handlers.ComponentHandler{Store: s}
	hmmH := &handlers.HMMHandler{Store: s}
	hmmQuizH := &handlers.HMMQuizHandler{Store: s}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(userID))
	r.Post("/api/login", authH.Login)
	r.Post("/api/register", authH.Register)
	r.Get("/api/verify-email", authH.VerifyEmail)
	r.Get("/api/me", authH.Me)
	r.Post("/api/change-password", authH.ChangePassword)
	r.Get("/api/quiz/next", quizH.Next)
	r.Post("/api/quiz/answer", quizH.Answer)
	r.Post("/api/quiz/skip", quizH.Skip)
	r.Post("/api/quiz/acknowledge", quizH.Acknowledge)
	r.Post("/api/quiz/acknowledge-random", quizH.AcknowledgeRandom)
	r.Post("/api/quiz/advance", quizH.Advance)
	r.Get("/api/quiz/stats", quizH.Stats)
	r.Get("/api/quiz/langs", quizH.Langs)
	r.Get("/api/quiz/daily-stats", quizH.DailyStats)
	r.Get("/api/quiz/word-stats", quizH.WordStats)
	r.Get("/api/quiz/due-date-distribution", quizH.DueDateDistribution)
	r.Get("/api/mismatches", mismatchH.List)
	r.Route("/api/words", func(r chi.Router) {
		r.Get("/", wordsH.List)
		r.Post("/", wordsH.Create)
		r.Get("/export", wordsH.Export)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", wordsH.GetByID)
			r.Put("/", wordsH.Update)
			r.Delete("/", wordsH.Delete)
			r.Post("/translations", wordsH.AddTranslation)
			r.Post("/review", wordsH.MarkReview)
		})
	})
	r.Get("/api/import/source-tags", importH.SourceTags)
	r.Get("/api/import/preview", importH.Preview)
	r.Post("/api/import", importH.Import)
	r.Get("/api/tags/details", tagsH.Details)
	r.Put("/api/tags/{name}", tagsH.Update)
	r.Get("/api/config", translateH.Config(true, true))
	r.Post("/api/translate", translateH.Translate)
	r.Get("/api/components", componentH.List)
	r.Post("/api/component/answer", componentH.Answer)
	r.Post("/api/component/seen", componentH.Seen)
	r.Post("/api/component/skip", componentH.Skip)
	r.Get("/api/component/stats", componentH.Stats)
	r.Post("/api/hmm-quiz/skip", hmmQuizH.Skip)
	r.Get("/api/hmm/breakdown", hmmH.GetBreakdown)
	r.Get("/api/settings", settingsH.Get)
	r.Patch("/api/settings", settingsH.Patch)
	r.Put("/api/settings/api-keys", settingsH.PutAPIKeys)
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
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       zhText,
		Pinyin:       pinyin,
		Translations: map[string][]string{"en": enTexts},
	})
	if err != nil {
		t.Fatalf("seedWord: %v", err)
	}
	return id
}

func seedWordFull(t *testing.T, s *db.Store, userID int64, zhText, pinyin string, enTexts, deTexts, tags []string) int64 {
	t.Helper()
	tr := map[string][]string{"en": enTexts}
	if len(deTexts) > 0 {
		tr["de"] = deTexts
	}
	id, err := s.CreateWord(context.Background(), userID, models.CreateWordRequest{
		ZhText:       zhText,
		Pinyin:       pinyin,
		Translations: tr,
		Tags:         tags,
	})
	if err != nil {
		t.Fatalf("seedWordFull: %v", err)
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
	_, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:  "你好",
		Pinyin:  "", // no pinyin
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

	for _, mode := range []string{models.ModeTranslToZh, models.ModeZhToTransl, models.ModeZhPinyinToTransl} {
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
	validModes := map[string]bool{models.ModeTranslToZh: true, models.ModeZhToTransl: true, models.ModeZhPinyinToTransl: true}
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

	// Acknowledge id1 so it counts as today's introduced word.
	if err := s.AcknowledgeWord(ctx, int64(2), id1); err != nil {
		t.Fatalf("AcknowledgeWord id1: %v", err)
	}

	// Build a router with maxNew=1 (cap is now reached).
	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 1}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
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
		ZhText:  "你好",
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
		Translations: map[string][]string{"en": {"to study"}},
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
		Translations: map[string][]string{"en": {"hello"}},
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
		ZhText:  "test",
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

// ── GET /api/words/export ─────────────────────────────────────────────────────

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
		"mode":    "zh_to_transl",
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
		"mode":    "zh_to_transl",
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
		"mode":    "zh_to_transl",
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
		"mode":    "transl_to_zh",
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
			"mode":    "zh_to_transl",
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

// ── Progressive mode ─────────────────────────────────────────────────────────

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
		p.LearningNewWord = false // graduated: use progressive tier logic
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
		models.ModeTranslToZh:       true,
		models.ModeZhToTransl:       true,
		models.ModeZhPinyinToTransl: true,
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

// ── POST /api/quiz/skip ──────────────────────────────────────────────────────

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

// ── POST /api/quiz/acknowledge ───────────────────────────────────────────────

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

	_, total, err := s.GetComponentCounts(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetComponentCounts: %v", err)
	}
	if total == 0 {
		t.Error("expected component_progress rows after acknowledge")
	}
}

// ── POST /api/words/{id}/review ───────────────────────────────────────────────

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
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
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

// ── GET /api/quiz/daily-stats ────────────────────────────────────────────────

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

// ── /api/quiz/stats new fields ────────────────────────────────────────────────

func TestStatsHandlerNewFields(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)

	for _, key := range []string{"today_attempts", "today_mistakes", "available_to_advance", "new_available", "hmm_due_today"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("stats response missing key %q", key)
		}
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

// ── mnemonics param ───────────────────────────────────────────────────────────

// seedHMMCard names an actor so EnsureHMMProgress creates a due progress row.
func seedHMMCard(t *testing.T, s *db.Store) {
	t.Helper()
	ctx := context.Background()
	actors, err := s.GetHMMActors(ctx, int64(2))
	if err != nil || len(actors) == 0 {
		t.Skip("no actor slots available for HMM seeding")
	}
	if err := s.UpdateHMMActor(ctx, int64(2), actors[0].Initial, "TestActor"); err != nil {
		t.Fatalf("seedHMMCard: %v", err)
	}
	if err := s.EnsureHMMProgress(ctx, int64(2)); err != nil {
		t.Fatalf("seedHMMCard EnsureHMMProgress: %v", err)
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

// ── /api/quiz/advance ─────────────────────────────────────────────────────────

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

// ── AcknowledgeRandom ─────────────────────────────────────────────────────────

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

	_, total, err := s.GetComponentCounts(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetComponentCounts: %v", err)
	}
	if total == 0 {
		t.Error("expected component_progress rows after acknowledge-random")
	}
}

// ── Stats new_available with learning words present ───────────────────────────

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

// ── StartTraining sets learning phase ─────────────────────────────────────────

func TestWordsCreate_StartTraining_SetsLearningPhase(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	ctx := context.Background()

	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:        "学",
		Translations: map[string][]string{"en": {"study"}},
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

// ── Input length validation ───────────────────────────────────────────────────

func TestWordsCreate_ZhTextTooLong(t *testing.T) {
	r := newRouter(openTestDB(t))
	long201 := ""
	for i := 0; i < 201; i++ {
		long201 += "好"
	}
	rec := do(t, r, "POST", "/api/words", models.CreateWordRequest{
		ZhText:  long201,
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
		ZhText:  "好",
		Translations: map[string][]string{"en": {"ok"}},
		Tags:    tags,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for > 20 tags, got %d", rec.Code)
	}
}

// ── GET /api/quiz/due-date-distribution ──────────────────────────────────────

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

// ── Pinyin Quiz Handlers ────────────────────────────────────────────────────

func newPinyinRouter(t *testing.T, s *db.Store) http.Handler {
	t.Helper()
	pinyinH := &handlers.PinyinQuizHandler{Store: s, PinyinAudioDirs: []string{t.TempDir()}}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/pinyin-quiz/next", pinyinH.Next)
	r.Post("/api/pinyin-quiz/answer", pinyinH.Answer)
	r.Get("/api/pinyin-quiz/stats", pinyinH.Stats)
	r.Get("/api/pinyin-quiz/tags", pinyinH.ListTags)
	return r
}

func seedPinyinSounds(t *testing.T, store *db.Store) {
	t.Helper()
	sounds := []models.PinyinSound{
		{Initial: "b", Final: "a", Tone: 1, Syllable: "ba", Filename: "ba1.mp3", Tag: "b_p_m_f"},
		{Initial: "b", Final: "a", Tone: 2, Syllable: "ba", Filename: "ba2.mp3", Tag: "b_p_m_f"},
		{Initial: "b", Final: "a", Tone: 3, Syllable: "ba", Filename: "ba3.mp3", Tag: "b_p_m_f"},
		{Initial: "b", Final: "a", Tone: 4, Syllable: "ba", Filename: "ba4.mp3", Tag: "b_p_m_f"},
		{Initial: "p", Final: "a", Tone: 1, Syllable: "pa", Filename: "pa1.mp3", Tag: "b_p_m_f"},
	}
	for _, snd := range sounds {
		if _, err := store.InsertPinyinSound(context.Background(), 2, snd); err != nil {
			t.Fatalf("seedPinyinSounds: %v", err)
		}
	}
}

func TestPinyinQuizNext_EmptyDB(t *testing.T) {
	s := openTestDB(t)
	r := newPinyinRouter(t, s)
	rec := do(t, r, "GET", "/api/pinyin-quiz/next", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestPinyinQuizNext_ReturnsCard(t *testing.T) {
	s := openTestDB(t)
	seedPinyinSounds(t, s)
	r := newPinyinRouter(t, s)

	rec := do(t, r, "GET", "/api/pinyin-quiz/next", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var card models.PinyinCard
	decodeJSON(t, rec, &card)

	if card.SoundID == 0 {
		t.Error("expected non-zero sound_id")
	}
	if card.Mode != models.PinyinModeMultipleChoice {
		t.Errorf("expected multiple_choice mode for new sound, got %q", card.Mode)
	}
	if len(card.Options) < 2 {
		t.Errorf("expected at least 2 options, got %d", len(card.Options))
	}
	if card.AudioFile == "" {
		t.Error("expected non-empty audio_file")
	}
}

func TestPinyinQuizAnswer_Correct(t *testing.T) {
	s := openTestDB(t)
	seedPinyinSounds(t, s)
	r := newPinyinRouter(t, s)

	// Get a card first
	rec := do(t, r, "GET", "/api/pinyin-quiz/next", nil)
	var card models.PinyinCard
	decodeJSON(t, rec, &card)

	// Submit correct answer (the card's own sound_id)
	rec = do(t, r, "POST", "/api/pinyin-quiz/answer", models.PinyinAnswerRequest{
		SoundID: card.SoundID,
		Answer:  fmt.Sprintf("%d", card.SoundID),
		Mode:    models.PinyinModeMultipleChoice,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.PinyinAnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("expected correct=true")
	}
	if resp.CorrectAnswer == "" {
		t.Error("expected non-empty correct_answer")
	}
}

func TestPinyinQuizAnswer_Wrong(t *testing.T) {
	s := openTestDB(t)
	seedPinyinSounds(t, s)
	r := newPinyinRouter(t, s)

	rec := do(t, r, "GET", "/api/pinyin-quiz/next", nil)
	var card models.PinyinCard
	decodeJSON(t, rec, &card)

	// Find a wrong option
	var wrongID int64
	for _, opt := range card.Options {
		if opt.SoundID != card.SoundID {
			wrongID = opt.SoundID
			break
		}
	}
	if wrongID == 0 {
		t.Fatal("no wrong option found")
	}

	rec = do(t, r, "POST", "/api/pinyin-quiz/answer", models.PinyinAnswerRequest{
		SoundID: card.SoundID,
		Answer:  fmt.Sprintf("%d", wrongID),
		Mode:    models.PinyinModeMultipleChoice,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.PinyinAnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Correct {
		t.Error("expected correct=false")
	}
	if resp.ConfusedWith == nil {
		t.Error("expected confusion detail for wrong MC answer")
	}
}

func TestPinyinQuizStats(t *testing.T) {
	s := openTestDB(t)
	seedPinyinSounds(t, s)
	r := newPinyinRouter(t, s)

	rec := do(t, r, "GET", "/api/pinyin-quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	var stats map[string]int
	decodeJSON(t, rec, &stats)
	if stats["total"] != 5 {
		t.Errorf("expected total=5, got %d", stats["total"])
	}
}

func TestPinyinQuizTags(t *testing.T) {
	s := openTestDB(t)
	seedPinyinSounds(t, s)
	r := newPinyinRouter(t, s)

	rec := do(t, r, "GET", "/api/pinyin-quiz/tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	var tags []string
	decodeJSON(t, rec, &tags)
	if len(tags) != 1 || tags[0] != "b_p_m_f" {
		t.Errorf("expected [b_p_m_f], got %v", tags)
	}
}

// ── GET /api/quiz/langs ───────────────────────────────────────────────────────

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
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
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

// ── POST /api/quiz/answer — multi-lang ───────────────────────────────────────

func TestQuizAnswer_MultiLang_DEAccepted(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
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
		ZhText:  "再见",
		Pinyin:  "zàijiàn",
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
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
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

// ── GET /api/quiz/next — new_word with langs ──────────────────────────────────

func TestQuizNext_NewWordWithLangs_PopulatesDeTexts(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	_, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
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

// ── PUT /api/words/{id} — unchanged zh_text ───────────────────────────────────

func TestWordsUpdate_SameZhText_NoUniqueError(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	// Re-save with the exact same zh_text — should not return 500.
	rec := do(t, r, "PUT", fmt.Sprintf("/api/words/%d", id), models.UpdateWordRequest{
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
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

// ── GET /api/words?missing_lang= ─────────────────────────────────────────────

func TestWordsList_MissingLangDE(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Word with EN only.
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	// Word with both EN and DE.
	_, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "再见",
		Pinyin:  "zàijiàn",
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

// ── Auth: Register ─────────────────────────────────────────────────────────────

func TestRegister_OK(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/register", map[string]string{
		"email": "new@example.com", "password": "securepass1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	decodeJSON(t, rec, &body)
	if body["auto_login"] != true {
		t.Errorf("expected auto_login=true (nil email sender), got %v", body["auto_login"])
	}
	if rec.Result().Cookies() == nil {
		t.Error("expected session cookie to be set")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	r := newRouter(openTestDB(t))
	payload := map[string]string{"email": "new@example.com", "password": "securepass1"}
	do(t, r, "POST", "/api/register", payload)
	rec := do(t, r, "POST", "/api/register", payload)
	if rec.Code != http.StatusConflict {
		t.Errorf("want 409, got %d: %s", rec.Code, rec.Body)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/register", map[string]string{
		"email": "a@b.com", "password": "short",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/register", map[string]string{
		"email": "notanemail", "password": "securepass1",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

// ── Auth: VerifyEmail ──────────────────────────────────────────────────────────

func TestVerifyEmail_BadToken(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/verify-email?token=badtoken", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/?error=invalid_token" {
		t.Errorf("want redirect to /?error=invalid_token, got %q", loc)
	}
}

func TestVerifyEmail_MissingToken(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/verify-email", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d", rec.Code)
	}
}

func TestVerifyEmail_OK(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// Create unverified user with a known token
	token := "testtoken1234567890abcdef12345678"
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err := s.CreateUser(context.Background(), "verify@example.com", "$2a$10$placeholder", token, expiresAt)
	if err != nil {
		t.Fatal(err)
	}

	rec := do(t, r, "GET", "/api/verify-email?token="+token, nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/train" {
		t.Errorf("want redirect to /train, got %q", loc)
	}
}

// ── Auth: Login ────────────────────────────────────────────────────────────────

func TestLogin_UnverifiedEmail(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// Register creates an unverified user if emailSender != nil, but here
	// we nil emailSender so Register auto-verifies. Create directly instead.
	token := "unverifiedtoken1234567890123456"
	expiresAt := time.Now().Add(24 * time.Hour)
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	_, err := s.CreateUser(context.Background(), "unverified@example.com", string(hash), token, expiresAt)
	if err != nil {
		t.Fatal(err)
	}

	rec := do(t, r, "POST", "/api/login", map[string]string{
		"email": "unverified@example.com", "password": "password123",
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", rec.Code, rec.Body)
	}
	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["error"] != "email_not_verified" {
		t.Errorf("expected email_not_verified error, got %q", body["error"])
	}
}

// ── Auth: Me ───────────────────────────────────────────────────────────────────

func TestMe_OK(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	decodeJSON(t, rec, &body)
	if body["email"] == "" || body["email"] == nil {
		t.Error("expected non-empty email in response")
	}
}

// ── Auth: ChangePassword ───────────────────────────────────────────────────────

func TestChangePassword_OK(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// user ID 2 is "me@example.de" / "I learn zh" from TestMain env
	rec := do(t, r, "POST", "/api/change-password", map[string]string{
		"current_password": "I learn zh",
		"new_password":     "newpassword123",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
}

func TestChangePassword_WrongCurrent(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/change-password", map[string]string{
		"current_password": "wrongpassword",
		"new_password":     "newpassword123",
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", rec.Code, rec.Body)
	}
}

func TestChangePassword_ShortNew(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/change-password", map[string]string{
		"current_password": "I learn zh",
		"new_password":     "short",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

// ── GET /api/import/source-tags ───────────────────────────────────────────────

func TestImportSourceTags_ReturnsTags(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"HSK1"})
	seedWordFull(t, s, 1, "谢谢", "xiè xie", []string{"thank you"}, nil, []string{"HSK1"})
	// User 2 has a different tag — should not appear
	seedWordFull(t, s, 2, "再见", "zài jiàn", []string{"goodbye"}, nil, []string{"HSK2"})

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/import/source-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 1 || tags[0].Name != "HSK1" {
		t.Errorf("want [{Name:HSK1 ...}], got %v", tags)
	}
	if !tags[0].Importable {
		t.Errorf("expected importable=true by default")
	}
	hasEn := false
	for _, l := range tags[0].AvailableLangs {
		if l == "en" {
			hasEn = true
		}
	}
	if !hasEn {
		t.Errorf("expected available_langs to include 'en' for tag with EN translations")
	}
	for _, l := range tags[0].AvailableLangs {
		if l == "de" {
			t.Errorf("expected 'de' not in available_langs when no DE translations")
		}
	}
}

func TestImportSourceTags_WithDeFlag(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, []string{"hallo"}, []string{"greetings"})
	seedWordFull(t, s, 1, "再见", "zài jiàn", []string{"goodbye"}, nil, []string{"greetings"})

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/import/source-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 1 {
		t.Fatalf("want 1 tag, got %d", len(tags))
	}
	hasEn, hasDe := false, false
	for _, l := range tags[0].AvailableLangs {
		if l == "en" {
			hasEn = true
		}
		if l == "de" {
			hasDe = true
		}
	}
	if !hasEn {
		t.Errorf("expected available_langs to include 'en'")
	}
	if !hasDe {
		t.Errorf("expected available_langs to include 'de' when at least one word has DE")
	}
}

func TestImportSourceTags_EmptyWhenNoWords(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/import/source-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 0 {
		t.Errorf("want empty, got %v", tags)
	}
}

func TestImportSourceTags_HidesNonImportable(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"public"})
	seedWordFull(t, s, 1, "秘密", "", []string{"secret"}, nil, []string{"private"})
	// Mark private tag as not importable.
	if err := s.UpsertTagMeta(context.Background(), int64(1), "private", "", false); err != nil {
		t.Fatalf("UpsertTagMeta: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/import/source-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 1 || tags[0].Name != "public" {
		t.Errorf("want only [public], got %v", tags)
	}
}

// ── GET /api/import/preview ───────────────────────────────────────────────────

func TestImportPreview_ValidTag(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, []string{"hallo"}, []string{"HSK1"})
	seedWordFull(t, s, 1, "谢谢", "xiè xie", []string{"thank you"}, nil, []string{"HSK1"})
	seedWordFull(t, s, 1, "再见", "zài jiàn", []string{"goodbye"}, nil, []string{"HSK1"})

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/import/preview?tag=HSK1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Tag            string         `json:"tag"`
		Total          int            `json:"total"`
		AvailableLangs map[string]int `json:"available_langs"`
		Examples       []struct {
			ZhText       string              `json:"zh_text"`
			Pinyin       string              `json:"pinyin"`
			Translations map[string][]string `json:"translations"`
		} `json:"examples"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Tag != "HSK1" {
		t.Errorf("want tag HSK1, got %q", resp.Tag)
	}
	if resp.Total != 3 {
		t.Errorf("want total 3, got %d", resp.Total)
	}
	if resp.AvailableLangs["en"] != 3 {
		t.Errorf("want available_langs[en]=3, got %d", resp.AvailableLangs["en"])
	}
	if resp.AvailableLangs["de"] != 1 {
		t.Errorf("want available_langs[de]=1, got %d", resp.AvailableLangs["de"])
	}
	if len(resp.Examples) != 3 {
		t.Errorf("want 3 examples, got %d", len(resp.Examples))
	}
	if len(resp.Examples) > 50 {
		t.Errorf("want at most 50 examples, got %d", len(resp.Examples))
	}
	if resp.Examples[0].ZhText == "" {
		t.Error("expected non-empty zh_text in first example")
	}
	if len(resp.Examples[0].Translations["en"]) == 0 {
		t.Error("expected en translations in first example")
	}
}

func TestImportPreview_UnknownTag(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/import/preview?tag=nonexistent", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Total int `json:"total"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Total != 0 {
		t.Errorf("want total 0, got %d", resp.Total)
	}
}

func TestImportPreview_MissingTag(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/import/preview", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

// ── POST /api/import ──────────────────────────────────────────────────────────

func TestImport_Basic(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"HSK1"})
	seedWordFull(t, s, 1, "谢谢", "xiè xie", []string{"thank you"}, nil, []string{"HSK1"})
	seedWordFull(t, s, 1, "再见", "zài jiàn", []string{"goodbye"}, nil, []string{"HSK1"})

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{"HSK1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Imported int `json:"imported"`
		Skipped  int `json:"skipped"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Imported != 3 {
		t.Errorf("want imported=3, got %d", resp.Imported)
	}
	if resp.Skipped != 0 {
		t.Errorf("want skipped=0, got %d", resp.Skipped)
	}

	// Verify words now exist for user 2
	listRec := do(t, r, "GET", "/api/words/?tags=HSK1", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d: %s", listRec.Code, listRec.Body)
	}
	var listResp struct {
		Total int `json:"total"`
	}
	decodeJSON(t, listRec, &listResp)
	if listResp.Total != 3 {
		t.Errorf("want 3 words in user list, got %d", listResp.Total)
	}
}

func TestImport_SkipsDuplicates(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"HSK1"})
	seedWordFull(t, s, 1, "再见", "zài jiàn", []string{"goodbye"}, nil, []string{"HSK1"})
	// User 2 already has 你好
	seedWordFull(t, s, 2, "你好", "nǐ hǎo", []string{"hello"}, nil, nil)

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{"HSK1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Imported int `json:"imported"`
		Skipped  int `json:"skipped"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Imported != 1 {
		t.Errorf("want imported=1, got %d", resp.Imported)
	}
	if resp.Skipped != 1 {
		t.Errorf("want skipped=1, got %d", resp.Skipped)
	}
}

func TestImport_DeFlag(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, []string{"Hallo"}, []string{"HSK1"})

	r := newRouter(s)
	// Import with DE
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en", "de"},
		"apply_tags":   []string{"HSK1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct{ Imported int `json:"imported"` }
	decodeJSON(t, rec, &resp)
	if resp.Imported != 1 {
		t.Fatalf("want imported=1, got %d", resp.Imported)
	}

	// Fetch the word and verify DE translation is present
	listRec := do(t, r, "GET", "/api/words/?tags=HSK1", nil)
	var listResp struct {
		Words []struct {
			Translations map[string][]string `json:"translations"`
		} `json:"words"`
	}
	decodeJSON(t, listRec, &listResp)
	if len(listResp.Words) == 0 {
		t.Fatal("no words returned")
	}
	if len(listResp.Words[0].Translations["de"]) == 0 {
		t.Error("expected DE translations to be imported")
	}
}

func TestImport_DeFlagFalse(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, []string{"Hallo"}, []string{"HSK1"})

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	listRec := do(t, r, "GET", "/api/words/", nil)
	var listResp struct {
		Words []struct {
			Translations map[string][]string `json:"translations"`
		} `json:"words"`
	}
	decodeJSON(t, listRec, &listResp)
	if len(listResp.Words) == 0 {
		t.Fatal("no words returned")
	}
	if len(listResp.Words[0].Translations["de"]) != 0 {
		t.Errorf("expected no DE translations, got %v", listResp.Words[0].Translations["de"])
	}
}

func TestImport_ApplyCustomTags(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"HSK1"})

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{"HSK1", "my-review"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	// Verify both tags are on the imported word
	listRec := do(t, r, "GET", "/api/words/?tags=my-review", nil)
	var listResp struct{ Total int `json:"total"` }
	decodeJSON(t, listRec, &listResp)
	if listResp.Total != 1 {
		t.Errorf("want 1 word tagged my-review, got %d", listResp.Total)
	}
}

func TestImport_MissingTag(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"import_langs": []string{"en"},
		"apply_tags":   []string{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

// ── GET /api/tags/details ─────────────────────────────────────────────────────

func TestTagDetails_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/tags/details", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 0 {
		t.Errorf("want empty, got %v", tags)
	}
}

func TestTagDetails_ReturnsTags(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 2, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"greetings"})

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/tags/details", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 1 || tags[0].Name != "greetings" {
		t.Fatalf("want [{greetings ...}], got %v", tags)
	}
	if tags[0].Description != "" {
		t.Errorf("expected empty description, got %q", tags[0].Description)
	}
	if !tags[0].Importable {
		t.Errorf("expected importable=true by default")
	}
}

func TestTagDetails_DoesNotReturnOtherUserTags(t *testing.T) {
	s := openTestDB(t)
	// User 1 has a tag; user 2 (current user in tests) has none.
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"library"})

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/tags/details", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 0 {
		t.Errorf("want 0 tags for user 2, got %v", tags)
	}
}

// ── PUT /api/tags/{name} ──────────────────────────────────────────────────────

func TestTagUpdate_SetsDescriptionAndImportable(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 2, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"hsk1"})

	r := newRouter(s)
	rec := do(t, r, "PUT", "/api/tags/hsk1", map[string]any{
		"description": "HSK level 1 words",
		"importable":  false,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	// Verify via GET /api/tags/details.
	rec2 := do(t, r, "GET", "/api/tags/details", nil)
	var tags []models.TagDetail
	decodeJSON(t, rec2, &tags)
	if len(tags) != 1 {
		t.Fatalf("want 1 tag, got %d", len(tags))
	}
	if tags[0].Description != "HSK level 1 words" {
		t.Errorf("expected description 'HSK level 1 words', got %q", tags[0].Description)
	}
	if tags[0].Importable {
		t.Errorf("expected importable=false after update")
	}
}

func TestTagUpdate_InvalidBody(t *testing.T) {
	r := newRouter(openTestDB(t))
	req := httptest.NewRequest("PUT", "/api/tags/hsk1", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

// ── Role gating ───────────────────────────────────────────────────────────────

// TestConfig_PlusUserSeesAvailable verifies that user 2 (plus) gets deepl_available=true.
func TestConfig_PlusUserSeesAvailable(t *testing.T) {
	s := openTestDB(t)
	r := newRouterWithUserID(s, 2)
	rec := do(t, r, http.MethodGet, "/api/config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var cfg map[string]bool
	decodeJSON(t, rec, &cfg)
	if !cfg["deepl_available"] {
		t.Error("plus user: deepl_available should be true")
	}
	if !cfg["llm_available"] {
		t.Error("plus user: llm_available should be true")
	}
}

// TestConfig_FreeUserSeesConfiguredButNotAvailable verifies free users see configured=true, available=false.
func TestConfig_FreeUserSeesConfiguredButNotAvailable(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	freeID, err := s.CreateUser(ctx, "free@example.com", "hash", "tok-free", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	r := newRouterWithUserID(s, freeID)
	rec := do(t, r, http.MethodGet, "/api/config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var cfg map[string]bool
	decodeJSON(t, rec, &cfg)
	if !cfg["deepl_configured"] {
		t.Error("free user: deepl_configured should be true (key is set)")
	}
	if cfg["deepl_available"] {
		t.Error("free user: deepl_available should be false")
	}
	if !cfg["llm_configured"] {
		t.Error("free user: llm_configured should be true")
	}
	if cfg["llm_available"] {
		t.Error("free user: llm_available should be false")
	}
}

// TestTranslate_FreeUserForbidden verifies free users cannot call the translate endpoint.
func TestTranslate_FreeUserForbidden(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	freeID, err := s.CreateUser(ctx, "free2@example.com", "hash", "tok-free2", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	r := newRouterWithUserID(s, freeID)
	rec := do(t, r, http.MethodPost, "/api/translate", map[string]string{"zh_text": "你好"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("free user translate: want 403, got %d", rec.Code)
	}
}

// TestTranslate_PinyinOnlyAllowedForFreeUser verifies that the pinyin-only path
// (both zh_text and en_text provided) is not blocked for free users.
func TestTranslate_PinyinOnlyAllowedForFreeUser(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	freeID, err := s.CreateUser(ctx, "free3@example.com", "hash", "tok-free3", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	r := newRouterWithUserID(s, freeID)
	rec := do(t, r, http.MethodPost, "/api/translate", map[string]string{
		"zh_text":     "你好",
		"source_text": "hello",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("free user pinyin-only: want 200, got %d", rec.Code)
	}
	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if resp["pinyin"] == "" {
		t.Error("expected non-empty pinyin in response")
	}
}

// TestTranslate_PlusUserAllowed verifies plus users can call translate.
func TestTranslate_PlusUserAllowed(t *testing.T) {
	s := openTestDB(t)
	// user 2 is plus; the actual DeepL call will fail (no real key),
	// so we only check that we don't get 403.
	r := newRouterWithUserID(s, 2)
	rec := do(t, r, http.MethodPost, "/api/translate", map[string]string{"zh_text": "你好"})
	if rec.Code == http.StatusForbidden {
		t.Fatal("plus user should not be forbidden from translate")
	}
}

// ── Component handler tests ───────────────────────────────────────────────────

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

// ── POST /api/component/skip ─────────────────────────────────────────────────

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

	items, _, err := s.GetComponentList(ctx, int64(2), "", 1, 10)
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

// ── POST /api/hmm-quiz/skip ──────────────────────────────────────────────────

func TestHMMQuizSkip_DaysOne(t *testing.T) {
	s := openTestDB(t)
	seedHMMCard(t, s)
	ctx := context.Background()

	prog, err := s.GetHMMProgress(ctx, int64(2), models.HMMEntityActor, "n")
	if err != nil || prog == nil {
		// fall back: pick any actor with a progress row
		actors, _ := s.GetHMMActors(ctx, int64(2))
		var key string
		for _, a := range actors {
			if a.ActorName != "" {
				key = a.Initial
				break
			}
		}
		if key == "" {
			t.Skip("no named actor available")
		}
		prog, _ = s.GetHMMProgress(ctx, int64(2), models.HMMEntityActor, key)
	}
	if prog == nil {
		t.Skip("no hmm progress row available")
	}

	r := newRouter(s)
	rec := do(t, r, http.MethodPost, "/api/hmm-quiz/skip", map[string]any{
		"entity_type": models.HMMEntityActor,
		"entity_key":  prog.EntityKey,
		"days":        1,
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body)
	}

	after, err := s.GetHMMProgress(ctx, int64(2), models.HMMEntityActor, prog.EntityKey)
	if err != nil || after == nil {
		t.Fatalf("GetHMMProgress after skip: %v", err)
	}
	if after.TotalAttempts != prog.TotalAttempts {
		t.Error("skip should not change total_attempts")
	}
	delta := after.DueDate.Sub(time.Now())
	if delta < 23*time.Hour || delta > 25*time.Hour {
		t.Errorf("days=1 should move due_date ~24h ahead, got delta=%v", delta)
	}
}

func TestHMMQuizSkip_NotFound(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodPost, "/api/hmm-quiz/skip", map[string]any{
		"entity_type": models.HMMEntityActor,
		"entity_key":  "zzz",
		"days":        1,
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestHMMQuizSkip_BadEntityType(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodPost, "/api/hmm-quiz/skip", map[string]any{
		"entity_type": "garbage",
		"entity_key":  "x",
		"days":        1,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
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

func TestHMMBreakdown_Empty(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodGet, "/api/hmm/breakdown", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	items, _ := resp["breakdown"].([]any)
	if len(items) != 0 {
		t.Errorf("want empty breakdown on fresh DB, got %d items", len(items))
	}
}

// ── GET /api/settings ────────────────────────────────────────────────────────

func TestGetSettings_Defaults(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var st models.UserSettings
	decodeJSON(t, rec, &st)

	if st.PrimaryLang != "en" {
		t.Errorf("want primary_lang=en, got %q", st.PrimaryLang)
	}
	if st.SecondaryLang != "de" {
		t.Errorf("want secondary_lang=de, got %q", st.SecondaryLang)
	}
	if st.ProgNew != "transl_to_zh" {
		t.Errorf("want prog_new=transl_to_zh, got %q", st.ProgNew)
	}
	if st.ProgTierLearning != "zh_pinyin_to_transl" {
		t.Errorf("want prog_tier_learning=zh_pinyin_to_transl, got %q", st.ProgTierLearning)
	}
	if st.ProgTierMastered != "random" {
		t.Errorf("want prog_tier_mastered=random, got %q", st.ProgTierMastered)
	}
	if st.NewWordMode2 != "zh_to_transl" {
		t.Errorf("want new_word_mode_2=zh_to_transl, got %q", st.NewWordMode2)
	}
	if st.DeeplKeySet {
		t.Error("want deepl_key_set=false by default")
	}
}

// ── PATCH /api/settings ──────────────────────────────────────────────────────

func TestPatchSettings_Valid(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":         "de",
		"secondary_lang":       "en",
		"prog_new":             "zh_to_transl",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "zh_pinyin_to_transl",
		"new_word_mode_2":      "zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify by reading back
	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.PrimaryLang != "de" {
		t.Errorf("want primary_lang=de after patch, got %q", st.PrimaryLang)
	}
	if st.SecondaryLang != "en" {
		t.Errorf("want secondary_lang=en after patch, got %q", st.SecondaryLang)
	}
	if st.ProgNew != "zh_to_transl" {
		t.Errorf("want prog_new=zh_to_transl after patch, got %q", st.ProgNew)
	}
	if st.NewWordMode1 != "zh_pinyin_to_transl" {
		t.Errorf("want new_word_mode_1=zh_pinyin_to_transl, got %q", st.NewWordMode1)
	}
}

func TestPatchSettings_InvalidMode(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":         "en",
		"secondary_lang":       "de",
		"prog_new":             "invalid_mode",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "transl_to_zh",
		"new_word_mode_2":      "zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid mode, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSettings_SameLang(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":         "en",
		"secondary_lang":       "en", // same as primary — invalid
		"prog_new":             "transl_to_zh",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "transl_to_zh",
		"new_word_mode_2":      "zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 when primary=secondary lang, got %d", rec.Code)
	}
}

func TestPatchSettings_EmptySecondaryLang(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":         "en",
		"secondary_lang":       "", // no secondary — valid
		"prog_new":             "transl_to_zh",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "transl_to_zh",
		"new_word_mode_2":      "zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for empty secondary_lang, got %d: %s", rec.Code, rec.Body.String())
	}

	// Read back and verify secondary_lang is stored as empty
	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.SecondaryLang != "" {
		t.Errorf("want secondary_lang empty after patch, got %q", st.SecondaryLang)
	}
}

// ── GET /api/quiz/next uses user primary lang ─────────────────────────────────

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
		"new_word_mode_0": "transl_to_zh", "new_word_mode_1": "transl_to_zh",
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
