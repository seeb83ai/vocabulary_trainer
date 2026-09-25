package handlers_test

import (
	"context"
	"fmt"
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
	id1 := seedWord(t, s, "一", "", []string{"one"})
	id2 := seedWord(t, s, "二", "", []string{"two"})
	markWordTrained(t, s, id1)
	markWordTrained(t, s, id2)

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
	markWordTrained(t, s, a)
	markWordTrained(t, s, b)
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
	markWordTrained(t, s, newID) // seen, but below threshold — pinyin stays shown.

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

// TestMatchAnswer_DoesNotRecordAnswerTimestamps guards against issue #449:
// sm2_progress.last_attempt_at/last_wrong_at exist purely to drive the
// "newest"/"last mistakes" match-game repeat-avoidance rules (see
// RecordAnswerTimestamps and GetLastMistakesForGame) and must only reflect
// real training answers (quiz.go's Answer handler). A match-game answer must
// still update SM-2 scheduling but must NOT touch these timestamps — bumping
// last_wrong_at from inside the match-game itself would immediately
// re-satisfy its own repeat-avoidance check and let the word reappear in the
// very next game round without ever going through regular training.
func TestMatchAnswer_DoesNotRecordAnswerTimestamps(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "根据", "gēnjù", []string{"according to"})
	// Give the word a prior real training wrong answer, as it would have to
	// qualify for the "last mistakes" game mode in the first place.
	const priorWrongAt = "2026-01-01 10:00:00"
	if _, err := s.ExecForTest(`UPDATE sm2_progress SET last_attempt_at = ?, last_wrong_at = ? WHERE word_id = ?`,
		priorWrongAt, priorWrongAt, id); err != nil {
		t.Fatal(err)
	}

	r := newRouter(s)
	body := map[string]any{"zh_word_id": id, "correct": false}
	rec := do(t, r, "POST", "/api/quiz/match-answer", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	attemptAt, wrongAt, err := s.GetAnswerTimestampsForTest(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if attemptAt.String != priorWrongAt {
		t.Errorf("expected last_attempt_at to stay at %q (a match-game answer must not update it), got %q", priorWrongAt, attemptAt.String)
	}
	if wrongAt.String != priorWrongAt {
		t.Errorf("expected last_wrong_at to stay at %q (a match-game answer must not update it), got %q", priorWrongAt, wrongAt.String)
	}
}

// TestMatchGame_LastMistakes_MatchGameAnswerDoesNotReQualify reproduces the
// maintainer's exact clarified scenario for issue #449 end-to-end through
// the real training and match-game HTTP endpoints:
//
//  1. word trained wrong (real training)        -> due for the game
//  2. shown in the last-mistakes game            -> now suppressed
//  3. answered inside the match-game (right or wrong) -> must STAY suppressed
//     (a match-game answer must never re-qualify the word on its own)
//  4. answered wrong again in real training       -> re-qualified
//  5. shown in the game again                     -> confirmed re-eligible
func TestMatchGame_LastMistakes_MatchGameAnswerDoesNotReQualify(t *testing.T) {
	s := openTestDB(t)
	enableOnlyGameMode(t, s, "last_mistakes")
	// 'a' is the word under test. 'p' is a padding candidate that's kept
	// independently re-qualified via real training wrong answers, purely so
	// the mode has matchGameMinCandidates=2 eligible words and actually
	// triggers at each step — the assertions below only ever check whether
	// 'a' specifically is present.
	a := seedWord(t, s, "根据", "gēnjù", []string{"according to"})
	p := seedWord(t, s, "喝水", "hē shuǐ", []string{"drink water"})
	markWordTrained(t, s, a)
	markWordTrained(t, s, p)
	r := newRouter(s)

	wrongAnswer := func(wordID int64, wrongText string) {
		t.Helper()
		rec := do(t, r, "POST", "/api/quiz/answer", map[string]any{
			"word_id": wordID,
			"mode":    "zh_to_transl",
			"answer":  wrongText,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("answer word %d: want 200, got %d: %s", wordID, rec.Code, rec.Body.String())
		}
	}
	containsWord := func(resp models.MatchGameResponse, wordID int64) bool {
		for _, w := range resp.Words {
			if w.ZhWordID == wordID {
				return true
			}
		}
		return false
	}
	// backdateShown pushes every word_game_shown row for this game mode an
	// hour further into the past. All the timestamps this test cares about
	// (last_wrong_at, last_shown_in_game) share second-granularity columns,
	// so successive real events within the same test run could otherwise tie
	// instead of compare strictly greater. A full hour of separation between
	// "shown" and the next real event removes that flakiness without
	// weakening what's under test.
	backdateShown := func() {
		t.Helper()
		if _, err := s.ExecForTest(`UPDATE word_game_shown SET last_shown_in_game = datetime(last_shown_in_game, '-1 hour') WHERE game_mode = 'last_mistakes'`); err != nil {
			t.Fatal(err)
		}
	}

	// Step 1: both words trained wrong via the real training answer path.
	// Backdate the resulting last_wrong_at by 2 hours so it's unambiguously
	// earlier than the "shown" timestamp round 1 is about to stamp (avoids
	// same-second ties between successive real events below).
	wrongAnswer(a, "definitely wrong")
	wrongAnswer(p, "definitely wrong")
	if _, err := s.ExecForTest(`UPDATE sm2_progress SET last_wrong_at = datetime(last_wrong_at, '-2 hours') WHERE word_id IN (?, ?)`, a, p); err != nil {
		t.Fatal(err)
	}

	// Step 2: both are due for the last-mistakes game and get shown.
	rec1 := do(t, r, "GET", "/api/quiz/match-game", nil)
	var resp1 models.MatchGameResponse
	decodeJSON(t, rec1, &resp1)
	if !containsWord(resp1, a) || !containsWord(resp1, p) {
		t.Fatalf("expected both words shown, got %+v", resp1.Words)
	}
	backdateShown()

	// Step 3: answer 'a' wrong inside the match-game itself. This must not
	// re-qualify 'a' for the game on its own.
	rec2 := do(t, r, "POST", "/api/quiz/match-answer", map[string]any{"zh_word_id": a, "correct": false})
	if rec2.Code != http.StatusOK {
		t.Fatalf("match-answer a: want 200, got %d: %s", rec2.Code, rec2.Body.String())
	}

	// Re-qualify 'p' via a real training wrong answer, so the mode has >= 2
	// candidates and actually returns a round for this check.
	wrongAnswer(p, "definitely wrong again")

	// Step 3 (confirmation): 'a' must still be suppressed — a match-game
	// wrong answer must not resurface it.
	rec3 := do(t, r, "GET", "/api/quiz/match-game", nil)
	var resp3 models.MatchGameResponse
	decodeJSON(t, rec3, &resp3)
	if containsWord(resp3, a) {
		t.Errorf("word %d reappeared in the game after only a match-game answer (no real training wrong answer since it was last shown) — got %+v", a, resp3.Words)
	}
	backdateShown()

	// Step 4: word 'a' gets a NEW real training wrong answer.
	wrongAnswer(a, "still wrong")
	// Keep 'p' eligible too so the mode still has >= 2 candidates.
	wrongAnswer(p, "definitely wrong once more")

	// Step 5: 'a' is eligible again now that it has a real training wrong
	// answer since it was last shown in the game.
	rec4 := do(t, r, "GET", "/api/quiz/match-game", nil)
	var resp4 models.MatchGameResponse
	decodeJSON(t, rec4, &resp4)
	if !containsWord(resp4, a) {
		t.Errorf("expected word %d re-eligible after a fresh real training wrong answer, got %+v", a, resp4.Words)
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

// setMatchGameSM2Update patches the match_game_sm2_update setting (issue #472).
func setMatchGameSM2Update(t *testing.T, r http.Handler, value string) {
	t.Helper()
	body := baseSettingsPatch()
	body["match_game_sm2_update"] = value
	if rec := do(t, r, "PATCH", "/api/settings", body); rec.Code != http.StatusOK {
		t.Fatalf("patch settings: %d %s", rec.Code, rec.Body.String())
	}
}

// TestMatchAnswer_SM2UpdateSetting covers issue #472: the
// match_game_sm2_update setting decides whether a match-game answer changes
// progress — for word tiles and component tiles alike.
func TestMatchAnswer_SM2UpdateSetting(t *testing.T) {
	cases := []struct {
		setting     string
		correct     bool
		wantUpdated bool
	}{
		{"never", true, false},
		{"never", false, false},
		{"wrong_only", true, false},
		{"wrong_only", false, true},
		{"always", true, true},
		{"always", false, true},
	}
	for _, c := range cases {
		name := fmt.Sprintf("%s/correct=%v", c.setting, c.correct)
		t.Run("word/"+name, func(t *testing.T) {
			s := openTestDB(t)
			ctx := context.Background()
			id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
			r := newRouter(s)
			setMatchGameSM2Update(t, r, c.setting)

			rec := do(t, r, "POST", "/api/quiz/match-answer", map[string]any{"zh_word_id": id, "correct": c.correct})
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			progress, err := s.GetSM2Progress(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if updated := progress.TotalAttempts == 1; updated != c.wantUpdated {
				t.Errorf("progress updated = %v (attempts=%d), want %v", updated, progress.TotalAttempts, c.wantUpdated)
			}
		})
		t.Run("component/"+name, func(t *testing.T) {
			s := openTestDB(t)
			ctx := context.Background()
			s.InsertComponentProgressForTest(ctx, int64(2), "扑", time.Now())
			r := newRouter(s)
			setMatchGameSM2Update(t, r, c.setting)

			rec := do(t, r, "POST", "/api/quiz/match-answer", map[string]any{"kind": "component", "character": "扑", "correct": c.correct})
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			progress, err := s.GetComponentProgress(ctx, int64(2), "扑")
			if err != nil {
				t.Fatal(err)
			}
			if updated := progress.TotalAttempts == 1; updated != c.wantUpdated {
				t.Errorf("component progress updated = %v (attempts=%d), want %v", updated, progress.TotalAttempts, c.wantUpdated)
			}
		})
	}
}
