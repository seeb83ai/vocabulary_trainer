package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

func hmmQuizRouter(t *testing.T) (http.Handler, *handlers.HMMQuizHandler) {
	t.Helper()
	store := openTestDB(t)
	h := &handlers.HMMQuizHandler{Store: store}
	r := chi.NewRouter()
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
	actors, err := h.Store.GetHMMActors(ctx)
	if err != nil {
		t.Fatalf("clearAllHMMNames GetHMMActors: %v", err)
	}
	for _, a := range actors {
		if a.ActorName != "" {
			if err := h.Store.UpdateHMMActor(ctx, a.Initial, ""); err != nil {
				t.Fatalf("clearAllHMMNames UpdateHMMActor %s: %v", a.Initial, err)
			}
		}
	}
	for tone := 1; tone <= 5; tone++ {
		if err := h.Store.UpdateHMMToneRoom(ctx, tone, ""); err != nil {
			t.Fatalf("clearAllHMMNames tone %d: %v", tone, err)
		}
	}
	for _, radical := range []string{"一", "二"} {
		_ = h.Store.UpsertHMMProp(ctx, radical, "")
	}
}

// seedHMMActorEntry uses public Store methods to add a named actor and ensure
// a progress row exists. It also clears tone room names to avoid interference.
func seedHMMActorEntry(t *testing.T, h *handlers.HMMQuizHandler, initial, name string) {
	t.Helper()
	ctx := context.Background()
	clearAllHMMNames(t, h)
	if err := h.Store.UpdateHMMActor(ctx, initial, name); err != nil {
		t.Fatalf("UpdateHMMActor: %v", err)
	}
	if err := h.Store.EnsureHMMProgress(ctx, int64(1)); err != nil {
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

