package db

import (
	"context"
	"testing"
	"time"

	"vocabulary_trainer/models"
)

// markReviewed sets total_attempts/total_correct/due_date/first_seen_at on an
// existing word's sm2_progress row, simulating a word that has already been
// quizzed at least once.
func markReviewed(t *testing.T, s *Store, wordID int64, totalCorrect, totalAttempts int, due time.Time) {
	t.Helper()
	_, err := s.db.ExecContext(context.Background(),
		`UPDATE sm2_progress SET total_attempts = ?, total_correct = ?, due_date = ?, first_seen_at = date('now'), learning_new_word = 0
		 WHERE word_id = ?`,
		totalAttempts, totalCorrect, due.UTC().Format("2006-01-02 15:04:05"), wordID)
	if err != nil {
		t.Fatalf("markReviewed word %d: %v", wordID, err)
	}
}

// ── segmentSentence ──────────────────────────────────────────────────────────

func TestSegmentSentence_FullCoverage(t *testing.T) {
	known := map[string]int64{"我": 1, "买": 2, "牛奶": 3}
	segs, ok := segmentSentence("我买牛奶", known)
	if !ok {
		t.Fatal("expected fullyCovered=true")
	}
	want := []wordSegment{{Text: "我", WordID: 1}, {Text: "买", WordID: 2}, {Text: "牛奶", WordID: 3}}
	if len(segs) != len(want) {
		t.Fatalf("want %d segments, got %d: %+v", len(want), len(segs), segs)
	}
	for i, w := range want {
		if segs[i] != w {
			t.Errorf("segment %d: want %+v, got %+v", i, w, segs[i])
		}
	}
}

func TestSegmentSentence_LongestMatchPreferred(t *testing.T) {
	known := map[string]int64{"我": 1, "们": 2, "我们": 3}
	segs, ok := segmentSentence("我们", known)
	if !ok {
		t.Fatal("expected fullyCovered=true")
	}
	if len(segs) != 1 || segs[0] != (wordSegment{Text: "我们", WordID: 3}) {
		t.Errorf("expected single longest-match segment 我们, got %+v", segs)
	}
}

func TestSegmentSentence_GapNotCovered(t *testing.T) {
	known := map[string]int64{"我": 1, "买": 2} // missing 牛奶
	segs, ok := segmentSentence("我买牛奶", known)
	if ok {
		t.Fatalf("expected fullyCovered=false, got true with segments %+v", segs)
	}
}

func TestSegmentSentence_EmptyKnown(t *testing.T) {
	if _, ok := segmentSentence("我", map[string]int64{}); ok {
		t.Error("expected fullyCovered=false when known is empty")
	}
	if _, ok := segmentSentence("", map[string]int64{}); !ok {
		t.Error("expected fullyCovered=true for empty text regardless of known")
	}
}

// ── locateTranslationBlank ───────────────────────────────────────────────────

func TestLocateTranslationBlank_Found(t *testing.T) {
	blanked, ok := locateTranslationBlank("I buy milk every day.", []string{"milk"})
	if !ok {
		t.Fatal("expected match")
	}
	if blanked != "I buy ___ every day." {
		t.Errorf("got %q", blanked)
	}
}

func TestLocateTranslationBlank_CaseInsensitive(t *testing.T) {
	blanked, ok := locateTranslationBlank("I Buy Milk every day.", []string{"milk"})
	if !ok || blanked != "I Buy ___ every day." {
		t.Errorf("got %q, ok=%v", blanked, ok)
	}
}

func TestLocateTranslationBlank_NotFound(t *testing.T) {
	_, ok := locateTranslationBlank("I bought milk.", []string{"buy"})
	if ok {
		t.Error("expected no match for conjugated form not present verbatim")
	}
}

// ── findSentenceBlank ─────────────────────────────────────────────────────────

func seedSentenceScenario(t *testing.T, s *Store) (wo, mai, niunai, sentence int64) {
	t.Helper()
	wo = seedWord(t, s, "我", "wǒ", []string{"I"})
	mai = seedWord(t, s, "买", "mǎi", []string{"buy"})
	niunai = seedWord(t, s, "牛奶", "niú nǎi", []string{"milk"})
	sentence = seedWordWithTags(t, s, "我买牛奶", "wǒ mǎi niú nǎi", []string{"I buy milk"}, []string{"s_test"})
	return
}

func TestFindSentenceBlank_ReturnsEarliestDueWordInEligibleSentence(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	wo, mai, niunai, sentence := seedSentenceScenario(t, s)

	now := time.Now().UTC()
	markReviewed(t, s, wo, 1, 1, now.Add(2*time.Hour))
	markReviewed(t, s, mai, 1, 1, now.Add(-1*time.Hour)) // earliest due
	markReviewed(t, s, niunai, 1, 1, now.Add(3*time.Hour))

	match, err := s.findSentenceBlank(ctx, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if match == nil {
		t.Fatal("expected a match")
	}
	if match.TargetWordID != mai {
		t.Errorf("want target word %d (买), got %d", mai, match.TargetWordID)
	}
	if match.SentenceID != sentence {
		t.Errorf("want sentence %d, got %d", sentence, match.SentenceID)
	}
	if match.TargetText != "买" {
		t.Errorf("want target text 买, got %q", match.TargetText)
	}
}

func TestFindSentenceBlank_NoDueWords_ReturnsNil(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	wo, mai, niunai, _ := seedSentenceScenario(t, s)

	future := time.Now().UTC().Add(48 * time.Hour)
	markReviewed(t, s, wo, 1, 1, future)
	markReviewed(t, s, mai, 1, 1, future)
	markReviewed(t, s, niunai, 1, 1, future)

	match, err := s.findSentenceBlank(ctx, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if match != nil {
		t.Errorf("expected nil match when nothing is due, got %+v", match)
	}
}

func TestFindSentenceBlank_SentenceNotFullyCovered_NotEligible(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	wo, mai, _, _ := seedSentenceScenario(t, s)
	// 牛奶 (niunai) deliberately left unreviewed (total_attempts=0) — the
	// sentence 我买牛奶 can't be fully segmented from the known-word set, so
	// it must not be eligible even though 我 and 买 are due.
	now := time.Now().UTC()
	markReviewed(t, s, wo, 1, 1, now.Add(-2*time.Hour))
	markReviewed(t, s, mai, 1, 1, now.Add(-1*time.Hour))

	match, err := s.findSentenceBlank(ctx, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if match != nil {
		t.Errorf("expected nil match when the sentence isn't fully covered, got %+v", match)
	}
}

// ── NextSentenceBlankCard ────────────────────────────────────────────────────

func TestNextSentenceBlankCard_ZhBlankDirection(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	wo, mai, niunai, _ := seedSentenceScenario(t, s)
	now := time.Now().UTC()
	markReviewed(t, s, wo, 1, 1, now.Add(1*time.Hour))
	// totalAttempts=1 (<3) → SelectProgressiveMode returns cfg.New → default transl_to_zh.
	markReviewed(t, s, mai, 0, 1, now.Add(-1*time.Hour))
	markReviewed(t, s, niunai, 1, 1, now.Add(2*time.Hour))

	cfg := defaultProgCfg()
	nwCfg := defaultNWCfg()
	card, err := s.NextSentenceBlankCard(ctx, testUserID, cfg, nwCfg, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if card == nil {
		t.Fatal("expected a card")
	}
	if card.CardType != "sentence" {
		t.Errorf("want card_type=sentence, got %q", card.CardType)
	}
	if card.WordID != mai {
		t.Errorf("want word_id=%d, got %d", mai, card.WordID)
	}
	if card.Mode != models.ModeTranslToZh {
		t.Errorf("want mode=transl_to_zh, got %q", card.Mode)
	}
	if card.SentenceBlank != "我___牛奶" {
		t.Errorf("want blanked sentence 我___牛奶, got %q", card.SentenceBlank)
	}
	if card.SentenceContext != "I buy milk" {
		t.Errorf("want context %q, got %q", "I buy milk", card.SentenceContext)
	}
}

func TestNextSentenceBlankCard_TranslationBlankDirection(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	wo, mai, niunai, _ := seedSentenceScenario(t, s)
	now := time.Now().UTC()
	markReviewed(t, s, wo, 1, 1, now.Add(1*time.Hour))
	markReviewed(t, s, mai, 1, 1, now.Add(2*time.Hour))
	// High accuracy, >=10 attempts → Mastered tier → cfg.Mastered. Earliest
	// due date of the three, so it's the target word.
	markReviewed(t, s, niunai, 10, 10, now.Add(-1*time.Hour))

	cfg := defaultProgCfg()
	cfg.Mastered = models.ModeZhToTransl // pin instead of "random" for a deterministic test
	nwCfg := defaultNWCfg()
	card, err := s.NextSentenceBlankCard(ctx, testUserID, cfg, nwCfg, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if card == nil {
		t.Fatal("expected a card")
	}
	if card.WordID != niunai {
		t.Errorf("want word_id=%d (牛奶), got %d", niunai, card.WordID)
	}
	if card.Mode != models.ModeZhToTransl {
		t.Errorf("want mode=zh_to_transl, got %q", card.Mode)
	}
	if card.SentenceBlank != "I buy ___" {
		t.Errorf("want blanked translation 'I buy ___', got %q", card.SentenceBlank)
	}
	if card.SentenceContext != "我买牛奶" {
		t.Errorf("want context 我买牛奶, got %q", card.SentenceContext)
	}
}

func TestNextSentenceBlankCard_TranslationUnsupported_FallsBackToZhBlank(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	// Sentence translation uses a conjugated form ("bought") that never
	// appears verbatim in the word's own translation ("buy"), so direction
	// (b) has no substring match and must fall back to direction (a).
	wo := seedWord(t, s, "我", "wǒ", []string{"I"})
	mai := seedWord(t, s, "买", "mǎi", []string{"buy"})
	niunai := seedWord(t, s, "牛奶", "niú nǎi", []string{"milk"})
	seedWordWithTags(t, s, "我买牛奶", "wǒ mǎi niú nǎi", []string{"I bought milk"}, []string{"s_test"})

	now := time.Now().UTC()
	markReviewed(t, s, wo, 1, 1, now.Add(1*time.Hour))
	markReviewed(t, s, mai, 10, 10, now.Add(-1*time.Hour))
	markReviewed(t, s, niunai, 1, 1, now.Add(2*time.Hour))

	cfg := defaultProgCfg()
	cfg.Mastered = models.ModeZhToTransl
	nwCfg := defaultNWCfg()
	card, err := s.NextSentenceBlankCard(ctx, testUserID, cfg, nwCfg, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if card == nil {
		t.Fatal("expected a card")
	}
	if card.WordID != mai {
		t.Errorf("want word_id=%d (买), got %d", mai, card.WordID)
	}
	if card.Mode != models.ModeTranslToZh {
		t.Errorf("want fallback mode=transl_to_zh, got %q", card.Mode)
	}
	if card.SentenceBlank != "我___牛奶" {
		t.Errorf("want blanked sentence 我___牛奶, got %q", card.SentenceBlank)
	}
}

func TestNextSentenceBlankCard_NoEligibleSentence_ReturnsNil(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "水", "shuǐ", []string{"water"})
	markReviewed(t, s, id, 1, 1, time.Now().UTC().Add(-1*time.Hour))

	card, err := s.NextSentenceBlankCard(ctx, testUserID, defaultProgCfg(), defaultNWCfg(), []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if card != nil {
		t.Errorf("expected nil card when no sentence exists, got %+v", card)
	}
}

func defaultProgCfg() models.ProgressiveModeConfig {
	return models.ProgressiveModeConfig{
		New:        models.ModeTranslToZh,
		Struggling: models.ModeTranslToZh,
		Learning:   models.ModeZhPinyinToTransl,
		Practicing: models.ModeZhToTransl,
		Mastered:   models.ModeZhToTransl,
	}
}

func defaultNWCfg() models.NewWordModeConfig {
	return models.NewWordModeConfig{
		Step0: models.ModeTranslToZh,
		Step1: models.ModeTranslToZh,
		Step2: models.ModeZhToTransl,
	}
}

// TestSegmentSentence_InternalPunctuationIsIgnored reproduces the production
// symptom from issue #351: real-world multi-clause sentences almost always
// contain internal punctuation (commas, colons, quotation marks — not just
// the trailing full stop stripSentencePunctuation already handles). Before
// the fix, segmentSentence had no notion of punctuation at all, so a single
// internal comma between two otherwise fully-known clauses made the whole
// sentence unsegmentable forever, regardless of how complete the known-word
// set was — silently starving the feature of any eligible sentence on a
// realistic vocabulary. Punctuation characters must be skipped (not counted
// as blank-worthy "words") rather than breaking coverage.
func TestSegmentSentence_InternalPunctuationIsIgnored(t *testing.T) {
	known := map[string]int64{"我": 1, "买": 2, "牛奶": 3, "你": 4, "喜欢": 5, "茶": 6}
	// "I buy milk, you like tea" — every real word is known; only the comma
	// between the two clauses is not itself a "known word".
	segs, ok := segmentSentence("我买牛奶，你喜欢茶", known)
	if !ok {
		t.Fatalf("expected fullyCovered=true once internal punctuation is skipped, got segments %+v", segs)
	}
	want := []wordSegment{
		{Text: "我", WordID: 1}, {Text: "买", WordID: 2}, {Text: "牛奶", WordID: 3},
		{Text: "你", WordID: 4}, {Text: "喜欢", WordID: 5}, {Text: "茶", WordID: 6},
	}
	if len(segs) != len(want) {
		t.Fatalf("want %d segments, got %d: %+v", len(want), len(segs), segs)
	}
	for i, w := range want {
		if segs[i] != w {
			t.Errorf("segment %d: want %+v, got %+v", i, w, segs[i])
		}
	}
}

// TestSegmentSentence_UnknownNonPunctuationStillBreaksCoverage guards against
// an overly-broad fix: a genuinely unknown word/character (not punctuation)
// must still make the sentence ineligible.
func TestSegmentSentence_UnknownNonPunctuationStillBreaksCoverage(t *testing.T) {
	known := map[string]int64{"我": 1, "买": 2} // 牛奶 deliberately missing
	segs, ok := segmentSentence("我买牛奶，你好", known)
	if ok {
		t.Fatalf("expected fullyCovered=false for a genuinely unknown word, got segments %+v", segs)
	}
}
