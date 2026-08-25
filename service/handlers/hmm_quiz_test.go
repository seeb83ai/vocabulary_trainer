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

func hmmQuizRouter(t *testing.T) (http.Handler, *handlers.HMMQuizHandler) {
	t.Helper()
	store := openTestDB(t)
	h := &handlers.HMMQuizHandler{Store: store}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(2))
	r.Post("/api/hmm-quiz/answer", h.Answer)
	return r, h
}

// clearAllHMMNames blanks every library entry so no entries qualify as named.
// Migration v13 seeds actor "null" (Jackie Chan), 5 tone rooms, and 2 props
// with non-empty names; this resets everything to a blank state.
func clearAllHMMNames(t *testing.T, h *handlers.HMMQuizHandler) {
	t.Helper()
	ctx := context.Background()
	// Blank all actors that have names (migration seeds "null" → "Jackie Chan")
	actors, err := h.Store.GetHMMActors(ctx, int64(2))
	if err != nil {
		t.Fatalf("clearAllHMMNames GetHMMActors: %v", err)
	}
	for _, a := range actors {
		if a.ActorName != "" {
			if err := h.Store.UpdateHMMActor(ctx, int64(2), a.Initial, ""); err != nil {
				t.Fatalf("clearAllHMMNames UpdateHMMActor %s: %v", a.Initial, err)
			}
		}
	}
	for tone := 1; tone <= 5; tone++ {
		if err := h.Store.UpdateHMMToneRoom(ctx, int64(2), tone, ""); err != nil {
			t.Fatalf("clearAllHMMNames tone %d: %v", tone, err)
		}
	}
	for _, radical := range []string{"一", "二"} {
		_ = h.Store.UpsertHMMProp(ctx, int64(2), radical, "")
	}
}

// seedHMMActorEntry uses public Store methods to add a named actor and ensure
// a progress row exists. It also clears tone room names to avoid interference.
func seedHMMActorEntry(t *testing.T, h *handlers.HMMQuizHandler, initial, name string) {
	t.Helper()
	ctx := context.Background()
	clearAllHMMNames(t, h)
	if err := h.Store.UpdateHMMActor(ctx, int64(2), initial, name); err != nil {
		t.Fatalf("UpdateHMMActor: %v", err)
	}
	if err := h.Store.EnsureHMMProgress(ctx, int64(2)); err != nil {
		t.Fatalf("EnsureHMMProgress: %v", err)
	}
}

func TestHMMQuizAnswer_Correct(t *testing.T) {
	router, h := hmmQuizRouter(t)
	seedHMMActorEntry(t, h, "b", "Bruce Lee")

	rec := do(t, router, "POST", "/api/hmm-quiz/answer", models.HMMAnswerRequest{
		EntityType: models.HMMEntityActor,
		EntityKey:  "b",
		Answer:     "Bruce Lee",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp models.HMMAnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("expected correct = true")
	}
	if resp.CorrectAnswer != "Bruce Lee" {
		t.Errorf("correct_answer = %q, want 'Bruce Lee'", resp.CorrectAnswer)
	}
	if resp.YourAnswer != "" {
		t.Errorf("your_answer should be empty on correct answer, got %q", resp.YourAnswer)
	}
}

func TestHMMQuizAnswer_Wrong(t *testing.T) {
	router, h := hmmQuizRouter(t)
	seedHMMActorEntry(t, h, "b", "Bruce Lee")

	rec := do(t, router, "POST", "/api/hmm-quiz/answer", models.HMMAnswerRequest{
		EntityType: models.HMMEntityActor,
		EntityKey:  "b",
		Answer:     "Jackie Chan",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp models.HMMAnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Correct {
		t.Error("expected correct = false")
	}
	if resp.YourAnswer != "Jackie Chan" {
		t.Errorf("your_answer = %q, want 'Jackie Chan'", resp.YourAnswer)
	}
	if resp.CorrectAnswer != "Bruce Lee" {
		t.Errorf("correct_answer = %q, want 'Bruce Lee'", resp.CorrectAnswer)
	}
}

func TestHMMQuizAnswer_Wrong_IncludesTier(t *testing.T) {
	router, h := hmmQuizRouter(t)
	seedHMMActorEntry(t, h, "b", "Bruce Lee")

	rec := do(t, router, "POST", "/api/hmm-quiz/answer", models.HMMAnswerRequest{
		EntityType: models.HMMEntityActor,
		EntityKey:  "b",
		Answer:     "Jackie Chan",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp models.HMMAnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Tier != "New" {
		t.Errorf("tier = %q, want 'New' on a wrong answer for a fresh learning-phase entry", resp.Tier)
	}
}

func TestHMMQuizAnswer_CaseInsensitive(t *testing.T) {
	router, h := hmmQuizRouter(t)
	seedHMMActorEntry(t, h, "b", "Bruce Lee")

	rec := do(t, router, "POST", "/api/hmm-quiz/answer", models.HMMAnswerRequest{
		EntityType: models.HMMEntityActor,
		EntityKey:  "b",
		Answer:     "bruce lee",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp models.HMMAnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("expected case-insensitive match to be correct")
	}
}

func TestHMMQuizAnswer_OptionalParensPrefix(t *testing.T) {
	router, h := hmmQuizRouter(t)
	// Stored name has a bracketed prefix; user omits it.
	seedHMMActorEntry(t, h, "r", "(人) Arnold Schwarzenegger")

	rec := do(t, router, "POST", "/api/hmm-quiz/answer", models.HMMAnswerRequest{
		EntityType: models.HMMEntityActor,
		EntityKey:  "r",
		Answer:     "Arnold Schwarzenegger",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp models.HMMAnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("expected answer without bracketed prefix to be correct")
	}
}

func TestHMMQuizAnswer_TierOnFirstAttempt(t *testing.T) {
	router, h := hmmQuizRouter(t)
	seedHMMActorEntry(t, h, "b", "Bruce Lee")

	rec := do(t, router, "POST", "/api/hmm-quiz/answer", models.HMMAnswerRequest{
		EntityType: models.HMMEntityActor,
		EntityKey:  "b",
		Answer:     "Bruce Lee",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp models.HMMAnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Tier != "New" {
		t.Errorf("tier = %q, want 'New' for a fresh learning-phase entry", resp.Tier)
	}
	if resp.PrevTier != "" {
		t.Errorf("prev_tier = %q, want empty on first-ever attempt", resp.PrevTier)
	}
}

func TestHMMQuizAnswer_TierGraduatesFromLearningPhase(t *testing.T) {
	router, h := hmmQuizRouter(t)
	seedHMMActorEntry(t, h, "b", "Bruce Lee")
	ctx := context.Background()

	// Two correct answers in the learning phase (graduate reps = 3);
	// the third correct answer graduates the entry out of "New".
	for i := 0; i < 2; i++ {
		rec := do(t, router, "POST", "/api/hmm-quiz/answer", models.HMMAnswerRequest{
			EntityType: models.HMMEntityActor,
			EntityKey:  "b",
			Answer:     "Bruce Lee",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
	}
	progress, err := h.Store.GetHMMProgress(ctx, int64(2), models.HMMEntityActor, "b")
	if err != nil || progress == nil {
		t.Fatalf("GetHMMProgress: %v", err)
	}
	if !progress.Learning {
		t.Fatalf("expected entry to still be in learning phase after 2 correct answers")
	}

	rec := do(t, router, "POST", "/api/hmm-quiz/answer", models.HMMAnswerRequest{
		EntityType: models.HMMEntityActor,
		EntityKey:  "b",
		Answer:     "Bruce Lee",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp models.HMMAnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.PrevTier != "New" {
		t.Errorf("prev_tier = %q, want 'New'", resp.PrevTier)
	}
	if resp.Tier == "" || resp.Tier == "New" {
		t.Errorf("tier = %q, want a graduated tier after leaving the learning phase", resp.Tier)
	}
}

func TestHMMQuizAnswer_OptionalParensInline(t *testing.T) {
	router, h := hmmQuizRouter(t)
	// Stored name has bracketed segments inline; user omits them.
	seedHMMActorEntry(t, h, "r", "Kreuz (十) und Rasiermesser (一)")

	rec := do(t, router, "POST", "/api/hmm-quiz/answer", models.HMMAnswerRequest{
		EntityType: models.HMMEntityActor,
		EntityKey:  "r",
		Answer:     "Kreuz und Rasiermesser",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp models.HMMAnswerResponse
	decodeJSON(t, rec, &resp)
	if !resp.Correct {
		t.Error("expected answer without inline bracketed segments to be correct")
	}
}

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
