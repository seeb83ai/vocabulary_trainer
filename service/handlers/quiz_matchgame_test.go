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

// TestMatchGame_NewestMode_MarksShownAndHidesUntilWrongAnswer covers issue
// #350: a word shown in newest mode must not reappear on the very next call
// (unlike before the fix, where the mode had no shown-bookkeeping at all),
// and only becomes eligible again after a wrong answer in normal training.
func TestMatchGame_NewestMode_MarksShownAndHidesUntilWrongAnswer(t *testing.T) {
	s := openTestDB(t)
	enableOnlyGameMode(t, s, "newest")
	a := seedWord(t, s, "买牛奶", "", []string{"buy milk"})
	b := seedWord(t, s, "喝水", "", []string{"drink water"})
	r := newRouter(s)

	// First call returns both words and marks them shown.
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	var resp1 models.MatchGameResponse
	decodeJSON(t, rec, &resp1)
	if len(resp1.Words) != 2 {
		t.Fatalf("first call: expected 2 words, got %d: %+v", len(resp1.Words), resp1.Words)
	}

	// Second call returns empty — both words are suppressed until a wrong answer.
	rec2 := do(t, r, "GET", "/api/quiz/match-game", nil)
	var resp2 models.MatchGameResponse
	decodeJSON(t, rec2, &resp2)
	if len(resp2.Words) != 0 {
		t.Errorf("second call: expected 0 words, got %d: %+v", len(resp2.Words), resp2.Words)
	}

	// Wrong answers on both words in normal training re-eligible them (the
	// mode needs at least matchGameMinCandidates=2 eligible words to trigger
	// at all, so a single re-eligible word alone would not be enough here).
	for _, id := range []int64{a, b} {
		if _, err := s.ExecForTest(`UPDATE sm2_progress SET last_wrong_at = datetime('now', '+1 minute') WHERE word_id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}

	rec3 := do(t, r, "GET", "/api/quiz/match-game", nil)
	var resp3 models.MatchGameResponse
	decodeJSON(t, rec3, &resp3)
	if len(resp3.Words) != 2 {
		t.Errorf("expected 2 words re-eligible after wrong answers, got %+v", resp3.Words)
	}
}

// TestMatchGame_HidesPinyinAtOrAboveDefaultThreshold covers issue #349: the
// default gamification_hide_pinyin_from_bucket ("70-84" / Practicing) flags
// HidePinyin for a word whose SM-2 bucket has reached Practicing, while
// leaving it unflagged for a word still below that bucket. The pinyin value
// itself is always sent (issue #375) — HidePinyin only tells the client
// whether to show it up front or wait for the word tile to be attempted.
func TestMatchGame_HidesPinyinAtOrAboveDefaultThreshold(t *testing.T) {
	s := openTestDB(t)
	enableOnlyGameMode(t, s, "newest")
	practicedID := seedWord(t, s, "会", "huì", []string{"can"})
	newID := seedWord(t, s, "去", "qù", []string{"go"})
	// 10 attempts, 80% accuracy → Practicing tier (>= default threshold).
	makeDifficultForTest(t, s, practicedID, 8, 10)
	_ = newID // left at 0 attempts → TierNone, below threshold, pinyin stays shown.

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
	for _, w := range resp.Words {
		if w.Pinyin == "" {
			t.Errorf("word %d should always have pinyin sent, got empty", w.ZhWordID)
		}
		switch w.ZhWordID {
		case practicedID:
			if !w.HidePinyin {
				t.Error("practicing-tier word should be flagged HidePinyin")
			}
		case newID:
			if w.HidePinyin {
				t.Error("below-threshold word should not be flagged HidePinyin")
			}
		}
	}
}

// TestMatchGame_PinyinHideThreshold_ConfigurableToMastered verifies raising
// the threshold to "85-100" (Mastered) leaves a Practicing-tier word
// unflagged — only Mastered-and-above should be flagged HidePinyin.
func TestMatchGame_PinyinHideThreshold_ConfigurableToMastered(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	enableOnlyGameMode(t, s, "newest")
	st, err := s.GetUserSettings(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	st.GamificationHidePinyinFromBucket = "85-100"
	if err := s.UpdateUserSettings(ctx, int64(2), *st); err != nil {
		t.Fatal(err)
	}
	practicedID := seedWord(t, s, "会", "huì", []string{"can"})
	makeDifficultForTest(t, s, practicedID, 8, 10) // Practicing tier, not Mastered

	r := newRouter(s)
	seedWord(t, s, "去", "qù", []string{"go"})
	rec := do(t, r, "GET", "/api/quiz/match-game", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp models.MatchGameResponse
	decodeJSON(t, rec, &resp)
	for _, w := range resp.Words {
		if w.ZhWordID == practicedID {
			if w.Pinyin == "" {
				t.Error("Practicing-tier word should keep pinyin value when threshold is set to Mastered")
			}
			if w.HidePinyin {
				t.Error("Practicing-tier word should not be flagged HidePinyin when threshold is set to Mastered")
			}
		}
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

// TestMatchAnswer_LearningNewWord_UsesLearningPhase guards against issue #398:
// a word still in the new-word introduction phase (learning_new_word=1)
// answered correctly via the match-game widget must go through the same
// learning-phase update as the main quiz (sm2.ProcessAnswer/UpdateLearning:
// due date minutes away, single-correct-answer streak) rather than the full
// graduated SM-2 algorithm (due date days away). Previously MatchAnswer called
// sm2.Update directly regardless of LearningNewWord, permanently stranding the
// word in the new bucket with a real SM2 due date it could never graduate out of.
func TestMatchAnswer_LearningNewWord_UsesLearningPhase(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	before, err := s.GetSM2Progress(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !before.LearningNewWord {
		t.Fatalf("expected freshly seeded word to be learning_new_word=1")
	}

	body := map[string]any{"zh_word_id": id, "correct": true}
	rec := do(t, r, "POST", "/api/quiz/match-answer", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	after, err := s.GetSM2Progress(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !after.LearningNewWord {
		t.Fatalf("expected word to remain in the new-word phase after a single correct answer (graduates at %d)", 3)
	}
	if time.Until(after.DueDate) > time.Hour {
		t.Errorf("expected a learning-phase due date (minutes away), got due_date=%v (%v from now)", after.DueDate, time.Until(after.DueDate))
	}
	if after.IntervalDays > 1 {
		t.Errorf("expected interval_days to stay at the learning-phase default, got %d", after.IntervalDays)
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
