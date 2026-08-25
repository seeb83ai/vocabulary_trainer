package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

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
	demoH := &handlers.DemoHandler{Store: s}
	r.Get("/api/demo/cards", demoH.Cards)
	r.Post("/api/demo/answer", demoH.Answer)
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
	r.Get("/api/components/coverage", componentH.Coverage)
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
	adminH := &handlers.AdminHandler{Store: s}
	r.With(handlers.RequireAdmin(s)).Get("/api/admin/overview", adminH.Overview)
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
