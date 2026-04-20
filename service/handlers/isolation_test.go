package handlers_test

// isolation_test.go — cross-user data isolation tests.
//
// Strategy:
//   - Build two routers against the same in-memory DB: one for user 1 (admin)
//     and one for user 2 (me).
//   - Create words / HMM library entries as one user.
//   - Assert the other user cannot read, modify, or delete those resources.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

// newRouterForUser returns a chi router wired to the given store and user ID.
// All standard word / quiz / HMM routes are registered.
func newRouterForUser(s *db.Store, userID int64) http.Handler {
	wordsH := &handlers.WordsHandler{Store: s}
	quizH := &handlers.QuizHandler{Store: s, MaxNewPerDay: 100}
	hmmH := &handlers.HMMHandler{Store: s}
	mismatchH := &handlers.MismatchesHandler{Store: s}

	r := chi.NewRouter()
	r.Use(handlers.WithUserID(userID))

	r.Get("/api/quiz/next", quizH.Next)
	r.Post("/api/quiz/answer", quizH.Answer)
	r.Post("/api/quiz/skip", quizH.Skip)
	r.Post("/api/quiz/acknowledge", quizH.Acknowledge)
	r.Post("/api/quiz/advance", quizH.Advance)
	r.Get("/api/quiz/stats", quizH.Stats)
	r.Get("/api/quiz/word-stats", quizH.WordStats)
	r.Get("/api/quiz/due-date-distribution", quizH.DueDateDistribution)

	r.Route("/api/words", func(r chi.Router) {
		r.Get("/", wordsH.List)
		r.Post("/", wordsH.Create)
		r.Get("/export", wordsH.Export)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", wordsH.GetByID)
			r.Put("/", wordsH.Update)
			r.Delete("/", wordsH.Delete)
			r.Post("/review", wordsH.MarkReview)
			r.Put("/hmm", hmmH.SaveScene)
			r.Delete("/hmm", hmmH.DeleteScene)
		})
	})

	r.Route("/api/hmm", func(r chi.Router) {
		r.Get("/actors", hmmH.GetActors)
		r.Put("/actors/{initial}", hmmH.UpdateActor)
		r.Get("/locations", hmmH.GetLocations)
		r.Put("/locations/{final}", hmmH.UpdateLocation)
		r.Get("/tone-rooms", hmmH.GetToneRooms)
		r.Put("/tone-rooms/{tone}", hmmH.UpdateToneRoom)
		r.Get("/props", hmmH.GetProps)
		r.Put("/props", hmmH.UpsertProp)
	})

	r.Get("/api/mismatches", mismatchH.List)

	return r
}

// seedWordForUser creates a word directly in the DB owned by the given user.
func seedWordForUser(t *testing.T, s *db.Store, userID int64, zhText, pinyin string, enTexts []string) int64 {
	t.Helper()
	id, err := s.CreateWord(context.Background(), userID, models.CreateWordRequest{
		ZhText:  zhText,
		Pinyin:  pinyin,
		Translations: map[string][]string{"en": enTexts},
	})
	if err != nil {
		t.Fatalf("seedWordForUser(user=%d, %q): %v", userID, zhText, err)
	}
	return id
}

// ── Words: List isolation ─────────────────────────────────────────────────────

func TestIsolation_WordList_UserSeesOnlyOwnWords(t *testing.T) {
	s := openTestDB(t)

	// User 1 owns "再见"; user 2 owns "你好".
	seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})
	seedWordForUser(t, s, 2, "你好", "nǐ hǎo", []string{"hello"})

	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	// User 1 should see exactly "再见".
	rec := do(t, r1, "GET", "/api/words/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user1 list: want 200, got %d", rec.Code)
	}
	var resp1 models.WordListResponse
	decodeJSON(t, rec, &resp1)
	if resp1.Total != 1 {
		t.Errorf("user1 list: want 1 word, got %d", resp1.Total)
	}
	if resp1.Total == 1 && resp1.Words[0].ZhText != "再见" {
		t.Errorf("user1 list: want 再见, got %q", resp1.Words[0].ZhText)
	}

	// User 2 should see exactly "你好".
	rec = do(t, r2, "GET", "/api/words/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 list: want 200, got %d", rec.Code)
	}
	var resp2 models.WordListResponse
	decodeJSON(t, rec, &resp2)
	if resp2.Total != 1 {
		t.Errorf("user2 list: want 1 word, got %d", resp2.Total)
	}
	if resp2.Total == 1 && resp2.Words[0].ZhText != "你好" {
		t.Errorf("user2 list: want 你好, got %q", resp2.Words[0].ZhText)
	}
}

// ── Words: GetByID isolation ──────────────────────────────────────────────────

func TestIsolation_GetWordByID_CannotAccessOtherUsersWord(t *testing.T) {
	s := openTestDB(t)
	idA := seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})
	idB := seedWordForUser(t, s, 2, "你好", "nǐ hǎo", []string{"hello"})

	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	// User 1 can fetch their own word.
	rec := do(t, r1, "GET", fmt.Sprintf("/api/words/%d/", idA), nil)
	if rec.Code != http.StatusOK {
		t.Errorf("user1 get own word: want 200, got %d", rec.Code)
	}

	// User 2 cannot fetch user 1's word.
	rec = do(t, r2, "GET", fmt.Sprintf("/api/words/%d/", idA), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("user2 fetch user1 word: want 404, got %d", rec.Code)
	}

	// User 1 cannot fetch user 2's word.
	rec = do(t, r1, "GET", fmt.Sprintf("/api/words/%d/", idB), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("user1 fetch user2 word: want 404, got %d", rec.Code)
	}
}

// ── Words: Delete isolation ───────────────────────────────────────────────────

func TestIsolation_DeleteWord_CannotDeleteOtherUsersWord(t *testing.T) {
	s := openTestDB(t)
	idA := seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})

	r2 := newRouterForUser(s, 2)

	// User 2 tries to delete user 1's word — should get 404.
	rec := do(t, r2, "DELETE", fmt.Sprintf("/api/words/%d/", idA), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("user2 delete user1 word: want 404, got %d", rec.Code)
	}

	// Verify the word still exists for user 1.
	r1 := newRouterForUser(s, 1)
	rec = do(t, r1, "GET", fmt.Sprintf("/api/words/%d/", idA), nil)
	if rec.Code != http.StatusOK {
		t.Errorf("user1 word should still exist after failed cross-user delete, got %d", rec.Code)
	}
}

// ── Words: Update isolation ───────────────────────────────────────────────────

func TestIsolation_UpdateWord_CannotUpdateOtherUsersWord(t *testing.T) {
	s := openTestDB(t)
	idA := seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})

	r2 := newRouterForUser(s, 2)

	rec := do(t, r2, "PUT", fmt.Sprintf("/api/words/%d/", idA), models.UpdateWordRequest{
		ZhText:  "再见",
		Pinyin:  "zàijiàn",
		Translations: map[string][]string{"en": {"see you"}},
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("user2 update user1 word: want 404, got %d", rec.Code)
	}
}

// ── Words: MarkReview isolation ───────────────────────────────────────────────

func TestIsolation_MarkReview_CannotMarkOtherUsersWord(t *testing.T) {
	s := openTestDB(t)
	idA := seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})

	r2 := newRouterForUser(s, 2)
	rec := do(t, r2, "POST", fmt.Sprintf("/api/words/%d/review", idA), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("user2 mark user1 word for review: want 404, got %d", rec.Code)
	}
}

// ── Quiz: Next card isolation ─────────────────────────────────────────────────

func TestIsolation_QuizNext_OnlyOwnWords(t *testing.T) {
	s := openTestDB(t)

	// User 1 has "再见"; user 2 has "你好".
	seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})
	seedWordForUser(t, s, 2, "你好", "nǐ hǎo", []string{"hello"})

	// Acknowledge both words so they become quizzable.
	ctx := context.Background()
	words1, _, _ := s.GetWords(ctx, 1, "", 1, 100, "", "", nil, false, false, "", "", "")
	for _, w := range words1 {
		_ = s.AcknowledgeWord(ctx, 1, w.ID)
	}
	words2, _, _ := s.GetWords(ctx, 2, "", 1, 100, "", "", nil, false, false, "", "", "")
	for _, w := range words2 {
		_ = s.AcknowledgeWord(ctx, 2, w.ID)
	}

	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	rec1 := do(t, r1, "GET", "/api/quiz/next", nil)
	if rec1.Code != http.StatusOK {
		t.Fatalf("user1 quiz/next: want 200, got %d: %s", rec1.Code, rec1.Body)
	}
	var card1 models.QuizCard
	decodeJSON(t, rec1, &card1)

	rec2 := do(t, r2, "GET", "/api/quiz/next", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("user2 quiz/next: want 200, got %d: %s", rec2.Code, rec2.Body)
	}
	var card2 models.QuizCard
	decodeJSON(t, rec2, &card2)

	// The two cards must be different words.
	if card1.WordID == card2.WordID {
		t.Errorf("both users got the same word_id=%d; expected each user's own word", card1.WordID)
	}
}

// ── Quiz: Stats isolation ─────────────────────────────────────────────────────

func TestIsolation_QuizStats_CountOnlyOwnWords(t *testing.T) {
	s := openTestDB(t)

	// User 1 has 3 words; user 2 has 1 word.
	for i := 0; i < 3; i++ {
		seedWordForUser(t, s, 1, fmt.Sprintf("词%d", i), "pīnyīn", []string{fmt.Sprintf("word%d", i)})
	}
	seedWordForUser(t, s, 2, "你好", "nǐ hǎo", []string{"hello"})

	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	rec1 := do(t, r1, "GET", "/api/quiz/stats", nil)
	if rec1.Code != http.StatusOK {
		t.Fatalf("user1 stats: %d %s", rec1.Code, rec1.Body)
	}
	var s1 map[string]int
	decodeJSON(t, rec1, &s1)
	if s1["total"] != 3 {
		t.Errorf("user1 stats total: want 3, got %d", s1["total"])
	}

	rec2 := do(t, r2, "GET", "/api/quiz/stats", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("user2 stats: %d %s", rec2.Code, rec2.Body)
	}
	var s2 map[string]int
	decodeJSON(t, rec2, &s2)
	if s2["total"] != 1 {
		t.Errorf("user2 stats total: want 1, got %d", s2["total"])
	}
}

// ── HMM actors: Library isolation ────────────────────────────────────────────
//
// The migration seeds HMM actor/tone-room rows only for user 2 (the personal
// user). User 1 (admin) has no HMM rows. These tests confirm:
//   - User 2 can name their own actors; user 1 sees an empty list.
//   - Updating an actor via the HTTP API only affects the requesting user.

func TestIsolation_HMMActors_UserSeesOnlyOwnLibrary(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// User 2 names actor "b".
	if err := s.UpdateHMMActor(ctx, 2, "b", "Bruce Lee"); err != nil {
		t.Fatalf("UpdateHMMActor user2: %v", err)
	}

	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	// User 1 has no HMM data — expects empty list.
	rec := do(t, r1, "GET", "/api/hmm/actors", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user1 get actors: %d", rec.Code)
	}
	var actors1 []models.HMMActor
	decodeJSON(t, rec, &actors1)
	if len(actors1) != 0 {
		t.Errorf("user1 should see 0 actors (no HMM data), got %d", len(actors1))
	}

	// User 2 should see their actors including "b" = "Bruce Lee".
	rec = do(t, r2, "GET", "/api/hmm/actors", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 get actors: %d", rec.Code)
	}
	var actors2 []models.HMMActor
	decodeJSON(t, rec, &actors2)
	found := false
	for _, a := range actors2 {
		if a.Initial == "b" {
			if a.ActorName != "Bruce Lee" {
				t.Errorf("user2 actor b: want 'Bruce Lee', got %q", a.ActorName)
			}
			found = true
		}
	}
	if !found {
		t.Error("user2 actor b not found in GET /api/hmm/actors")
	}
}

func TestIsolation_HMMActors_UpdateDoesNotAffectOtherUser(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Pre-set user 2 actor "c" to "Chris Rock".
	if err := s.UpdateHMMActor(ctx, 2, "c", "Chris Rock"); err != nil {
		t.Fatalf("UpdateHMMActor user2: %v", err)
	}

	r2 := newRouterForUser(s, 2)

	// User 2 updates their actor "c" to "Chuck Norris".
	rec := do(t, r2, "PUT", "/api/hmm/actors/c", map[string]string{"actor_name": "Chuck Norris"})
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 update actor: want 200, got %d %s", rec.Code, rec.Body)
	}

	// User 1 has no actor rows — their list should remain empty.
	actors1, err := s.GetHMMActors(ctx, 1)
	if err != nil {
		t.Fatalf("GetHMMActors user1: %v", err)
	}
	if len(actors1) != 0 {
		t.Errorf("user1 should have no actors after user2 update, got %d", len(actors1))
	}

	// User 2's actor "c" should be "Chuck Norris".
	actors2, err := s.GetHMMActors(ctx, 2)
	if err != nil {
		t.Fatalf("GetHMMActors user2: %v", err)
	}
	found := false
	for _, a := range actors2 {
		if a.Initial == "c" {
			if a.ActorName != "Chuck Norris" {
				t.Errorf("user2 actor c: want 'Chuck Norris', got %q", a.ActorName)
			}
			found = true
		}
	}
	if !found {
		t.Error("user2 actor c not found")
	}
}

// ── HMM tone rooms: Library isolation ────────────────────────────────────────

func TestIsolation_HMMToneRooms_UserSeesOnlyOwnRooms(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// User 2 updates tone 1.
	if err := s.UpdateHMMToneRoom(ctx, 2, 1, "User2 tone1 room"); err != nil {
		t.Fatalf("UpdateHMMToneRoom user2: %v", err)
	}

	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	// User 1 has no tone-room rows — expects empty list.
	rec := do(t, r1, "GET", "/api/hmm/tone-rooms", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user1 get tone-rooms: %d", rec.Code)
	}
	var rooms1 []models.HMMToneRoom
	decodeJSON(t, rec, &rooms1)
	if len(rooms1) != 0 {
		t.Errorf("user1 should see 0 tone rooms (no HMM data), got %d", len(rooms1))
	}

	// User 2 should see their 5 tone rooms including the named tone 1.
	rec = do(t, r2, "GET", "/api/hmm/tone-rooms", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 get tone-rooms: %d", rec.Code)
	}
	var rooms2 []models.HMMToneRoom
	decodeJSON(t, rec, &rooms2)
	found := false
	for _, r := range rooms2 {
		if r.Tone == 1 {
			if r.RoomName != "User2 tone1 room" {
				t.Errorf("user2 tone1: want 'User2 tone1 room', got %q", r.RoomName)
			}
			found = true
		}
	}
	if !found {
		t.Error("user2 tone 1 not found in GET /api/hmm/tone-rooms")
	}
}

// ── Words: Creation is isolated ───────────────────────────────────────────────

func TestIsolation_CreateWord_WordOnlyVisibleToCreator(t *testing.T) {
	s := openTestDB(t)
	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	// User 1 creates a word via the HTTP API.
	rec := do(t, r1, "POST", "/api/words/", models.CreateWordRequest{
		ZhText:  "学习",
		Pinyin:  "xuéxí",
		Translations: map[string][]string{"en": {"to study"}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("user1 create word: want 201, got %d: %s", rec.Code, rec.Body)
	}
	var created map[string]int64
	decodeJSON(t, rec, &created)
	newID := created["id"]

	// User 2 cannot see it.
	rec = do(t, r2, "GET", fmt.Sprintf("/api/words/%d/", newID), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("user2 should not see user1's word: want 404, got %d", rec.Code)
	}

	// User 2's word list should be empty.
	rec = do(t, r2, "GET", "/api/words/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 list: %d", rec.Code)
	}
	var list models.WordListResponse
	decodeJSON(t, rec, &list)
	if list.Total != 0 {
		t.Errorf("user2 word list should be empty, got %d words", list.Total)
	}
}

// ── Quiz answer: word ownership enforced ──────────────────────────────────────

func TestIsolation_QuizAnswer_CannotAnswerOtherUsersWord(t *testing.T) {
	s := openTestDB(t)

	// User 1 creates a word.
	idA := seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})
	if err := s.AcknowledgeWord(context.Background(), 1, idA); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	r2 := newRouterForUser(s, 2)

	// User 2 tries to submit an answer for user 1's word.
	rec := do(t, r2, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: idA,
		Mode:   models.ModeZhToTransl,
		Answer: "goodbye",
	})
	// The handler looks up the word by user_id; it returns 404 for other users' words.
	if rec.Code != http.StatusNotFound {
		t.Errorf("user2 answer user1 word: want 404, got %d", rec.Code)
	}
}

// ── Due-date distribution isolation ──────────────────────────────────────────

func TestIsolation_DueDateDistribution_OnlyOwnWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// User 1 has a word that is seen (has first_seen_date set by AcknowledgeWord).
	idA := seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})
	if err := s.AcknowledgeWord(ctx, 1, idA); err != nil {
		t.Fatalf("AcknowledgeWord: %v", err)
	}

	// User 1 sees exactly 1 entry; user 2 sees 0.
	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	rec := do(t, r1, "GET", "/api/quiz/due-date-distribution", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user1 due-date-distribution: %d", rec.Code)
	}
	var dist1 models.DueDateDistributionResponse
	decodeJSON(t, rec, &dist1)
	if len(dist1.Dates) == 0 {
		t.Error("user1 should see at least one due-date bucket")
	}

	rec = do(t, r2, "GET", "/api/quiz/due-date-distribution", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 due-date-distribution: %d", rec.Code)
	}
	var dist2 models.DueDateDistributionResponse
	decodeJSON(t, rec, &dist2)
	if len(dist2.Dates) != 0 {
		t.Errorf("user2 should see no due dates (no words), got %d entries", len(dist2.Dates))
	}
}

// ── Quiz: Skip isolation ──────────────────────────────────────────────────────

func TestIsolation_Skip_CannotSkipOtherUsersWord(t *testing.T) {
	s := openTestDB(t)

	// User 1 owns a word.
	idA := seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})

	r2 := newRouterForUser(s, 2)

	// User 2 tries to skip user 1's word — must be rejected (404).
	rec := do(t, r2, "POST", "/api/quiz/skip", map[string]int64{"word_id": idA})
	if rec.Code != http.StatusNotFound {
		t.Errorf("user2 skip user1 word: want 404, got %d", rec.Code)
	}
}

// ── Quiz: Acknowledge isolation ───────────────────────────────────────────────

func TestIsolation_Acknowledge_CannotAcknowledgeOtherUsersWord(t *testing.T) {
	s := openTestDB(t)

	// User 1 owns a word.
	idA := seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})

	r2 := newRouterForUser(s, 2)

	rec := do(t, r2, "POST", "/api/quiz/acknowledge", map[string]int64{"word_id": idA})
	if rec.Code != http.StatusNotFound {
		t.Errorf("user2 acknowledge user1 word: want 404, got %d", rec.Code)
	}
}

// ── Quiz: Advance isolation ───────────────────────────────────────────────────

func TestIsolation_Advance_OnlyOwnWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// User 1 has a word with a future due date (acknowledged, then skipped forward).
	idA := seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})
	if err := s.AcknowledgeWord(ctx, 1, idA); err != nil {
		t.Fatalf("AcknowledgeWord user1: %v", err)
	}
	if err := s.SkipWord(ctx, 1, idA, 7); err != nil {
		t.Fatalf("SkipWord user1: %v", err)
	}

	r2 := newRouterForUser(s, 2)

	// User 2 advances 1 word — should advance 0 (they have no words).
	rec := do(t, r2, "POST", "/api/quiz/advance", map[string]any{"count": 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 advance: want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["advanced"].(float64) != 0 {
		t.Errorf("user2 advance: expected 0 words advanced (no own words), got %v", resp["advanced"])
	}

	// Verify user1's word still has a future due date via stats.
	// If advance had wrongly affected user1, available_to_advance would be 0.
	r1 := newRouterForUser(s, 1)
	rec = do(t, r1, "GET", "/api/quiz/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user1 stats: %d", rec.Code)
	}
	var stats map[string]int
	decodeJSON(t, rec, &stats)
	if stats["available_to_advance"] != 1 {
		t.Errorf("user1 available_to_advance: want 1, got %d (user2's advance should not affect user1)", stats["available_to_advance"])
	}
}

// ── HMM: Scene delete isolation ───────────────────────────────────────────────

func TestIsolation_HMMScene_DeleteCannotDeleteOtherUsersScene(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// User 1 has a word with a scene.
	idA := seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})
	if err := s.UpsertHMMScene(ctx, idA, "User1's vivid scene."); err != nil {
		t.Fatalf("UpsertHMMScene: %v", err)
	}

	r2 := newRouterForUser(s, 2)

	// User 2 tries to delete user 1's scene.
	rec := do(t, r2, "DELETE", fmt.Sprintf("/api/words/%d/hmm", idA), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("user2 delete user1 scene: want 404, got %d", rec.Code)
	}

	// Scene should still exist for user 1.
	scene, err := s.GetHMMScene(ctx, idA)
	if err != nil {
		t.Fatalf("GetHMMScene: %v", err)
	}
	if scene == nil || scene.SceneText != "User1's vivid scene." {
		t.Error("user1 scene was deleted or corrupted by user2's delete attempt")
	}
}

// ── HMM: Locations isolation ──────────────────────────────────────────────────

func TestIsolation_HMMLocations_UserSeesOnlyOwnLocations(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// User 2 names location "a" (the "a" final).
	if err := s.UpdateHMMLocation(ctx, 2, "a", "User2 Hotel"); err != nil {
		t.Fatalf("UpdateHMMLocation user2: %v", err)
	}

	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	// User 1 has no location rows — expects empty list.
	rec := do(t, r1, "GET", "/api/hmm/locations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user1 get locations: %d", rec.Code)
	}
	var locs1 []models.HMMLocation
	decodeJSON(t, rec, &locs1)
	if len(locs1) != 0 {
		t.Errorf("user1 should see 0 locations (no HMM data), got %d", len(locs1))
	}

	// User 2 should see their locations including "a" = "User2 Hotel".
	rec = do(t, r2, "GET", "/api/hmm/locations", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 get locations: %d", rec.Code)
	}
	var locs2 []models.HMMLocation
	decodeJSON(t, rec, &locs2)
	found := false
	for _, l := range locs2 {
		if l.FinalKey == "a" {
			if l.LocationName != "User2 Hotel" {
				t.Errorf("user2 location a: want 'User2 Hotel', got %q", l.LocationName)
			}
			found = true
		}
	}
	if !found {
		t.Error("user2 location 'a' not found in GET /api/hmm/locations")
	}
}

// ── HMM: Props isolation ──────────────────────────────────────────────────────

func TestIsolation_HMMProps_UserSeesOnlyOwnProps(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// User 2 upserts a prop.
	if err := s.UpsertHMMProp(ctx, 2, "人", "person radical"); err != nil {
		t.Fatalf("UpsertHMMProp user2: %v", err)
	}

	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	// User 1 has no props — expects empty list.
	rec := do(t, r1, "GET", "/api/hmm/props", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user1 get props: %d", rec.Code)
	}
	var props1 []models.HMMProp
	decodeJSON(t, rec, &props1)
	if len(props1) != 0 {
		t.Errorf("user1 should see 0 props, got %d", len(props1))
	}

	// User 2 should see their prop.
	rec = do(t, r2, "GET", "/api/hmm/props", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 get props: %d", rec.Code)
	}
	var props2 []models.HMMProp
	decodeJSON(t, rec, &props2)
	if len(props2) != 1 || props2[0].Radical != "人" {
		t.Errorf("user2 should see prop '人', got %v", props2)
	}
}

// ── Mismatches: Confusion pairs isolation ─────────────────────────────────────

func TestIsolation_Mismatches_OnlyOwnConfusions(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// User 1 has two words with a confusion pair between them.
	id1A := seedWordForUser(t, s, 1, "鞋", "xié", []string{"shoe"})
	id1B := seedWordForUser(t, s, 1, "书", "shū", []string{"book"})
	if err := s.UpsertConfusion(ctx, id1A, id1B, "zh_to_transl"); err != nil {
		t.Fatalf("UpsertConfusion: %v", err)
	}

	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	// User 1 should see their confusion.
	rec := do(t, r1, "GET", "/api/mismatches", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user1 mismatches: %d", rec.Code)
	}
	var items1 []models.ConfusionDetail
	decodeJSON(t, rec, &items1)
	if len(items1) != 1 {
		t.Errorf("user1 mismatches: want 1, got %d", len(items1))
	}

	// User 2 should see no confusions.
	rec = do(t, r2, "GET", "/api/mismatches", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 mismatches: %d", rec.Code)
	}
	var items2 []models.ConfusionDetail
	decodeJSON(t, rec, &items2)
	if len(items2) != 0 {
		t.Errorf("user2 mismatches: want 0, got %d", len(items2))
	}
}

// ── Export: Only own words ────────────────────────────────────────────────────

func TestIsolation_Export_OnlyOwnWords(t *testing.T) {
	s := openTestDB(t)

	// User 1 has a word; user 2 has a different word.
	seedWordForUser(t, s, 1, "再见", "zàijiàn", []string{"goodbye"})
	seedWordForUser(t, s, 2, "你好", "nǐ hǎo", []string{"hello"})

	r1 := newRouterForUser(s, 1)
	r2 := newRouterForUser(s, 2)

	// User 1's export should contain only "再见".
	rec := do(t, r1, "GET", "/api/words/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user1 export: %d %s", rec.Code, rec.Body)
	}
	body1 := rec.Body.String()
	if !strings.Contains(body1, "再见") {
		t.Error("user1 export: expected 再见")
	}
	if strings.Contains(body1, "你好") {
		t.Error("user1 export: should not contain user2's word 你好")
	}

	// User 2's export should contain only "你好".
	rec = do(t, r2, "GET", "/api/words/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 export: %d %s", rec.Code, rec.Body)
	}
	body2 := rec.Body.String()
	if !strings.Contains(body2, "你好") {
		t.Error("user2 export: expected 你好")
	}
	if strings.Contains(body2, "再见") {
		t.Error("user2 export: should not contain user1's word 再见")
	}
}
