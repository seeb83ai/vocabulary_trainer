package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

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
