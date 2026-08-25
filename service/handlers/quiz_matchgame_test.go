package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

// enableOnlyGameMode disables every match-game mode except keep, so a test can
// exercise one mode's fetch/repeat-avoidance logic in isolation despite all 4
// modes defaulting to enabled (issue #288).
func enableOnlyGameMode(t *testing.T, s *db.Store, keep string) {
	t.Helper()
	ctx := context.Background()
	st, err := s.GetUserSettings(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	st.GameModeMismatch = keep == "mismatch"
	st.GameModeNewest = keep == "newest"
	st.GameModeHardest = keep == "hardest"
	st.GameModeLastMistakes = keep == "last_mistakes"
	if err := s.UpdateUserSettings(ctx, int64(2), *st); err != nil {
		t.Fatal(err)
	}
}

func makeDifficultForTest(t *testing.T, s *db.Store, wordID int64, totalCorrect, totalAttempts int) {
	t.Helper()
	if _, err := s.ExecForTest(`UPDATE sm2_progress SET learning_new_word = 0, first_seen_at = datetime('now'),
		total_correct = ?, total_attempts = ?, last_attempt_at = datetime('now', '-2 hours')
		WHERE word_id = ?`, totalCorrect, totalAttempts, wordID); err != nil {
		t.Fatalf("makeDifficultForTest(%d): %v", wordID, err)
	}
}

func TestMatchGame_EmptyWhenFewerThan2Pairs(t *testing.T) {
	s := openTestDB(t)
	enableOnlyGameMode(t, s, "mismatch")
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
	enableOnlyGameMode(t, s, "mismatch")
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
	enableOnlyGameMode(t, s, "mismatch")
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
	enableOnlyGameMode(t, s, "mismatch")
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
	enableOnlyGameMode(t, s, "mismatch")
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

func TestMatchGame_OnlyHardestEnabled_ReturnsHardestCandidates(t *testing.T) {
	s := openTestDB(t)
	enableOnlyGameMode(t, s, "hardest")
	a := seedWord(t, s, "一", "", []string{"one"})
	b := seedWord(t, s, "二", "", []string{"two"})
	makeDifficultForTest(t, s, a, 1, 10)
	makeDifficultForTest(t, s, b, 2, 10)

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.MatchGameResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Words) != 2 {
		t.Fatalf("expected 2 hardest-mode candidates, got %d: %+v", len(resp.Words), resp.Words)
	}
	ids := map[int64]bool{}
	for _, w := range resp.Words {
		ids[w.ZhWordID] = true
	}
	if !ids[a] || !ids[b] {
		t.Errorf("expected words %d and %d, got %+v", a, b, resp.Words)
	}
}

func TestMatchGame_AllModesDisabled_ReturnsEmpty(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	st, err := s.GetUserSettings(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	st.GameModeMismatch = false
	st.GameModeNewest = false
	st.GameModeHardest = false
	st.GameModeLastMistakes = false
	if err := s.UpdateUserSettings(ctx, int64(2), *st); err != nil {
		t.Fatal(err)
	}

	a := seedWord(t, s, "一", "", []string{"one"})
	b := seedWord(t, s, "二", "", []string{"two"})
	makeDifficultForTest(t, s, a, 1, 10)
	makeDifficultForTest(t, s, b, 2, 10)

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.MatchGameResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Words) != 0 {
		t.Errorf("expected 0 words with all modes disabled, got %d: %+v", len(resp.Words), resp.Words)
	}
}

// TestMatchGame_DisabledModeNeverSelectedEvenWithCandidates covers issue #288
// decision #3: a disabled mode is never picked even when it has plenty of
// eligible candidates — only enabled modes are ever considered.
func TestMatchGame_DisabledModeNeverSelectedEvenWithCandidates(t *testing.T) {
	s := openTestDB(t)
	enableOnlyGameMode(t, s, "last_mistakes")
	// Hardest-mode candidates exist too, but hardest is disabled.
	a := seedWord(t, s, "一", "", []string{"one"})
	b := seedWord(t, s, "二", "", []string{"two"})
	makeDifficultForTest(t, s, a, 1, 10)
	makeDifficultForTest(t, s, b, 2, 10)

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.MatchGameResponse
	decodeJSON(t, rec, &resp)
	// last_mistakes is enabled but has no recorded mistakes, and hardest (which
	// does have candidates) is disabled — the game must not trigger at all.
	if len(resp.Words) != 0 {
		t.Errorf("expected 0 words (only a disabled mode has candidates), got %d: %+v", len(resp.Words), resp.Words)
	}
}

func TestMatchGame_OnlyNewestEnabled_ReturnsNewestCandidates(t *testing.T) {
	s := openTestDB(t)
	enableOnlyGameMode(t, s, "newest")
	seedWord(t, s, "一", "", []string{"one"})
	seedWord(t, s, "二", "", []string{"two"})

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.MatchGameResponse
	decodeJSON(t, rec, &resp)
	if len(resp.Words) != 2 {
		t.Fatalf("expected 2 newest-mode candidates, got %d: %+v", len(resp.Words), resp.Words)
	}
}

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
