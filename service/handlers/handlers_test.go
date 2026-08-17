package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	os.Setenv("BCRYPT_COST", "min") // speed up bcrypt in tests
	os.Exit(m.Run())
}

// ── helpers ───────────────────────────────────────────────────────────────────

var (
	templateOnce sync.Once
	templatePath string
	templateErr  error
)

// buildTemplateDB runs all migrations once into a scratch file so individual
// tests can clone it instead of re-running migrations for every test.
func buildTemplateDB(tb testing.TB) string {
	tb.Helper()
	templateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "vocab-test-template-*")
		if err != nil {
			templateErr = err
			return
		}
		path := filepath.Join(dir, "template.db")
		if err := db.OpenMigratedTemplate(path); err != nil {
			templateErr = err
			return
		}
		templatePath = path
	})
	if templateErr != nil {
		tb.Fatalf("build template db: %v", templateErr)
	}
	return templatePath
}

// openTestDB creates a SQLite store for tests by cloning the pre-migrated
// template database rather than running all migrations from scratch.
func openTestDB(t *testing.T) *db.Store {
	t.Helper()
	tmpl := buildTemplateDB(t)
	data, err := os.ReadFile(tmpl)
	if err != nil {
		t.Fatalf("read template db: %v", err)
	}
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write test db copy: %v", err)
	}
	s, err := db.Open(path)
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

// GitHub issue handler config used by newRouter. Tests point
// testGitHubAPIBaseURL at an httptest mock server before building the router.
var (
	testGitHubToken      = "test-token"
	testGitHubRepo       = "owner/repo"
	testGitHubAPIBaseURL = ""
)

func newRouter(s *db.Store) http.Handler {
	return newRouterWithUserID(s, 2)
}

func newRouterWithUserID(s *db.Store, userID int64) http.Handler {
	wordsH := &handlers.WordsHandler{Store: s}
	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 100}
	mismatchH := &handlers.MismatchesHandler{Store: s}
	importH := &handlers.ImportHandler{Store: s}
	tagsH := &handlers.TagsHandler{Store: s}
	authH, _ := handlers.NewAuthHandlerWithEnv(s, nil, "http://localhost:8080", "", "dev")
	settingsH := handlers.NewSettingsHandler(s, authH.Secret())
	translateH := &handlers.TranslateHandler{Store: s, APIKey: "test-key", TargetLang: "EN", SettingsHandler: settingsH}
	componentH := &handlers.ComponentHandler{Store: s}
	hmmH := &handlers.HMMHandler{Store: s}
	llmH := &handlers.LLMHandler{Store: s}
	hmmQuizH := &handlers.HMMQuizHandler{Store: s}
	hanziH := &handlers.HanziHandler{Store: s}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(userID))
	r.Post("/api/login", authH.Login)
	r.Post("/api/register", authH.Register)
	r.Get("/api/verify-email", authH.VerifyEmail)
	r.Get("/api/me", authH.Me)
	r.Post("/api/change-password", authH.ChangePassword)
	r.Get("/api/quiz/next", quizH.Next)
	r.Post("/api/quiz/answer", quizH.Answer)
	r.Post("/api/quiz/accept-correct", quizH.AcceptCorrect)
	r.Post("/api/quiz/skip", quizH.Skip)
	r.Post("/api/quiz/acknowledge", quizH.Acknowledge)
	r.Post("/api/quiz/acknowledge-random", quizH.AcknowledgeRandom)
	r.Post("/api/quiz/advance", quizH.Advance)
	r.Get("/api/quiz/match-game", quizH.MatchGame)
	r.Post("/api/quiz/match-answer", quizH.MatchAnswer)
	r.Post("/api/quiz/difficult", quizH.FlagDifficult)
	r.Post("/api/quiz/difficult/clear", quizH.ClearDifficult)
	r.Get("/api/quiz/stats", quizH.Stats)
	r.Get("/api/quiz/langs", quizH.Langs)
	r.Get("/api/quiz/daily-stats", quizH.DailyStats)
	r.Get("/api/quiz/word-stats", quizH.WordStats)
	r.Get("/api/quiz/due-date-distribution", quizH.DueDateDistribution)
	r.Post("/api/quiz/record-time", quizH.RecordTime)
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
			r.Post("/reset", wordsH.ResetProgress)
		})
	})
	uploadCSVH := &handlers.UploadCSVHandler{Store: s}
	r.Post("/api/words/upload-csv", uploadCSVH.UploadCSV)
	r.Get("/api/import/source-tags", importH.SourceTags)
	r.Get("/api/import/preview", importH.Preview)
	r.Post("/api/import", importH.Import)
	r.Get("/api/tags/details", tagsH.Details)
	r.Put("/api/tags/{name}", tagsH.Update)
	r.Get("/api/config", translateH.Config(true, true))
	r.Post("/api/translate", translateH.Translate)
	ghH := &handlers.GitHubHandler{Store: s, Token: testGitHubToken, Repo: testGitHubRepo, APIBaseURL: testGitHubAPIBaseURL, Labels: []string{"from-app"}}
	r.Post("/api/github/issues", ghH.Create)
	r.Get("/api/github/config", ghH.ConfigFlag)
	r.Get("/api/components", componentH.List)
	r.Post("/api/component/answer", componentH.Answer)
	r.Post("/api/component/accept-correct", componentH.AcceptCorrect)
	r.Post("/api/component/seen", componentH.Seen)
	r.Post("/api/component/skip", componentH.Skip)
	r.Get("/api/component/stats", componentH.Stats)
	r.Get("/api/component/due-date-distribution", componentH.DueDateDistribution)
	r.Post("/api/components/{char}/review", componentH.Review)
	r.Put("/api/components/{char}/translation", componentH.UpdateTranslation)
	r.Get("/api/components/{char}/translations", componentH.GetTranslations)
	r.Get("/api/components/{char}/hmm-scene", componentH.GetHMMScene)
	r.Put("/api/components/{char}/hmm-scene", componentH.PutHMMScene)
	r.Delete("/api/components/{char}/hmm-scene", componentH.DeleteHMMScene)
	r.Get("/api/components/{char}/hmm/context", componentH.GetComponentHMMContext)
	r.Put("/api/components/{char}/hmm", componentH.SaveCompScene)
	r.Post("/api/components/{char}/hmm/generate-scene", llmH.GenerateCompScene)
	r.Get("/api/hanzi/decompose", hanziH.Decompose)
	r.Post("/api/hmm-quiz/skip", hmmQuizH.Skip)
	r.Get("/api/hmm/breakdown", hmmH.GetBreakdown)
	r.Get("/api/settings", settingsH.Get)
	r.Patch("/api/settings", settingsH.Patch)
	r.Put("/api/settings/api-keys", settingsH.PutAPIKeys)
	r.Patch("/api/training-filters", settingsH.PatchTrainingFilters)
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

// ── TranslToZh wrong answer: user_answer_pinyin ───────────────────────────────

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

// ── GET /api/quiz/next?exclude=... ───────────────────────────────────────────

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

	_, total, err := s.GetComponentCounts(ctx, int64(2), nil)
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

	_, total, err := s.GetComponentCounts(ctx, int64(2), nil)
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

// ── Input length validation ───────────────────────────────────────────────────

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

// ── POST /api/quiz/answer — multi-lang ───────────────────────────────────────

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

// ── GET /api/quiz/next — new_word with langs ──────────────────────────────────

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

// ── PUT /api/words/{id} — unchanged zh_text ───────────────────────────────────

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

// ── GET /api/words?missing_lang= ─────────────────────────────────────────────

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
	// A duplicate registration must NOT return a distinct status that
	// reveals the email is already in use. See
	// TestRegister_ExistingEmail_DoesNotLeak in auth_test.go.
	r := newRouter(openTestDB(t))
	payload := map[string]string{"email": "new@example.com", "password": "securepass1"}
	do(t, r, "POST", "/api/register", payload)
	rec := do(t, r, "POST", "/api/register", payload)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 (indistinguishable from a fresh registration), got %d: %s", rec.Code, rec.Body)
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

// ── Audio cache headers ──────────────────────────────────────────────────────

// TestAudio_NotImmutable verifies that the audio handler does NOT mark
// responses as immutable. Marking them so means a regenerated MP3
// (e.g. after a zh_text edit) is served stale for up to a year. The
// existing handler set this and was flagged in the security review.
func TestAudio_NotImmutable(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	tmpDir := t.TempDir()
	audioH := &handlers.AudioHandler{Store: s, AudioDir: tmpDir}
	if err := os.WriteFile(tmpDir+"/"+fmt.Sprint(id)+".mp3", []byte("fake-mp3"), 0644); err != nil {
		t.Fatalf("seed mp3: %v", err)
	}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/{id}", audioH.ServeAudio)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/audio/%d", id), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	cc := rec.Header().Get("Cache-Control")
	if cc == "" {
		t.Fatal("Cache-Control header should be set")
	}
	if strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control must not include 'immutable' (causes stale audio after regeneration): %q", cc)
	}
	if !strings.Contains(cc, "must-revalidate") && !strings.Contains(cc, "no-cache") {
		t.Errorf("Cache-Control should require revalidation (must-revalidate or no-cache): %q", cc)
	}
}

// TestServeAudio_OtherUserForbidden verifies that the cached-audio path enforces
// word ownership: a user must not be able to fetch a cached <id>.mp3 for a word
// they do not own (IDOR). The ownership check must run BEFORE serving the file.
func TestServeAudio_OtherUserForbidden(t *testing.T) {
	s := openTestDB(t)
	// Word belongs to user 2 (seedWord default owner).
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	tmpDir := t.TempDir()
	audioH := &handlers.AudioHandler{Store: s, AudioDir: tmpDir}
	// Pre-seed a cached MP3 so the lazy-generation branch is skipped.
	if err := os.WriteFile(tmpDir+"/"+fmt.Sprint(id)+".mp3", []byte("fake-mp3"), 0644); err != nil {
		t.Fatalf("seed mp3: %v", err)
	}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(999)) // a different user
	r.Get("/api/audio/{id}", audioH.ServeAudio)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/audio/%d", id), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for non-owner, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "fake-mp3") {
		t.Errorf("must not serve cached audio to non-owner")
	}
}

// TestGenerateAsync_LogsErrorOnFailure asserts the fire-and-forget TTS helper
// logs a failure with word-ID context instead of swallowing the error.
func TestGenerateAsync_LogsErrorOnFailure(t *testing.T) {
	// Force generate() to fail deterministically: point AudioDir at a path whose
	// parent is a regular file, so os.MkdirAll cannot create the directory.
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	audioH := &handlers.AudioHandler{AudioDir: filepath.Join(blocker, "audio")}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	audioH.GenerateAsync(4242, "你好")

	if !strings.Contains(buf.String(), "async tts generate word 4242") {
		t.Fatalf("expected async tts error log with word id, got: %q", buf.String())
	}
}

// TestServeAudio_TTSFailureLogged verifies that a TTS synthesis failure is
// logged with word-ID context (and surfaced as 503) instead of being swallowed.
func TestServeAudio_TTSFailureLogged(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	tmpDir := t.TempDir()
	audioH := &handlers.AudioHandler{
		Store:    s,
		AudioDir: tmpDir,
		Synth:    func(string) ([]byte, error) { return nil, fmt.Errorf("tts boom") },
	}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/{id}", audioH.ServeAudio)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/audio/%d", id), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
	logged := buf.String()
	if !strings.Contains(logged, fmt.Sprint(id)) || !strings.Contains(logged, "tts boom") {
		t.Errorf("expected TTS failure logged with word-ID context, got: %q", logged)
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
	var resp struct {
		Imported int `json:"imported"`
	}
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
	var listResp struct {
		Total int `json:"total"`
	}
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

// ── GET /api/component/due-date-distribution ─────────────────────────────────

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

// baseSettingsPatch returns a minimal valid PATCH /api/settings payload.
func baseSettingsPatch() map[string]any {
	return map[string]any{
		"primary_lang":         "en",
		"prog_new":             "zh_to_transl",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "zh_pinyin_to_transl",
		"new_word_mode_2":      "zh_to_transl",
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
	if !st.NewWordRequireZh {
		t.Error("want new_word_require_zh=true by default")
	}
	if !st.NewWordRequireTrans {
		t.Error("want new_word_require_trans=true by default")
	}
	if !st.ExtendSessionWithExtraWords {
		t.Error("want extend_session_with_extra_words=true by default")
	}
}

// PutAPIKeys must reject a local LLM URL that points at an internal address
// (SSRF guard) before doing anything else.
func TestPutAPIKeys_RejectsInternalLLMURL(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	body := map[string]any{
		"llm_provider":  "local",
		"llm_local_url": "http://169.254.169.254/latest/meta-data/",
		"llm_key":       "x",
	}
	rec := do(t, r, http.MethodPut, "/api/settings/api-keys", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for internal llm_local_url, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "llm_local_url") {
		t.Errorf("want llm_local_url error, got %s", rec.Body.String())
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

func TestPatchSettings_ExtendSessionWithExtraWords_RoundTrip(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := baseSettingsPatch()
	payload["extend_session_with_extra_words"] = false
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.ExtendSessionWithExtraWords {
		t.Error("want extend_session_with_extra_words=false after patch")
	}
}

// ── PATCH /api/training-filters ──────────────────────────────────────────────

func TestPatchTrainingFilters_Valid(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]any{
		"mode":       "cycle",
		"bucket":     "50-69",
		"langs":      []string{"de", "en"},
		"mnemonics":  false,
		"components": true,
		"tags":       []string{"HSK1"},
	}
	rec := do(t, r, http.MethodPatch, "/api/training-filters", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify via GET /api/settings
	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.TrainMode != "cycle" {
		t.Errorf("want train_mode=cycle, got %q", st.TrainMode)
	}
	if st.TrainBucket != "50-69" {
		t.Errorf("want train_bucket=50-69, got %q", st.TrainBucket)
	}
	if len(st.TrainLangs) != 2 || st.TrainLangs[0] != "de" {
		t.Errorf("want train_langs=[de en], got %v", st.TrainLangs)
	}
	if st.TrainMnemonics {
		t.Error("want train_mnemonics=false")
	}
	if !st.TrainComponents {
		t.Error("want train_components=true")
	}
	if len(st.TrainTags) != 1 || st.TrainTags[0] != "HSK1" {
		t.Errorf("want train_tags=[HSK1], got %v", st.TrainTags)
	}
}

func TestPatchTrainingFilters_ZhToTranslNoSoundAccepted(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]any{
		"mode":  "zh_to_transl_no_sound",
		"langs": []string{"en"},
	}
	rec := do(t, r, http.MethodPatch, "/api/training-filters", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.TrainMode != "zh_to_transl_no_sound" {
		t.Errorf("want train_mode=zh_to_transl_no_sound, got %q", st.TrainMode)
	}
}

func TestPatchTrainingFilters_InvalidMode(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]any{
		"mode":  "invalid_mode",
		"langs": []string{"en"},
	}
	rec := do(t, r, http.MethodPatch, "/api/training-filters", payload)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid mode, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSettings_NewWordRequire(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]interface{}{
		"primary_lang":           "en",
		"secondary_lang":         "de",
		"prog_new":               "transl_to_zh",
		"prog_tier_struggling":   "transl_to_zh",
		"prog_tier_learning":     "zh_pinyin_to_transl",
		"prog_tier_practicing":   "zh_to_transl",
		"prog_tier_mastered":     "random",
		"new_word_mode_0":        "transl_to_zh",
		"new_word_mode_1":        "transl_to_zh",
		"new_word_mode_2":        "zh_to_transl",
		"new_word_require_zh":    false,
		"new_word_require_trans": true,
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.NewWordRequireZh {
		t.Error("want new_word_require_zh=false after patch")
	}
	if !st.NewWordRequireTrans {
		t.Error("want new_word_require_trans=true after patch")
	}
}

func TestPatchSettings_ZhToTranslNoSoundAccepted(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]interface{}{
		"primary_lang":         "en",
		"secondary_lang":       "de",
		"prog_new":             "transl_to_zh",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_to_transl_no_sound",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "transl_to_zh",
		"new_word_mode_2":      "zh_to_transl_no_sound",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.ProgTierLearning != "zh_to_transl_no_sound" {
		t.Errorf("want prog_tier_learning=zh_to_transl_no_sound, got %q", st.ProgTierLearning)
	}
	if st.NewWordMode2 != "zh_to_transl_no_sound" {
		t.Errorf("want new_word_mode_2=zh_to_transl_no_sound, got %q", st.NewWordMode2)
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

// ── Hanzi decompose handler ───────────────────────────────────────────────────

func TestDecompose_EmptyCharsReturnsEmptyArray(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, http.MethodGet, "/api/hanzi/decompose", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result []models.HanziDecomposition
	decodeJSON(t, rec, &result)
	if len(result) != 0 {
		t.Errorf("want empty array, got %d entries", len(result))
	}
}

func TestDecompose_MarkNew_FlagsUntrainedComponents(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	s.SeedHanziDecompositionWithDecompForTest(ctx, "好", "good", "⿰女子")
	s.SeedHanziDecompositionForTest(ctx, "女", "woman")
	s.SeedHanziDecompositionForTest(ctx, "子", "child")

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/hanzi/decompose?chars=%E5%A5%BD&mark_new=true", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result []models.HanziDecomposition
	decodeJSON(t, rec, &result)
	if len(result) == 0 || len(result[0].Components) == 0 {
		t.Skip("no components returned")
	}
	for _, comp := range result[0].Components {
		if comp.IsNewComponent == nil || !*comp.IsNewComponent {
			t.Errorf("component %q: want is_new_component=true (no progress row), got %v", comp.Character, comp.IsNewComponent)
		}
	}
}

func TestDecompose_MarkNew_TrainedComponentNotFlagged(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	s.SeedHanziDecompositionWithDecompForTest(ctx, "好", "good", "⿰女子")
	s.SeedHanziDecompositionForTest(ctx, "女", "woman")
	s.SeedHanziDecompositionForTest(ctx, "子", "child")
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now())
	// Mark as trained.
	s.SetComponentSeenForTest(ctx, int64(2), "女")
	s.SetComponentAttemptsForTest(ctx, int64(2), "女", 1)

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/hanzi/decompose?chars=%E5%A5%BD&mark_new=true", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result []models.HanziDecomposition
	decodeJSON(t, rec, &result)
	if len(result) == 0 || len(result[0].Components) == 0 {
		t.Skip("no components returned")
	}
	byChar := map[string]*bool{}
	for _, comp := range result[0].Components {
		byChar[comp.Character] = comp.IsNewComponent
	}
	if v := byChar["女"]; v == nil || *v {
		t.Errorf("component 女: want is_new_component=false (trained), got %v", v)
	}
	if v := byChar["子"]; v == nil || !*v {
		t.Errorf("component 子: want is_new_component=true (untrained), got %v", v)
	}
}

func TestDecompose_Langs_PopulatesDefinitions(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	s.SeedHanziDecompositionWithDecompForTest(ctx, "好", "good", "⿰女子")
	s.SeedHanziDecompositionForTest(ctx, "女", "woman")
	s.SeedHanziDecompositionForTest(ctx, "子", "child")
	if err := s.SeedHanziTranslationForTest(ctx, "女", "de", "Frau"); err != nil {
		t.Fatalf("seed translation: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, http.MethodGet, "/api/hanzi/decompose?chars=%E5%A5%BD&langs=en,de", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result []models.HanziDecomposition
	decodeJSON(t, rec, &result)
	if len(result) == 0 || len(result[0].Components) == 0 {
		t.Skip("no components returned")
	}
	byChar := map[string]map[string]string{}
	for _, comp := range result[0].Components {
		byChar[comp.Character] = comp.Definitions
	}
	if defs := byChar["女"]; defs["en"] != "woman" {
		t.Errorf("女 EN: want %q, got %q", "woman", defs["en"])
	}
	if defs := byChar["女"]; defs["de"] != "Frau" {
		t.Errorf("女 DE: want %q, got %q", "Frau", defs["de"])
	}
}

// ── Component review ──────────────────────────────────────────────────────────

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

// ── Component translation update ──────────────────────────────────────────────

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

// ── Component list review filter ──────────────────────────────────────────────

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

// ── GetTranslations ───────────────────────────────────────────────────────────

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

// ── component is_also_word field ─────────────────────────────────────────────

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

// ── Component HMM scene handler tests ────────────────────────────────────────

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

// ── Component HMM context handler tests ──────────────────────────────────────

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

// ── POST /api/words/upload-csv ────────────────────────────────────────────────

func doMultipart(t *testing.T, r http.Handler, path string, fields map[string]string, fileContent string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write field %s: %v", k, err)
		}
	}
	if fileContent != "" {
		fw, err := w.CreateFormFile("file", "words.csv")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fw.Write([]byte(fileContent)); err != nil {
			t.Fatalf("write file content: %v", err)
		}
	}
	w.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestUploadCSV_NoFile(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv", map[string]string{"tags": "test"}, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadCSV_MissingTags(t *testing.T) {
	csv := "chinese,pinyin,en\n我,wǒ,I"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv", map[string]string{}, csv)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadCSV_BadCSVHeader(t *testing.T) {
	csv := "word,pinyin,en\n我,wǒ,I"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv", map[string]string{"tags": "test"}, csv)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadCSV_ValidBasic(t *testing.T) {
	csv := "chinese,pinyin,en\n我要回家了,wǒ yào huí jiā le,I go home"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 1 {
		t.Errorf("want imported=1, got %d", resp["imported"])
	}
	if resp["updated"] != 0 {
		t.Errorf("want updated=0, got %d", resp["updated"])
	}
	if resp["total"] != 1 {
		t.Errorf("want total=1, got %d", resp["total"])
	}
}

func TestUploadCSV_DuplicateCallsUpdate(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "我要回家了", "wǒ yào huí jiā le", []string{"old translation"})
	csv := "chinese,pinyin,en\n我要回家了,wǒ yào huí jiā le,I go home"
	r := newRouter(s)
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 0 {
		t.Errorf("want imported=0, got %d", resp["imported"])
	}
	if resp["updated"] != 1 {
		t.Errorf("want updated=1, got %d", resp["updated"])
	}
}

func TestUploadCSV_MultipleLanguages(t *testing.T) {
	csv := "chinese,pinyin,en,de\n你好,nǐ hǎo,hello,Hallo"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 1 {
		t.Errorf("want imported=1, got %d", resp["imported"])
	}
}

func TestUploadCSV_NoPinyinColumn(t *testing.T) {
	csv := "chinese,en\n我要回家了,I go home\n你好,hello"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for CSV without pinyin column, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 2 {
		t.Errorf("want imported=2, got %d", resp["imported"])
	}
}

func TestUploadCSV_NoPinyinMultipleLangs(t *testing.T) {
	csv := "chinese,en,de\n你好,hello,Hallo"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for CSV without pinyin column with multiple langs, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 1 {
		t.Errorf("want imported=1, got %d", resp["imported"])
	}
}

func TestUploadCSV_AutoGeneratesPinyinWhenColumnAbsent(t *testing.T) {
	csv := "chinese,en\n你好,hello"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var upload map[string]int
	decodeJSON(t, rec, &upload)
	if upload["imported"] != 1 {
		t.Fatalf("want imported=1, got %d", upload["imported"])
	}

	rec2 := do(t, r, "GET", "/api/words?page=1&per_page=20", nil)
	var resp models.WordListResponse
	decodeJSON(t, rec2, &resp)
	if len(resp.Words) != 1 {
		t.Fatalf("want 1 word in list, got %d", len(resp.Words))
	}
	wd := resp.Words[0]
	if wd.Pinyin == nil || *wd.Pinyin == "" {
		t.Errorf("expected pinyin to be auto-generated for %q, got nil/empty", wd.ZhText)
	}
}

func TestUploadCSV_MultipleSemicolonTranslations(t *testing.T) {
	csv := "chinese,pinyin,de\n我要回家了,wǒ yào huí jiā le,Ich gehe nach Hause; Ich gehe heim"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["imported"] != 1 {
		t.Errorf("want imported=1, got %d", resp["imported"])
	}
}

func TestUploadCSV_StartTraining(t *testing.T) {
	csv := "chinese,pinyin,en\n一,yī,one\n二,èr,two\n三,sān,three"
	r := newRouter(openTestDB(t))
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "test", "start_training_count": "2"}, csv)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	decodeJSON(t, rec, &resp)
	if resp["total"] != 3 {
		t.Errorf("want total=3, got %d", resp["total"])
	}
}

// ── Cycle mode ────────────────────────────────────────────────────────────────

func TestSettingsCycleSequence(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// Default cycle_sequence should be the canonical 3-step sequence.
	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET settings: want 200, got %d", rec.Code)
	}
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	want := "zh_pinyin_to_transl,transl_to_zh,zh_to_transl"
	if st.CycleSequence != want {
		t.Errorf("default cycle_sequence: want %q, got %q", want, st.CycleSequence)
	}

	// PATCH with a custom sequence.
	payload := map[string]string{
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
	rec = do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Read back and verify.
	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st2 models.UserSettings
	decodeJSON(t, rec, &st2)
	if st2.CycleSequence != "transl_to_zh,zh_to_transl" {
		t.Errorf("after PATCH cycle_sequence: want %q, got %q", "transl_to_zh,zh_to_transl", st2.CycleSequence)
	}
}

func TestSettingsCycleSequence_InvalidMode(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":   "en",
		"secondary_lang": "",
		"prog_new":       "transl_to_zh", "prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning": "zh_pinyin_to_transl", "prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered": "random",
		"new_word_mode_0":    "transl_to_zh", "new_word_mode_1": "transl_to_zh", "new_word_mode_2": "zh_to_transl",
		"cycle_sequence": "transl_to_zh,invalid_mode",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid cycle mode, got %d", rec.Code)
	}
}

func TestSettingsCycleSequence_TooFewSteps(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":   "en",
		"secondary_lang": "",
		"prog_new":       "transl_to_zh", "prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning": "zh_pinyin_to_transl", "prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered": "random",
		"new_word_mode_0":    "transl_to_zh", "new_word_mode_1": "transl_to_zh", "new_word_mode_2": "zh_to_transl",
		"cycle_sequence": "transl_to_zh",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for cycle_sequence with only 1 step, got %d", rec.Code)
	}
}

func TestSettingsCycleSequence_TooManySteps(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":   "en",
		"secondary_lang": "",
		"prog_new":       "transl_to_zh", "prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning": "zh_pinyin_to_transl", "prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered": "random",
		"new_word_mode_0":    "transl_to_zh", "new_word_mode_1": "transl_to_zh", "new_word_mode_2": "zh_to_transl",
		"cycle_sequence": "transl_to_zh,zh_to_transl,zh_pinyin_to_transl,mask_pinyin,transl_to_zh,zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for cycle_sequence with 6 steps, got %d", rec.Code)
	}
}

func TestSettingsCycleSequence_FiveSteps(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	fiveStep := "transl_to_zh,zh_to_transl,zh_pinyin_to_transl,mask_pinyin,transl_to_zh"
	payload := map[string]string{
		"primary_lang":   "en",
		"secondary_lang": "",
		"prog_new":       "transl_to_zh", "prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning": "zh_pinyin_to_transl", "prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered": "random",
		"new_word_mode_0":    "transl_to_zh", "new_word_mode_1": "transl_to_zh", "new_word_mode_2": "zh_to_transl",
		"cycle_sequence": fiveStep,
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for 5-step cycle_sequence, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.CycleSequence != fiveStep {
		t.Errorf("after PATCH 5-step cycle_sequence: want %q, got %q", fiveStep, st.CycleSequence)
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

	// Set total_attempts=4 directly so position=(4-1)%3=0 → back to step 0.
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
	// total_attempts=4 → (4-1)%3=0 → zh_pinyin_to_transl (wrapped back to step 0)
	if card.Mode != models.ModeZhPinyinToTransl {
		t.Errorf("cycle wrapped: want %s, got %s", models.ModeZhPinyinToTransl, card.Mode)
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

// ── Cycle advance on success only ────────────────────────────────────────────

func TestSettingsCycleAdvanceOnSuccessOnly_DefaultFalse(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET settings: want 200, got %d", rec.Code)
	}
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.CycleAdvanceOnSuccessOnly {
		t.Error("default cycle_advance_on_success_only: want false, got true")
	}
}

func TestSettingsCycleAdvanceOnSuccessOnly_RoundTrip(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]interface{}{
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
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if !st.CycleAdvanceOnSuccessOnly {
		t.Error("after PATCH: want cycle_advance_on_success_only=true, got false")
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

// ── Answer prev_state persistence ────────────────────────────────────────────

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

// ── Ambiguity detection ──────────────────────────────────────────────────────

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

// ── POST /api/quiz/accept-correct ────────────────────────────────────────────

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

// ── POST /api/component/accept-correct ───────────────────────────────────────

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

// ── Settings: accept_correct_mode ────────────────────────────────────────────

func validSettingsPayload() map[string]string {
	return map[string]string{
		"primary_lang":         "en",
		"secondary_lang":       "de",
		"prog_new":             "zh_to_transl",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "zh_pinyin_to_transl",
		"new_word_mode_2":      "zh_to_transl",
	}
}

func TestSettingsPatchAcceptCorrectMode(t *testing.T) {
	r := newRouter(openTestDB(t))

	payload := validSettingsPayload()
	payload["accept_correct_mode"] = "always"
	rec := do(t, r, "PATCH", "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body)
	}

	rec2 := do(t, r, "GET", "/api/settings", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET settings: want 200, got %d", rec2.Code)
	}
	var st models.UserSettings
	decodeJSON(t, rec2, &st)
	if st.AcceptCorrectMode != "always" {
		t.Errorf("AcceptCorrectMode: want %q, got %q", "always", st.AcceptCorrectMode)
	}
}

func TestSettingsPatchAcceptCorrectModeInvalid(t *testing.T) {
	r := newRouter(openTestDB(t))
	payload := validSettingsPayload()
	payload["accept_correct_mode"] = "banana"
	rec := do(t, r, "PATCH", "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid accept_correct_mode, got %d", rec.Code)
	}
}

// ── Settings: daily learning ──────────────────────────────────────────────────

func TestGetSettings_DailyLearningDefaults(t *testing.T) {
	r := newRouter(openTestDB(t))

	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var st models.UserSettings
	decodeJSON(t, rec, &st)

	if st.MaxNewWordsPerDay < 1 {
		t.Errorf("want MaxNewWordsPerDay >= 1, got %d", st.MaxNewWordsPerDay)
	}
	if !st.SkipNewWordsVisible {
		t.Error("want SkipNewWordsVisible=true by default")
	}
	if st.BaselineDueTodayEnabled {
		t.Error("want BaselineDueTodayEnabled=false by default")
	}
	if st.BaselineDueTodayValue <= 0 {
		t.Errorf("want BaselineDueTodayValue > 0, got %d", st.BaselineDueTodayValue)
	}
	if st.BaselineStrugglingEnabled {
		t.Error("want BaselineStrugglingEnabled=false by default")
	}
	if st.BaselineStrugglingValue <= 0 {
		t.Errorf("want BaselineStrugglingValue > 0, got %d", st.BaselineStrugglingValue)
	}
	if st.BaselineLearningEnabled {
		t.Error("want BaselineLearningEnabled=false by default")
	}
	if st.BaselineLearningValue <= 0 {
		t.Errorf("want BaselineLearningValue > 0, got %d", st.BaselineLearningValue)
	}
	if st.BaselineNewBucketEnabled {
		t.Error("want BaselineNewBucketEnabled=false by default")
	}
	if st.BaselineNewBucketValue <= 0 {
		t.Errorf("want BaselineNewBucketValue > 0, got %d", st.BaselineNewBucketValue)
	}
}

func TestPatchSettings_DailyLearning(t *testing.T) {
	r := newRouter(openTestDB(t))

	payload := validSettingsPayload()
	// Overlay daily learning fields using a combined map
	type dailyPayload struct {
		PrimaryLang               string `json:"primary_lang"`
		SecondaryLang             string `json:"secondary_lang"`
		ProgNew                   string `json:"prog_new"`
		ProgTierStruggling        string `json:"prog_tier_struggling"`
		ProgTierLearning          string `json:"prog_tier_learning"`
		ProgTierPracticing        string `json:"prog_tier_practicing"`
		ProgTierMastered          string `json:"prog_tier_mastered"`
		NewWordMode0              string `json:"new_word_mode_0"`
		NewWordMode1              string `json:"new_word_mode_1"`
		NewWordMode2              string `json:"new_word_mode_2"`
		MaxNewWordsPerDay         int    `json:"max_new_words_per_day"`
		SkipNewWordsVisible       bool   `json:"skip_new_words_visible"`
		BaselineDueTodayEnabled   bool   `json:"baseline_due_today_enabled"`
		BaselineDueTodayValue     int    `json:"baseline_due_today_value"`
		BaselineStrugglingEnabled bool   `json:"baseline_struggling_enabled"`
		BaselineStrugglingValue   int    `json:"baseline_struggling_value"`
		BaselineLearningEnabled   bool   `json:"baseline_learning_enabled"`
		BaselineLearningValue     int    `json:"baseline_learning_value"`
		BaselineNewBucketEnabled  bool   `json:"baseline_new_bucket_enabled"`
		BaselineNewBucketValue    int    `json:"baseline_new_bucket_value"`
	}
	req := dailyPayload{
		PrimaryLang:               payload["primary_lang"],
		SecondaryLang:             payload["secondary_lang"],
		ProgNew:                   payload["prog_new"],
		ProgTierStruggling:        payload["prog_tier_struggling"],
		ProgTierLearning:          payload["prog_tier_learning"],
		ProgTierPracticing:        payload["prog_tier_practicing"],
		ProgTierMastered:          payload["prog_tier_mastered"],
		NewWordMode0:              payload["new_word_mode_0"],
		NewWordMode1:              payload["new_word_mode_1"],
		NewWordMode2:              payload["new_word_mode_2"],
		MaxNewWordsPerDay:         3,
		SkipNewWordsVisible:       false,
		BaselineDueTodayEnabled:   true,
		BaselineDueTodayValue:     15,
		BaselineStrugglingEnabled: true,
		BaselineStrugglingValue:   8,
		BaselineLearningEnabled:   false,
		BaselineLearningValue:     20,
		BaselineNewBucketEnabled:  true,
		BaselineNewBucketValue:    3,
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)

	if st.MaxNewWordsPerDay != 3 {
		t.Errorf("want MaxNewWordsPerDay=3, got %d", st.MaxNewWordsPerDay)
	}
	if st.SkipNewWordsVisible {
		t.Error("want SkipNewWordsVisible=false after patch")
	}
	if !st.BaselineDueTodayEnabled {
		t.Error("want BaselineDueTodayEnabled=true after patch")
	}
	if st.BaselineDueTodayValue != 15 {
		t.Errorf("want BaselineDueTodayValue=15, got %d", st.BaselineDueTodayValue)
	}
	if !st.BaselineStrugglingEnabled {
		t.Error("want BaselineStrugglingEnabled=true after patch")
	}
	if st.BaselineStrugglingValue != 8 {
		t.Errorf("want BaselineStrugglingValue=8, got %d", st.BaselineStrugglingValue)
	}
	if st.BaselineLearningEnabled {
		t.Error("want BaselineLearningEnabled=false after patch")
	}
	if !st.BaselineNewBucketEnabled {
		t.Error("want BaselineNewBucketEnabled=true after patch")
	}
	if st.BaselineNewBucketValue != 3 {
		t.Errorf("want BaselineNewBucketValue=3, got %d", st.BaselineNewBucketValue)
	}
}

func TestPatchSettings_BaselineNewBucketValue_Invalid(t *testing.T) {
	r := newRouter(openTestDB(t))

	payload := validSettingsPayload()
	type dailyPayload struct {
		PrimaryLang            string `json:"primary_lang"`
		SecondaryLang          string `json:"secondary_lang"`
		ProgNew                string `json:"prog_new"`
		ProgTierStruggling     string `json:"prog_tier_struggling"`
		ProgTierLearning       string `json:"prog_tier_learning"`
		ProgTierPracticing     string `json:"prog_tier_practicing"`
		ProgTierMastered       string `json:"prog_tier_mastered"`
		NewWordMode0           string `json:"new_word_mode_0"`
		NewWordMode1           string `json:"new_word_mode_1"`
		NewWordMode2           string `json:"new_word_mode_2"`
		BaselineNewBucketValue int    `json:"baseline_new_bucket_value"`
	}
	req := dailyPayload{
		PrimaryLang:            payload["primary_lang"],
		SecondaryLang:          payload["secondary_lang"],
		ProgNew:                payload["prog_new"],
		ProgTierStruggling:     payload["prog_tier_struggling"],
		ProgTierLearning:       payload["prog_tier_learning"],
		ProgTierPracticing:     payload["prog_tier_practicing"],
		ProgTierMastered:       payload["prog_tier_mastered"],
		NewWordMode0:           payload["new_word_mode_0"],
		NewWordMode1:           payload["new_word_mode_1"],
		NewWordMode2:           payload["new_word_mode_2"],
		BaselineNewBucketValue: -1,
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for negative baseline_new_bucket_value, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSettings_MaxNewWordsPerDay_Invalid(t *testing.T) {
	r := newRouter(openTestDB(t))

	type payload struct {
		PrimaryLang        string `json:"primary_lang"`
		SecondaryLang      string `json:"secondary_lang"`
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
	rec := do(t, r, http.MethodPatch, "/api/settings", payload{
		PrimaryLang:        "en",
		SecondaryLang:      "de",
		ProgNew:            "zh_to_transl",
		ProgTierStruggling: "transl_to_zh",
		ProgTierLearning:   "zh_pinyin_to_transl",
		ProgTierPracticing: "zh_to_transl",
		ProgTierMastered:   "random",
		NewWordMode0:       "transl_to_zh",
		NewWordMode1:       "zh_pinyin_to_transl",
		NewWordMode2:       "zh_to_transl",
		MaxNewWordsPerDay:  0,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for max_new_words_per_day=0, got %d", rec.Code)
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

// ── Settings: new word cooldown ───────────────────────────────────────────────

func TestGetSettings_CooldownDefault(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.NewWordCooldownMinutes < 0 {
		t.Errorf("want NewWordCooldownMinutes >= 0, got %d", st.NewWordCooldownMinutes)
	}
}

func TestPatchSettings_Cooldown(t *testing.T) {
	r := newRouter(openTestDB(t))

	type payload struct {
		PrimaryLang            string `json:"primary_lang"`
		SecondaryLang          string `json:"secondary_lang"`
		ProgNew                string `json:"prog_new"`
		ProgTierStruggling     string `json:"prog_tier_struggling"`
		ProgTierLearning       string `json:"prog_tier_learning"`
		ProgTierPracticing     string `json:"prog_tier_practicing"`
		ProgTierMastered       string `json:"prog_tier_mastered"`
		NewWordMode0           string `json:"new_word_mode_0"`
		NewWordMode1           string `json:"new_word_mode_1"`
		NewWordMode2           string `json:"new_word_mode_2"`
		MaxNewWordsPerDay      int    `json:"max_new_words_per_day"`
		NewWordCooldownMinutes int    `json:"new_word_cooldown_minutes"`
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload{
		PrimaryLang:            "en",
		SecondaryLang:          "de",
		ProgNew:                "zh_to_transl",
		ProgTierStruggling:     "transl_to_zh",
		ProgTierLearning:       "zh_pinyin_to_transl",
		ProgTierPracticing:     "zh_to_transl",
		ProgTierMastered:       "random",
		NewWordMode0:           "transl_to_zh",
		NewWordMode1:           "zh_pinyin_to_transl",
		NewWordMode2:           "zh_to_transl",
		MaxNewWordsPerDay:      5,
		NewWordCooldownMinutes: 30,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.NewWordCooldownMinutes != 30 {
		t.Errorf("want NewWordCooldownMinutes=30, got %d", st.NewWordCooldownMinutes)
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

// uploadRouterWithLimits builds a minimal authenticated router whose CSV handler
// uses the given DoS limits, so the body-size and row caps can be exercised
// without constructing multi-megabyte payloads.
func uploadRouterWithLimits(s *db.Store, maxBytes int64, maxRows int) http.Handler {
	h := &handlers.UploadCSVHandler{Store: s, MaxBytes: maxBytes, MaxRows: maxRows}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Post("/api/words/upload-csv", h.UploadCSV)
	return r
}

func TestUploadCSV_RejectsOversizedBody(t *testing.T) {
	// Tiny body cap; the multipart payload below exceeds it.
	r := uploadRouterWithLimits(openTestDB(t), 500, 5000)
	var b strings.Builder
	b.WriteString("chinese,pinyin,en\n")
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "字%d,zì,meaning number %d here padding padding\n", i, i)
	}
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "t", "start_training_count": "0"}, b.String())
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 for oversized body, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadCSV_RejectsTooManyRows(t *testing.T) {
	// Row cap of 2; the CSV below has 3 data rows.
	r := uploadRouterWithLimits(openTestDB(t), 0, 2)
	csv := "chinese,pinyin,en\n一,yī,one\n二,èr,two\n三,sān,three"
	rec := doMultipart(t, r, "/api/words/upload-csv",
		map[string]string{"tags": "t", "start_training_count": "0"}, csv)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for too many rows, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "too many rows") {
		t.Errorf("expected a 'too many rows' message, got %s", rec.Body.String())
	}
}

// ── GET /api/quiz/match-game ──────────────────────────────────────────────────

func TestMatchGame_EmptyWhenFewerThan2Pairs(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})
	// Only 1 confusion pair — game should not trigger
	if _, err := s.ExecForTest(`INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen) VALUES (2, ?, ?, 'zh_to_transl', 1, datetime('now'))`, id1, id2); err != nil {
		t.Fatal(err)
	}
	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	words := resp["words"].([]any)
	if len(words) != 0 {
		t.Errorf("expected 0 words, got %d", len(words))
	}
}

func TestMatchGame_Returns4UniqueWordsFrom2Pairs(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})
	id3 := seedWord(t, s, "谢谢", "xiè xie", []string{"thank you"})
	id4 := seedWord(t, s, "对不起", "duì bu qǐ", []string{"sorry"})
	// 2 distinct pairs → 4 unique words
	for _, pair := range [][2]int64{{id1, id2}, {id3, id4}} {
		if _, err := s.ExecForTest(`INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen) VALUES (2, ?, ?, 'zh_to_transl', 1, datetime('now'))`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	words := resp["words"].([]any)
	if len(words) != 4 {
		t.Errorf("expected 4 words, got %d", len(words))
	}
}

func TestMatchGame_DeduplicatesOverlappingPairs(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})
	id3 := seedWord(t, s, "谢谢", "xiè xie", []string{"thank you"})
	// Pair (1→2) and (2→3): word id2 appears in both, so only 3 unique words
	for _, pair := range [][2]int64{{id1, id2}, {id2, id3}} {
		if _, err := s.ExecForTest(`INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen) VALUES (2, ?, ?, 'zh_to_transl', 1, datetime('now'))`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	words := resp["words"].([]any)
	if len(words) != 3 {
		t.Errorf("expected 3 unique words, got %d", len(words))
	}
}

func TestMatchGame_MarksShownAndHidesOnSecondCall(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})
	id3 := seedWord(t, s, "谢谢", "xiè xie", []string{"thank you"})
	id4 := seedWord(t, s, "对不起", "duì bu qǐ", []string{"sorry"})
	for _, pair := range [][2]int64{{id1, id2}, {id3, id4}} {
		if _, err := s.ExecForTest(`INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen) VALUES (2, ?, ?, 'zh_to_transl', 1, datetime('now'))`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	r := newRouter(s)

	// First call returns words
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	var resp1 map[string]any
	decodeJSON(t, rec, &resp1)
	if len(resp1["words"].([]any)) == 0 {
		t.Fatal("first call: expected words")
	}

	// Second call returns empty (pairs marked as shown)
	rec2 := do(t, r, "GET", "/api/quiz/match-game", nil)
	var resp2 map[string]any
	decodeJSON(t, rec2, &resp2)
	if len(resp2["words"].([]any)) != 0 {
		t.Errorf("second call: expected 0 words, got %d", len(resp2["words"].([]any)))
	}
}

// TestMatchGame_IncludesComponentPairs covers issue #280: component-vs-word
// confusion pairs (created by a wrong component answer) must feed into the
// match game the same way word-vs-word pairs already do.
func TestMatchGame_IncludesComponentPairs(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})
	if err := s.UpsertComponentConfusion(ctx, int64(2), "扑", 0, "去", "zh_pinyin_to_transl"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExecForTest(`INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_id, mode, count, last_seen) VALUES (2, ?, ?, 'zh_to_transl', 1, datetime('now'))`, id1, id2); err != nil {
		t.Fatal(err)
	}

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.MatchGameResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Words) != 4 {
		t.Fatalf("expected 4 words (2 words + 2 components), got %d: %+v", len(resp.Words), resp.Words)
	}
	var sawComponent bool
	for _, w := range resp.Words {
		if w.Kind == models.ConfusionKindComponent {
			sawComponent = true
			if w.Character == "" {
				t.Errorf("component word missing character: %+v", w)
			}
		} else if w.Kind != models.ConfusionKindWord {
			t.Errorf("unexpected kind %q", w.Kind)
		}
	}
	if !sawComponent {
		t.Error("expected at least one component-kind word in the match game")
	}
}

// ── POST /api/quiz/match-answer ───────────────────────────────────────────────

func TestMatchAnswer_ComponentKind_RecordsComponentProgress(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	s.InsertComponentProgressForTest(ctx, int64(2), "扑", time.Now())

	r := newRouter(s)
	body := map[string]any{"kind": "component", "character": "扑", "correct": true}
	rec := do(t, r, "POST", "/api/quiz/match-answer", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("expected correct=true")
	}
	if resp.TotalAttempts != 1 || resp.TotalCorrect != 1 {
		t.Errorf("expected 1 attempt/1 correct, got attempts=%d correct=%d", resp.TotalAttempts, resp.TotalCorrect)
	}

	progress, err := s.GetComponentProgress(ctx, int64(2), "扑")
	if err != nil {
		t.Fatal(err)
	}
	if progress == nil || progress.TotalAttempts != 1 {
		t.Errorf("expected component_progress to be updated, got %+v", progress)
	}
}

func TestMatchAnswer_ComponentKind_MissingCharacter(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, "POST", "/api/quiz/match-answer", map[string]any{"kind": "component", "correct": true})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMatchAnswer_Correct(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)
	body := map[string]any{"zh_word_id": id, "correct": true}
	rec := do(t, r, "POST", "/api/quiz/match-answer", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["correct"] != true {
		t.Errorf("expected correct=true, got %v", resp["correct"])
	}
	if resp["zh_text"] != "你好" {
		t.Errorf("expected zh_text=你好, got %v", resp["zh_text"])
	}
}

func TestMatchAnswer_Wrong(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)
	body := map[string]any{"zh_word_id": id, "correct": false}
	rec := do(t, r, "POST", "/api/quiz/match-answer", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["correct"] != false {
		t.Errorf("expected correct=false, got %v", resp["correct"])
	}
}

func TestMatchAnswer_MissingWordID(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, "POST", "/api/quiz/match-answer", map[string]any{"correct": true})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMatchAnswer_WordNotFound(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, "POST", "/api/quiz/match-answer", map[string]any{"zh_word_id": 9999, "correct": true})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ── PATCH /api/settings — gamification ───────────────────────────────────────

func TestSettingsPatch_GamificationFields(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	body := baseSettingsPatch()
	body["gamification_enabled"] = true
	body["gamification_frequency"] = 10
	rec := do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec2, &st)
	if st["gamification_enabled"] != true {
		t.Errorf("gamification_enabled: got %v", st["gamification_enabled"])
	}
	if st["gamification_frequency"].(float64) != 10 {
		t.Errorf("gamification_frequency: got %v", st["gamification_frequency"])
	}
}

func TestSettingsPatch_GamificationFrequencyValidation(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	for _, freq := range []int{0, 1441} {
		body := baseSettingsPatch()
		body["gamification_frequency"] = freq
		rec := do(t, r, "PATCH", "/api/settings", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("frequency=%d: expected 400, got %d", freq, rec.Code)
		}
	}
}

func TestSettingsPatch_BlurPinyin(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["blur_pinyin"] != false {
		t.Errorf("blur_pinyin: want false by default, got %v", st["blur_pinyin"])
	}

	body := baseSettingsPatch()
	body["blur_pinyin"] = true
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["blur_pinyin"] != true {
		t.Errorf("blur_pinyin: want true after update, got %v", st["blur_pinyin"])
	}
}

func TestSettingsPatch_NoAutoVoiceOnBlur(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["no_auto_voice_on_blur"] != false {
		t.Errorf("no_auto_voice_on_blur: want false by default, got %v", st["no_auto_voice_on_blur"])
	}

	body := baseSettingsPatch()
	body["no_auto_voice_on_blur"] = true
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["no_auto_voice_on_blur"] != true {
		t.Errorf("no_auto_voice_on_blur: want true after update, got %v", st["no_auto_voice_on_blur"])
	}
}

func TestSettingsPatch_CelebrateBucketChange(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["celebrate_bucket_change"] != false {
		t.Errorf("celebrate_bucket_change: want false by default, got %v", st["celebrate_bucket_change"])
	}

	body := baseSettingsPatch()
	body["celebrate_bucket_change"] = true
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["celebrate_bucket_change"] != true {
		t.Errorf("celebrate_bucket_change: want true after update, got %v", st["celebrate_bucket_change"])
	}
}

func TestSettingsPatch_VoiceUnavailable(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["voice_unavailable"] != false {
		t.Errorf("voice_unavailable: want false by default, got %v", st["voice_unavailable"])
	}

	body := baseSettingsPatch()
	body["voice_unavailable"] = true
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["voice_unavailable"] != true {
		t.Errorf("voice_unavailable: want true after update, got %v", st["voice_unavailable"])
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

// ── GET /api/audio/component/{char} ──────────────────────────────────────────

func TestServeComponentAudio_ServesPreCachedFile(t *testing.T) {
	tmpDir := t.TempDir()
	audioH := &handlers.AudioHandler{Store: openTestDB(t), AudioDir: tmpDir}

	// Pre-seed the cached file using the expected c_{hex}.mp3 naming pattern.
	// 木 = U+6728
	cachedPath := filepath.Join(tmpDir, "c_6728.mp3")
	if err := os.WriteFile(cachedPath, []byte("fake-mp3-wood"), 0644); err != nil {
		t.Fatalf("seed mp3: %v", err)
	}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/component/{char}", audioH.ServeComponentAudio)

	req := httptest.NewRequest("GET", "/api/audio/component/"+url.PathEscape("木"), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "fake-mp3-wood") {
		t.Errorf("want cached content, got %q", rec.Body.String())
	}
}

func TestServeComponentAudio_GeneratesOnDemand(t *testing.T) {
	tmpDir := t.TempDir()
	synthCalled := ""
	audioH := &handlers.AudioHandler{
		Store:    openTestDB(t),
		AudioDir: tmpDir,
		Synth:    func(text string) ([]byte, error) { synthCalled = text; return []byte("synth-mp3"), nil },
	}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/component/{char}", audioH.ServeComponentAudio)

	req := httptest.NewRequest("GET", "/api/audio/component/"+url.PathEscape("女"), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if synthCalled != "女" {
		t.Errorf("want synth called with 女, got %q", synthCalled)
	}
	// File must be written with c_{hex}.mp3 pattern (女 = U+5973).
	if _, err := os.Stat(filepath.Join(tmpDir, "c_5973.mp3")); err != nil {
		t.Errorf("expected c_5973.mp3 to exist after generation: %v", err)
	}
}

func TestServeComponentAudio_InvalidChar(t *testing.T) {
	audioH := &handlers.AudioHandler{Store: openTestDB(t), AudioDir: t.TempDir()}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Get("/api/audio/component/{char}", audioH.ServeComponentAudio)

	// Multi-character value must be rejected with 400.
	req := httptest.NewRequest("GET", "/api/audio/component/"+url.PathEscape("木火"), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("multi-char: want 400, got %d", rec.Code)
	}
}

func TestServeComponentAudio_FilenameDifferentFromWordIDs(t *testing.T) {
	// Ensure component files (c_{hex}.mp3) cannot collide with word audio files
	// ({integer}.mp3). A hex codepoint like "6728" must NOT be a valid word ID
	// file — verified by checking the naming prefix distinguishes them.
	wordFile := "42.mp3"
	componentFile := fmt.Sprintf("c_%04x.mp3", []rune("木")[0]) // c_6728.mp3
	if wordFile == componentFile {
		t.Error("component filename pattern must not match word id filename pattern")
	}
	if !strings.HasPrefix(componentFile, "c_") {
		t.Error("component filename must start with c_")
	}
}

func TestServeComponentAudio_RadicalUsesCanonicalFormForTTS(t *testing.T) {
	// Radical variant characters (e.g. 扌 U+624C) should have TTS generated
	// using the canonical/pronounceable character (手 U+624B), while the cached
	// file is still named after the actual component codepoint (c_624c.mp3).
	cases := []struct {
		radical   string // the radical variant shown in the quiz
		canonical string // what TTS should receive
		wantFile  string // expected cached filename
	}{
		{"扌", "手", "c_624c.mp3"}, // hand radical
		{"氵", "水", "c_6c35.mp3"}, // water (3-dot)
		{"亻", "人", "c_4ebb.mp3"}, // person radical
		{"讠", "言", "c_8ba0.mp3"}, // speech radical
	}

	for _, tc := range cases {
		t.Run(tc.radical, func(t *testing.T) {
			tmpDir := t.TempDir()
			var synthGot string
			audioH := &handlers.AudioHandler{
				Store:    openTestDB(t),
				AudioDir: tmpDir,
				Synth:    func(text string) ([]byte, error) { synthGot = text; return []byte("synth-mp3"), nil },
			}

			r := chi.NewRouter()
			r.Use(handlers.WithUserID(2))
			r.Get("/api/audio/component/{char}", audioH.ServeComponentAudio)

			req := httptest.NewRequest("GET", "/api/audio/component/"+url.PathEscape(tc.radical), nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if synthGot != tc.canonical {
				t.Errorf("TTS called with %q, want canonical %q", synthGot, tc.canonical)
			}
			if _, err := os.Stat(filepath.Join(tmpDir, tc.wantFile)); err != nil {
				t.Errorf("expected %s to exist after generation: %v", tc.wantFile, err)
			}
		})
	}
}

// ── Difficult-words drill ───────────────────────────────────────────────────

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
