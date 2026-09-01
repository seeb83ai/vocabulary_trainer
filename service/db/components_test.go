package db

import (
	"context"
	"strings"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

// ── shouldKeepComponent unit tests ──────────────────────────────────────────

func TestShouldKeep_PictophoneticPhoneticOnly_Dropped(t *testing.T) {
	ety := `{"type":"pictophonetic","phonetic":"马","semantic":"女","hint":"horse"}`
	if shouldKeepComponent('妈', '马', ety, "女", nil, nil) {
		t.Errorf("want false: 马 is the phonetic-only component of 妈")
	}
}

func TestShouldKeep_PictophoneticSemantic_Kept(t *testing.T) {
	ety := `{"type":"pictophonetic","phonetic":"马","semantic":"女","hint":"horse"}`
	if !shouldKeepComponent('妈', '女', ety, "女", nil, nil) {
		t.Errorf("want true: 女 is the semantic component of 妈")
	}
}

func TestShouldKeep_PhoneticEqualsSemantic_Kept(t *testing.T) {
	ety := `{"type":"pictophonetic","phonetic":"X","semantic":"X"}`
	if !shouldKeepComponent('Y', 'X', ety, "X", nil, nil) {
		t.Errorf("want true: component labelled as both phonetic and semantic")
	}
}

func TestShouldKeep_PhoneticEqualsRadical_Kept(t *testing.T) {
	ety := `{"type":"pictophonetic","phonetic":"马","semantic":"女"}`
	if !shouldKeepComponent('妈', '马', ety, "马", nil, nil) {
		t.Errorf("want true: phonetic component equals the radical")
	}
}

func TestShouldKeep_Ideographic_AllKept(t *testing.T) {
	ety := `{"type":"ideographic","hint":"sun and moon = bright"}`
	if !shouldKeepComponent('明', '日', ety, "日", nil, nil) {
		t.Errorf("want true: 日 kept for ideographic 明")
	}
	if !shouldKeepComponent('明', '月', ety, "日", nil, nil) {
		t.Errorf("want true: 月 kept for ideographic 明")
	}
}

func TestShouldKeep_Pictographic_AllKept(t *testing.T) {
	ety := `{"type":"pictographic","hint":"picture of a tree"}`
	if !shouldKeepComponent('木', 'X', ety, "木", nil, nil) {
		t.Errorf("want true: components of pictographic chars are kept")
	}
}

func TestShouldKeep_NoEtymology_PinyinSimilar_Dropped(t *testing.T) {
	if shouldKeepComponent('请', '青', "", "讠", []string{"qǐng"}, []string{"qīng"}) {
		t.Errorf("want false: 青 (qīng) shares final with 请 (qǐng), pinyin fallback should drop")
	}
}

func TestShouldKeep_NoEtymology_PinyinDifferent_Kept(t *testing.T) {
	if !shouldKeepComponent('好', '女', "", "女", []string{"hǎo"}, []string{"nǚ"}) {
		t.Errorf("want true: 女 (nü) does not share final with 好 (hao)")
	}
}

func TestShouldKeep_NoEtymology_PinyinMissing_Kept(t *testing.T) {
	if !shouldKeepComponent('请', '青', "", "讠", nil, nil) {
		t.Errorf("want true: no etymology and no pinyin → keep (conservative)")
	}
}

func TestShouldKeep_MalformedEtymology_FallsBackToPinyin(t *testing.T) {
	if shouldKeepComponent('请', '青', "{not json", "讠", []string{"qǐng"}, []string{"qīng"}) {
		t.Errorf("want false: malformed etymology should fall back to pinyin, which drops")
	}
}

func TestShouldKeep_SelfReference_Dropped(t *testing.T) {
	if shouldKeepComponent('好', '好', "", "", nil, nil) {
		t.Errorf("want false: self-reference never kept")
	}
}

// ── pinyinSimilar unit tests ────────────────────────────────────────────────

func TestPinyinSimilar_ToneStripped(t *testing.T) {
	if !pinyinSimilar([]string{"qǐng"}, []string{"qīng"}) {
		t.Errorf("want true: qǐng and qīng share final ing (after tone strip)")
	}
}

func TestPinyinSimilar_DifferentFinal(t *testing.T) {
	if pinyinSimilar([]string{"mā"}, []string{"fēng"}) {
		t.Errorf("want false: ma and feng have different finals")
	}
}

func TestPinyinSimilar_MultipleReadings_AnyMatch(t *testing.T) {
	if !pinyinSimilar([]string{"háng", "xíng"}, []string{"xīng"}) {
		t.Errorf("want true: xíng and xīng share final ing")
	}
}

func TestPinyinSimilar_EitherEmpty_False(t *testing.T) {
	if pinyinSimilar(nil, []string{"xīng"}) {
		t.Errorf("want false: empty parent pinyin")
	}
	if pinyinSimilar([]string{"xīng"}, nil) {
		t.Errorf("want false: empty comp pinyin")
	}
	if pinyinSimilar(nil, nil) {
		t.Errorf("want false: both empty")
	}
}

func TestPinyinSimilar_ToneDigitsStripped(t *testing.T) {
	if !pinyinSimilar([]string{"qing3"}, []string{"qing1"}) {
		t.Errorf("want true: tone digits should be stripped")
	}
}

// ── InitComponentsForWord integration tests (etymology-aware) ───────────────

// seedHanziFull inserts a complete hanzi_decomposition row with etymology,
// radical and pinyin.
func seedHanziFull(t *testing.T, s *Store, character, definition, decomp, etymology, radical, pinyinJSON string) {
	t.Helper()
	_, err := s.db.Exec(
		`INSERT INTO hanzi_decomposition (character, definition, decomposition, etymology, radical, pinyin)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(character) DO UPDATE SET
		   definition = excluded.definition,
		   decomposition = excluded.decomposition,
		   etymology = excluded.etymology,
		   radical = excluded.radical,
		   pinyin = excluded.pinyin`,
		character,
		nullIfEmpty(definition), nullIfEmpty(decomp), nullIfEmpty(etymology),
		nullIfEmpty(radical), nullIfEmpty(pinyinJSON),
	)
	if err != nil {
		t.Fatalf("seedHanziFull %q: %v", character, err)
	}
	if definition != "" {
		_, err = s.db.Exec(
			`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, 'EN', ?)
			 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
			character, definition)
		if err != nil {
			t.Fatalf("seedHanziFull translation %q: %v", character, err)
		}
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func TestInitComponentsForWord_ExcludesPhoneticByEtymology(t *testing.T) {
	s := openTestDB(t)
	// 妈 = 女 (semantic) + 马 (phonetic). 马 must NOT be inserted.
	ety := `{"type":"pictophonetic","phonetic":"马","semantic":"女","hint":"mother"}`
	seedHanziFull(t, s, "妈", "mother", "⿰女马", ety, "女", "")
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziDef(t, s, "马", "horse")

	if err := s.InitComponentsForWord(context.Background(), int64(2), "妈", time.Now()); err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}

	var chars []string
	rows, err := s.db.Query(`SELECT character FROM component_progress WHERE user_id = 2 ORDER BY character`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatalf("scan: %v", err)
		}
		chars = append(chars, c)
	}
	rows.Close()

	if len(chars) != 1 || chars[0] != "女" {
		t.Errorf("want [女], got %v (phonetic 马 must be excluded)", chars)
	}
}

func TestInitComponentsForWord_KeepsAllForIdeographic(t *testing.T) {
	s := openTestDB(t)
	ety := `{"type":"ideographic","hint":"sun + moon"}`
	seedHanziFull(t, s, "明", "bright", "⿰日月", ety, "日", "")
	seedHanziDef(t, s, "日", "sun; day")
	seedHanziDef(t, s, "月", "moon; month")

	if err := s.InitComponentsForWord(context.Background(), int64(2), "明", time.Now()); err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}

	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM component_progress WHERE user_id = 2`).Scan(&count)
	if count != 2 {
		t.Errorf("want 2 components for ideographic 明, got %d", count)
	}
}

func TestInitComponentsForWord_PinyinFallbackDrop(t *testing.T) {
	s := openTestDB(t)
	// Parent with no etymology. 青 (qīng) shares final with parent 请 (qǐng)
	// → should be dropped via pinyin fallback. 讠 has different pinyin → kept.
	seedHanziFull(t, s, "请", "request; please", "⿰讠青", "", "讠", `["qǐng"]`)
	seedHanziFull(t, s, "青", "blue/green", "", "", "", `["qīng"]`)
	seedHanziFull(t, s, "讠", "speech radical", "", "", "", `["yán"]`)

	if err := s.InitComponentsForWord(context.Background(), int64(2), "请", time.Now()); err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}

	var chars []string
	rows, err := s.db.Query(`SELECT character FROM component_progress WHERE user_id = 2 ORDER BY character`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatalf("scan: %v", err)
		}
		chars = append(chars, c)
	}
	rows.Close()

	if len(chars) != 1 || chars[0] != "讠" {
		t.Errorf("want [讠], got %v (pinyin-similar 青 must be excluded)", chars)
	}
}

func TestGetNextComponentCard_IncludesPinyin(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "女", "woman", "", "", "", `["nǚ"]`)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))

	card, err := s.GetNextComponentCard(ctx, int64(2), []string{"en"})
	if err != nil {
		t.Fatalf("GetNextComponentCard: %v", err)
	}
	if card == nil {
		t.Fatal("want a card, got nil")
	}
	if card.Pinyin != "nǚ" {
		t.Errorf("want pinyin %q, got %q", "nǚ", card.Pinyin)
	}
}

func TestGetNextComponentCard_MultipleReadingsJoined(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "行", "row/walk", "", "", "", `["háng","xíng"]`)
	s.InsertComponentProgressForTest(ctx, int64(2), "行", time.Now().Add(-time.Hour))

	card, err := s.GetNextComponentCard(ctx, int64(2), []string{"en"})
	if err != nil {
		t.Fatalf("GetNextComponentCard: %v", err)
	}
	if card == nil {
		t.Fatal("want a card, got nil")
	}
	if card.Pinyin != "háng / xíng" {
		t.Errorf("want pinyin %q, got %q", "háng / xíng", card.Pinyin)
	}
}

// TestGetNextComponentCard_WrongAnswerNotImmediatelyRepeated mirrors the word
// quiz behaviour (see GetNextCard's "due_date <= CURRENT_TIMESTAMP" preference
// in words.go): after a wrong answer, RecordComponentAnswer pushes the due
// date a few minutes into the future (sm2.WrongRetryDelay), but
// GetNextComponentCard's due-date filter (`< date('now', '+1 day')`) still
// matched it because it compares against midnight tomorrow, not "now" — so a
// component with no other due sibling was served again on the very next call
// (#391) instead of waiting out its own retry delay like a word would.
func TestGetNextComponentCard_WrongAnswerNotImmediatelyRepeated(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "女", "woman", "", "", "", `["nǚ"]`)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(-time.Hour))

	if _, _, err := s.RecordComponentAnswer(ctx, int64(2), "女", false); err != nil {
		t.Fatalf("RecordComponentAnswer: %v", err)
	}

	card, err := s.GetNextComponentCard(ctx, int64(2), []string{"en"})
	if err != nil {
		t.Fatalf("GetNextComponentCard: %v", err)
	}
	if card != nil {
		t.Errorf("want nil (component's retry delay has not elapsed yet), got card for %q — wrong answer was repeated immediately", card.Character)
	}
}

// ── Component HMM scene tests ────────────────────────────────────────────────

func TestUpsertAndGetComponentHMMScene(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	const userID = int64(2)
	const char = "木"

	text, err := s.GetComponentHMMSceneText(ctx, userID, char)
	if err != nil {
		t.Fatalf("GetComponentHMMSceneText (missing): %v", err)
	}
	if text != "" {
		t.Errorf("want empty string for missing scene, got %q", text)
	}

	if err := s.UpsertComponentHMMScene(ctx, userID, char, "A tree in the park"); err != nil {
		t.Fatalf("UpsertComponentHMMScene: %v", err)
	}

	text, err = s.GetComponentHMMSceneText(ctx, userID, char)
	if err != nil {
		t.Fatalf("GetComponentHMMSceneText: %v", err)
	}
	if text != "A tree in the park" {
		t.Errorf("want %q, got %q", "A tree in the park", text)
	}
}

func TestUpsertComponentHMMScene_Overwrites(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	const userID = int64(2)
	const char = "水"

	if err := s.UpsertComponentHMMScene(ctx, userID, char, "First scene"); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := s.UpsertComponentHMMScene(ctx, userID, char, "Second scene"); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	text, err := s.GetComponentHMMSceneText(ctx, userID, char)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if text != "Second scene" {
		t.Errorf("want %q, got %q", "Second scene", text)
	}
}

func TestDeleteComponentHMMScene(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	const userID = int64(2)
	const char = "火"

	if err := s.UpsertComponentHMMScene(ctx, userID, char, "Fire scene"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.DeleteComponentHMMScene(ctx, userID, char); err != nil {
		t.Fatalf("delete: %v", err)
	}

	text, err := s.GetComponentHMMSceneText(ctx, userID, char)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if text != "" {
		t.Errorf("want empty after delete, got %q", text)
	}
}

func TestComponentHMMScene_IsolatedPerUser(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	const char = "土"

	if err := s.UpsertComponentHMMScene(ctx, int64(2), char, "User 2 scene"); err != nil {
		t.Fatalf("upsert user2: %v", err)
	}

	text, err := s.GetComponentHMMSceneText(ctx, int64(3), char)
	if err != nil {
		t.Fatalf("get user3: %v", err)
	}
	if text != "" {
		t.Errorf("user 3 should not see user 2's scene, got %q", text)
	}
}

func TestGetComponentList_IncludesPinyin(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "女", "woman", "", "", "", `["nǚ"]`)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(time.Hour))

	items, _, err := s.GetComponentList(ctx, int64(2), "", 1, 50, false)
	if err != nil {
		t.Fatalf("GetComponentList: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Pinyin != "nǚ" {
		t.Errorf("want pinyin %q, got %q", "nǚ", items[0].Pinyin)
	}
}

func TestGetComponentList_MultipleReadingsJoined(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "行", "row/walk", "", "", "", `["háng","xíng"]`)
	s.InsertComponentProgressForTest(ctx, int64(2), "行", time.Now().Add(time.Hour))

	items, _, err := s.GetComponentList(ctx, int64(2), "", 1, 50, false)
	if err != nil {
		t.Fatalf("GetComponentList: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Pinyin != "háng / xíng" {
		t.Errorf("want pinyin %q, got %q", "háng / xíng", items[0].Pinyin)
	}
}

func TestGetComponentList_NullPinyinOmitted(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	// SeedHanziDecompositionForTest does not set pinyin (NULL).
	if err := s.SeedHanziDecompositionForTest(ctx, "日", "sun"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, int64(2), "日", time.Now().Add(time.Hour))

	items, _, err := s.GetComponentList(ctx, int64(2), "", 1, 50, false)
	if err != nil {
		t.Fatalf("GetComponentList: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Pinyin != "" {
		t.Errorf("want empty pinyin for NULL, got %q", items[0].Pinyin)
	}
}

func TestInitComponentsForWord_EtymologyIdempotent(t *testing.T) {
	s := openTestDB(t)
	ety := `{"type":"pictophonetic","phonetic":"马","semantic":"女"}`
	seedHanziFull(t, s, "妈", "mother", "⿰女马", ety, "女", "")
	seedHanziDef(t, s, "女", "woman")
	seedHanziDef(t, s, "马", "horse")

	for i := 0; i < 3; i++ {
		if err := s.InitComponentsForWord(context.Background(), int64(2), "妈", time.Now()); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM component_progress WHERE user_id = 2`).Scan(&count)
	if count != 1 {
		t.Errorf("want 1 component row (idempotent), got %d", count)
	}
}

// ── Component coverage tests ────────────────────────────────────────────────

// seedZhWord inserts a zh word row directly (bypassing CreateWord) so coverage
// tallies have vocabulary to count against.
func seedZhWord(t *testing.T, s *Store, userID int64, text string) {
	t.Helper()
	if _, err := s.db.Exec(
		`INSERT INTO words (text, language, user_id) VALUES (?, 'zh', ?)`, text, userID,
	); err != nil {
		t.Fatalf("seedZhWord %q: %v", text, err)
	}
}

// setComponentThreshold ensures a user_settings row exists and sets its
// component_coverage_threshold directly.
func setComponentThreshold(t *testing.T, s *Store, userID int64, threshold float64) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.GetUserSettings(ctx, userID); err != nil {
		t.Fatalf("ensure user settings: %v", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE user_settings SET component_coverage_threshold = ? WHERE user_id = ?`,
		threshold, userID,
	); err != nil {
		t.Fatalf("set component_coverage_threshold: %v", err)
	}
}

func TestWordRequiredComponents_DedupsRepeatedCharacter(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	ety := `{"type":"pictophonetic","phonetic":"马","semantic":"女","hint":"mother"}`
	seedHanziFull(t, s, "妈", "mother", "⿰女马", ety, "女", "")
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziDef(t, s, "马", "horse")

	comps, err := wordRequiredComponents(ctx, s.db, "妈妈")
	if err != nil {
		t.Fatalf("wordRequiredComponents: %v", err)
	}
	if len(comps) != 1 || comps[0] != "女" {
		t.Errorf("want [女] (deduped across repeated 妈, phonetic 马 excluded), got %v", comps)
	}
}

func TestComponentWordSets_MembershipAndTotal(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "明", "bright", "⿰日月", `{"type":"ideographic","hint":"sun+moon"}`, "日", "")
	seedHanziDef(t, s, "日", "sun; day")
	seedHanziDef(t, s, "月", "moon; month")

	const userID = int64(2)
	seedZhWord(t, s, userID, "妈") // no decomposition seeded for 妈 here -> contributes nothing
	seedZhWord(t, s, userID, "明")
	seedZhWord(t, s, userID, "明月")

	wordSets, wordComponentCounts, total, err := componentWordSets(ctx, s.db, userID)
	if err != nil {
		t.Fatalf("componentWordSets: %v", err)
	}
	if total != 3 {
		t.Errorf("want total=3, got %d", total)
	}
	if len(wordSets["日"]) != 2 || len(wordSets["月"]) != 2 {
		t.Errorf("want 日:2 月:2 words, got %v", wordSets)
	}
	if len(wordSets) != 2 {
		t.Errorf("want exactly 2 distinct components, got %v", wordSets)
	}
	// 妈 has no decomposition seeded → 0 components; 明 and 明月 each have 日+月 → 2.
	for wid, cnt := range wordComponentCounts {
		if cnt != 0 && cnt != 2 {
			t.Errorf("unexpected component count %d for word %d", cnt, wid)
		}
	}
	if len(wordComponentCounts) != 3 {
		t.Errorf("want 3 entries in wordComponentCounts, got %d", len(wordComponentCounts))
	}
}

func TestSelectComponentsForCoverage_AllComponentsRequired(t *testing.T) {
	// Word 1 needs 日 and 月 (both required to cover it).
	// Word 2 needs 日 and 女 (both required to cover it).
	// Word 3 needs 女 only.
	// 日 alone covers neither word 1 nor word 2 (each needs a second component).
	// 女 alone immediately fully covers word 3.
	// To reach 2 fully-covered words we need e.g. 日+月 (covers word1) or 日+女 (covers word2+word3).
	wordSets := map[string]map[int64]bool{
		"日": {1: true, 2: true},
		"月": {1: true},
		"女": {2: true, 3: true},
	}
	wordComponentCounts := map[int64]int{1: 2, 2: 2, 3: 1}

	// 33% of 3 = 1 word. Only 女 can immediately fully cover a word (word 3, count=1).
	// 日 and 月 alone don't yet cover anything fully.
	got := selectComponentsForCoverage(wordSets, wordComponentCounts, 3, 33)
	if len(got) != 1 || got[0] != "女" {
		t.Errorf("want [女] at 33%% target, got %v", got)
	}

	// 50% of 3 = ceil(1.5) = 2 words. After 女: word3 covered (1), word2 remaining needs 日.
	// Next: 日 covers word2 (now both 女+日 selected, word2 fully covered = 2 words).
	got = selectComponentsForCoverage(wordSets, wordComponentCounts, 3, 50)
	if len(got) != 2 || got[0] != "女" || got[1] != "日" {
		t.Errorf("want [女 日] at 50%% target, got %v", got)
	}

	// 100% of 3 = 3 words -> needs 女, 日, 月 (word1 still needs 月).
	got = selectComponentsForCoverage(wordSets, wordComponentCounts, 3, 100)
	if len(got) != 3 {
		t.Errorf("want all 3 components at 100%% target, got %v", got)
	}
}

func TestSelectComponentsForCoverage_WordsWithNoComponentsAlwaysCovered(t *testing.T) {
	// Word 1 has no trainable components (count=0, not in any wordSet) -> auto-covered.
	// Word 2 needs 日. Total 2 words; even 50% target needs word 2 covered.
	wordSets := map[string]map[int64]bool{
		"日": {2: true},
	}
	wordComponentCounts := map[int64]int{1: 0, 2: 1}

	// 50% of 2 = 1 word. Word 1 is auto-covered; word 2 needs 日. So 1 is already
	// covered -> target met without selecting anything? No: we start with covered=1
	// (the zero-component words), target=1, so loop exits immediately with 0 picks.
	got := selectComponentsForCoverage(wordSets, wordComponentCounts, 2, 50)
	if len(got) != 0 {
		t.Errorf("want 0 selections when auto-covered words already meet target, got %v", got)
	}

	// 100% of 2 = 2 words -> word 2 needs 日.
	got = selectComponentsForCoverage(wordSets, wordComponentCounts, 2, 100)
	if len(got) != 1 || got[0] != "日" {
		t.Errorf("want [日] at 100%% target, got %v", got)
	}
}

func TestSelectComponentsForCoverage_ZeroTargetSelectsNothing(t *testing.T) {
	wordSets := map[string]map[int64]bool{"日": {1: true}}
	wordComponentCounts := map[int64]int{1: 1}
	if got := selectComponentsForCoverage(wordSets, wordComponentCounts, 1, 0); got != nil {
		t.Errorf("want nil selection at target 0, got %v", got)
	}
}

func TestGetComponentCoverageThreshold_DefaultsToZero(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	got, err := getComponentCoverageThreshold(ctx, s.db, int64(2))
	if err != nil {
		t.Fatalf("getComponentCoverageThreshold: %v", err)
	}
	if got != 0 {
		t.Errorf("want 0 for a fresh user with no settings row, got %v", got)
	}
}

func TestGetComponentCoverageThreshold_ReadsStoredValue(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	setComponentThreshold(t, s, int64(2), 12.5)

	got, err := getComponentCoverageThreshold(ctx, s.db, int64(2))
	if err != nil {
		t.Fatalf("getComponentCoverageThreshold: %v", err)
	}
	if got != 12.5 {
		t.Errorf("want 12.5, got %v", got)
	}
}

func TestInitComponentsForWord_ThresholdZero_IncludesLowCoverageComponent(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "明", "bright", "⿰日月", `{"type":"ideographic","hint":"sun+moon"}`, "日", "")
	seedHanziDef(t, s, "日", "sun; day")
	seedHanziDef(t, s, "月", "moon; month")

	const userID = int64(2)
	// Only one other word in the whole vocabulary uses 明 — very low coverage —
	// but the default threshold (0, never set) must never filter anything out.
	seedZhWord(t, s, userID, "甲乙") // unrelated filler word, no decomposition

	if err := s.InitComponentsForWord(ctx, userID, "明", time.Now()); err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM component_progress WHERE user_id = ?`, userID).Scan(&count)
	if count != 2 {
		t.Errorf("want both 日 and 月 inserted at default threshold 0, got %d rows", count)
	}
}

func TestInitComponentsForWord_ThresholdSelectsMinimalSetToReachTarget(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	// 甲 requires only 日 (一 has no seeded definition, so it's filtered out).
	seedHanziFull(t, s, "甲", "armor", "⿱日一", `{"type":"ideographic","hint":"filler"}`, "日", "")
	seedHanziDef(t, s, "日", "sun; day")
	// 乙 requires only 月 (一 has no seeded definition, so it's filtered out).
	seedHanziFull(t, s, "乙", "second", "⿱月一", `{"type":"ideographic","hint":"filler"}`, "月", "")
	seedHanziDef(t, s, "月", "moon; month")
	// 明 requires both 日 and 月.
	seedHanziFull(t, s, "明", "bright", "⿰日月", `{"type":"ideographic","hint":"sun+moon"}`, "日", "")

	const userID = int64(2)
	// 3 pre-existing words need only 日, 1 needs only 月 — 4 words total. 日
	// alone already covers 3 of them, so a 60% target (2.4 words) is reached
	// by 日 alone; 月 would add nothing new that 日 doesn't already cover for
	// those pre-existing words, so it's skipped as not worth the training slot.
	seedZhWord(t, s, userID, "甲子")
	seedZhWord(t, s, userID, "甲丑")
	seedZhWord(t, s, userID, "甲寅")
	seedZhWord(t, s, userID, "乙卯")

	setComponentThreshold(t, s, userID, 60)

	if err := s.InitComponentsForWord(ctx, userID, "明", time.Now()); err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}

	var chars []string
	rows, err := s.db.Query(`SELECT character FROM component_progress WHERE user_id = ? ORDER BY character`, userID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatalf("scan: %v", err)
		}
		chars = append(chars, c)
	}
	rows.Close()

	if len(chars) != 1 || chars[0] != "日" {
		t.Errorf("want only [日] (already enough to reach the 60%% coverage target), got %v", chars)
	}
}

func TestInitComponentsForWord_ThresholdSelectsBothWhenTargetNeedsFullCoverage(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "甲", "armor", "⿱日一", `{"type":"ideographic","hint":"filler"}`, "日", "")
	seedHanziDef(t, s, "日", "sun; day")
	seedHanziFull(t, s, "乙", "second", "⿱月一", `{"type":"ideographic","hint":"filler"}`, "月", "")
	seedHanziDef(t, s, "月", "moon; month")
	seedHanziFull(t, s, "明", "bright", "⿰日月", `{"type":"ideographic","hint":"sun+moon"}`, "日", "")

	const userID = int64(2)
	seedZhWord(t, s, userID, "甲子")
	seedZhWord(t, s, userID, "甲丑")
	seedZhWord(t, s, userID, "甲寅")
	seedZhWord(t, s, userID, "乙卯")

	// 90% of 4 pre-existing words = 3.6 -> 日 alone (3) isn't enough; 月 is
	// also needed to cover the remaining word.
	setComponentThreshold(t, s, userID, 90)

	if err := s.InitComponentsForWord(ctx, userID, "明", time.Now()); err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}

	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM component_progress WHERE user_id = ?`, userID).Scan(&count)
	if count != 2 {
		t.Errorf("want both 日 and 月 inserted to reach the 90%% coverage target, got %d rows", count)
	}
}

func TestGetComponentCoverage_ReturnsWordIDSetsSortedByCharacter(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "明", "bright", "⿰日月", `{"type":"ideographic","hint":"sun+moon"}`, "日", "")
	seedHanziDef(t, s, "日", "sun; day")
	seedHanziDef(t, s, "月", "moon; month")
	ety := `{"type":"pictophonetic","phonetic":"马","semantic":"女","hint":"mother"}`
	seedHanziFull(t, s, "妈", "mother", "⿰女马", ety, "女", "")
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziDef(t, s, "马", "horse")

	const userID = int64(2)
	seedZhWord(t, s, userID, "明")
	seedZhWord(t, s, userID, "明月")
	seedZhWord(t, s, userID, "妈")
	// 日 and 月 each cover 2 of the 3 words, 女 covers 1 (马 excluded — phonetic-only).

	items, _, total, trained, err := s.GetComponentCoverage(ctx, userID)
	if err != nil {
		t.Fatalf("GetComponentCoverage: %v", err)
	}
	if total != 3 {
		t.Errorf("want total_words=3, got %d", total)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 components (日, 月, 女), got %d: %+v", len(items), items)
	}
	// Sorted by character ascending.
	if items[0].Character != "女" || items[1].Character != "日" || items[2].Character != "月" {
		t.Errorf("want order [女 日 月], got [%s %s %s]", items[0].Character, items[1].Character, items[2].Character)
	}
	byChar := map[string]ComponentCoverageComponent{}
	for _, it := range items {
		byChar[it.Character] = it
	}
	if len(byChar["日"].WordIDs) != 2 || len(byChar["月"].WordIDs) != 2 {
		t.Errorf("want 日 and 月 each covering 2 words, got %v", items)
	}
	if len(byChar["女"].WordIDs) != 1 {
		t.Errorf("want 女 covering 1 word, got %v", byChar["女"].WordIDs)
	}
	if len(trained) != 0 {
		t.Errorf("want no trained components (none added to component_progress yet), got %v", trained)
	}
}

// TestGetComponentCoverage_ReturnsTrainedCharacters verifies that
// GetComponentCoverage reports which qualifying components already have a
// component_progress row for the user — used by the Settings page to show
// how many components are already in training (issue #352).
func TestGetComponentCoverage_ReturnsTrainedCharacters(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "明", "bright", "⿰日月", `{"type":"ideographic","hint":"sun+moon"}`, "日", "")
	seedHanziDef(t, s, "日", "sun; day")
	seedHanziDef(t, s, "月", "moon; month")

	const userID = int64(2)
	seedZhWord(t, s, userID, "明")
	s.InsertComponentProgressForTest(ctx, userID, "日", time.Now())
	// A trained row for a user's own component that no longer qualifies (or
	// belongs to another user) must not leak into another user's set.
	s.InsertComponentProgressForTest(ctx, int64(99), "月", time.Now())

	_, _, _, trained, err := s.GetComponentCoverage(ctx, userID)
	if err != nil {
		t.Fatalf("GetComponentCoverage: %v", err)
	}
	if len(trained) != 1 || !trained["日"] {
		t.Errorf("want trained = {日}, got %v", trained)
	}
}

// TestInitComponentsForWord_InsertsKnownCharacters verifies that components
// extracted from a word's hanzi decomposition are inserted into component_progress.
// "好" decomposes to ⿰女子, so components 女 and 子 (both with definitions) should be inserted.
func TestInitComponentsForWord_InsertsKnownCharacters(t *testing.T) {
	s := openTestDB(t)
	seedHanziDecomp(t, s, "好", "⿰女子")
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziDef(t, s, "子", "child; son")

	err := s.InitComponentsForWord(context.Background(), int64(2), "好", time.Now())
	if err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}

	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM component_progress WHERE user_id = 2`).Scan(&count)
	if count != 2 {
		t.Errorf("want 2 component rows (女 and 子), got %d", count)
	}
}

// TestInitComponentsForWord_SkipsNoDecomp verifies that characters with no
// decomposition entry are skipped (no component_progress rows inserted).
func TestInitComponentsForWord_SkipsNoDecomp(t *testing.T) {
	s := openTestDB(t)
	// No hanzi_decomposition entries at all → should not insert anything.
	err := s.InitComponentsForWord(context.Background(), int64(2), "好", time.Now())
	if err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM component_progress WHERE user_id = 2`).Scan(&count)
	if count != 0 {
		t.Errorf("want 0 component rows, got %d", count)
	}
}

// TestInitComponentsForWord_SkipsComponentsWithNoDefinition verifies that
// components without a definition are not inserted.
func TestInitComponentsForWord_SkipsComponentsWithNoDefinition(t *testing.T) {
	s := openTestDB(t)
	seedHanziDecomp(t, s, "好", "⿰女子")
	// Decomposition exists for 好 but neither 女 nor 子 has a definition.

	err := s.InitComponentsForWord(context.Background(), int64(2), "好", time.Now())
	if err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM component_progress WHERE user_id = 2`).Scan(&count)
	if count != 0 {
		t.Errorf("want 0 component rows, got %d", count)
	}
}

// TestInitComponentsForWord_Idempotent verifies that repeated calls do not
// create duplicate component_progress rows.
func TestInitComponentsForWord_Idempotent(t *testing.T) {
	s := openTestDB(t)
	seedHanziDecomp(t, s, "好", "⿰女子")
	seedHanziDef(t, s, "女", "woman") // 子 has no definition, so only 女 is inserted.

	for i := 0; i < 3; i++ {
		if err := s.InitComponentsForWord(context.Background(), int64(2), "好", time.Now()); err != nil {
			t.Fatalf("InitComponentsForWord iteration %d: %v", i, err)
		}
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM component_progress WHERE user_id = 2`).Scan(&count)
	if count != 1 {
		t.Errorf("want 1 component row (idempotent), got %d", count)
	}
}

func TestGetNextComponentCard_ReturnsNilWhenEmpty(t *testing.T) {
	s := openTestDB(t)
	card, err := s.GetNextComponentCard(context.Background(), int64(2), []string{"en"})
	if err != nil {
		t.Fatalf("GetNextComponentCard: %v", err)
	}
	if card != nil {
		t.Errorf("want nil card when no components, got %+v", card)
	}
}

// TestGetNextComponentCard_ReturnsDueCard verifies that a component inserted
// via the two-step lookup (word→decomposition→components) is returned as due.
func TestGetNextComponentCard_ReturnsDueCard(t *testing.T) {
	s := openTestDB(t)
	seedHanziDecomp(t, s, "好", "⿰女子")
	seedHanziDef(t, s, "女", "woman; female")
	past := time.Now().Add(-24 * time.Hour)
	if err := s.InitComponentsForWord(context.Background(), int64(2), "好", past); err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}

	card, err := s.GetNextComponentCard(context.Background(), int64(2), []string{"en"})
	if err != nil {
		t.Fatalf("GetNextComponentCard: %v", err)
	}
	if card == nil {
		t.Fatal("want a card, got nil")
	}
	if card.Character != "女" {
		t.Errorf("want character 女, got %q", card.Character)
	}
	if card.Definitions["en"] == "" {
		t.Error("want non-empty en definition")
	}
}

func TestRecordComponentAnswer_UpdatesProgress(t *testing.T) {
	s := openTestDB(t)
	seedHanziDef(t, s, "女", "woman")
	// Insert directly — this test is about RecordComponentAnswer, not InitComponentsForWord.
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", time.Now().Add(-time.Hour))

	p, _, err := s.RecordComponentAnswer(context.Background(), int64(2), "女", true)
	if err != nil {
		t.Fatalf("RecordComponentAnswer: %v", err)
	}
	if p.TotalCorrect != 1 {
		t.Errorf("want TotalCorrect=1, got %d", p.TotalCorrect)
	}
	if p.TotalAttempts != 1 {
		t.Errorf("want TotalAttempts=1, got %d", p.TotalAttempts)
	}
}

func TestRecordComponentStat_IncreasesCount(t *testing.T) {
	s := openTestDB(t)
	if err := s.RecordComponentStat(context.Background(), int64(2), true); err != nil {
		t.Fatalf("RecordComponentStat: %v", err)
	}
	var correct int
	s.db.QueryRow(`SELECT correct FROM component_stats WHERE user_id = 2 AND date = date('now')`).Scan(&correct)
	if correct != 1 {
		t.Errorf("want correct=1, got %d", correct)
	}
}

func TestGetComponentCounts_ReturnsCorrectCounts(t *testing.T) {
	s := openTestDB(t)
	seedHanziDef(t, s, "女", "woman")
	seedHanziTranslation(t, s, "女", "en", "woman")
	past := time.Now().Add(-24 * time.Hour)
	// Insert directly — this test is about GetComponentCounts, not InitComponentsForWord.
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", past)
	// Mark as seen so it counts toward due_today.
	s.db.Exec(`UPDATE component_progress SET first_seen_date = date('now') WHERE character = '女' AND user_id = 2`)

	due, total, err := s.GetComponentCounts(context.Background(), int64(2), []string{"en"})
	if err != nil {
		t.Fatalf("GetComponentCounts: %v", err)
	}
	if due != 1 {
		t.Errorf("want due=1, got %d", due)
	}
	if total != 1 {
		t.Errorf("want total=1, got %d", total)
	}
}

// TestGetComponentCounts_FiltersByLang reproduces issues #230/#232: a user
// training in a non-English language (e.g. German) saw "Due today: 0" while
// GetNextComponentCard kept serving a due component whose only translation
// was in German — because GetComponentCounts hardcoded lang='EN' instead of
// honoring the caller's configured langs.
func TestGetComponentCounts_FiltersByLang(t *testing.T) {
	s := openTestDB(t)
	if _, err := s.db.Exec(`INSERT INTO hanzi_decomposition (character, definition) VALUES ('女', 'woman')`); err != nil {
		t.Fatalf("seed hanzi_decomposition: %v", err)
	}
	seedHanziTranslation(t, s, "女", "de", "Frau")
	past := time.Now().Add(-24 * time.Hour)
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", past)
	s.db.Exec(`UPDATE component_progress SET first_seen_date = date('now') WHERE character = '女' AND user_id = 2`)

	due, _, err := s.GetComponentCounts(context.Background(), int64(2), []string{"de"})
	if err != nil {
		t.Fatalf("GetComponentCounts: %v", err)
	}
	if due != 1 {
		t.Errorf("want due=1 for de-only component with langs=[de], got %d", due)
	}

	due, _, err = s.GetComponentCounts(context.Background(), int64(2), []string{"en"})
	if err != nil {
		t.Fatalf("GetComponentCounts: %v", err)
	}
	if due != 0 {
		t.Errorf("want due=0 for de-only component with langs=[en], got %d", due)
	}
}

func TestGetComponentDefinitions_ENOnly(t *testing.T) {
	s := openTestDB(t)
	seedHanziDef(t, s, "女", "woman; female")

	defs, err := s.GetComponentDefinitions(context.Background(), 2, "女", []string{"en"})
	if err != nil {
		t.Fatalf("GetComponentDefinitions: %v", err)
	}
	if defs["en"] != "woman; female" {
		t.Errorf("want en=woman; female, got %q", defs["en"])
	}
	if _, ok := defs["de"]; ok {
		t.Error("want no de entry when not requested")
	}
}

func TestGetComponentDefinitions_ENAndDE(t *testing.T) {
	s := openTestDB(t)
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziTranslation(t, s, "女", "de", "Frau; weiblich")

	defs, err := s.GetComponentDefinitions(context.Background(), 2, "女", []string{"en", "de"})
	if err != nil {
		t.Fatalf("GetComponentDefinitions: %v", err)
	}
	if defs["en"] != "woman; female" {
		t.Errorf("want en=woman; female, got %q", defs["en"])
	}
	if defs["de"] != "Frau; weiblich" {
		t.Errorf("want de=Frau; weiblich, got %q", defs["de"])
	}
}

func TestGetComponentDefinitions_MissingDEOmitted(t *testing.T) {
	s := openTestDB(t)
	seedHanziDef(t, s, "女", "woman")
	// No DE translation seeded.

	defs, err := s.GetComponentDefinitions(context.Background(), 2, "女", []string{"en", "de"})
	if err != nil {
		t.Fatalf("GetComponentDefinitions: %v", err)
	}
	if defs["en"] != "woman" {
		t.Errorf("want en=woman, got %q", defs["en"])
	}
	if _, ok := defs["de"]; ok {
		t.Error("want de omitted when no translation exists")
	}
}

func TestGetNextComponentCard_DELangFilter(t *testing.T) {
	s := openTestDB(t)
	// 女 has only EN definition, no DE translation → should be skipped when DE-only.
	seedHanziDecomp(t, s, "好", "⿰女子")
	seedHanziDef(t, s, "女", "woman; female")
	past := time.Now().Add(-24 * time.Hour)
	if err := s.InitComponentsForWord(context.Background(), int64(2), "好", past); err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}

	card, err := s.GetNextComponentCard(context.Background(), int64(2), []string{"de"})
	if err != nil {
		t.Fatalf("GetNextComponentCard: %v", err)
	}
	if card != nil {
		t.Errorf("want nil when no DE translation available, got card for %q", card.Character)
	}
}

func TestGetNextComponentCard_DEWithTranslation(t *testing.T) {
	s := openTestDB(t)
	seedHanziDecomp(t, s, "好", "⿰女子")
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziTranslation(t, s, "女", "de", "Frau; weiblich")
	past := time.Now().Add(-24 * time.Hour)
	if err := s.InitComponentsForWord(context.Background(), int64(2), "好", past); err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}

	card, err := s.GetNextComponentCard(context.Background(), int64(2), []string{"de"})
	if err != nil {
		t.Fatalf("GetNextComponentCard: %v", err)
	}
	if card == nil {
		t.Fatal("want card with DE translation, got nil")
	}
	if card.Character != "女" {
		t.Errorf("want character 女, got %q", card.Character)
	}
	if card.Definitions["de"] != "Frau; weiblich" {
		t.Errorf("want de=Frau; weiblich, got %q", card.Definitions["de"])
	}
}

func TestGetNextComponentCard_ENAndDE(t *testing.T) {
	s := openTestDB(t)
	seedHanziDecomp(t, s, "好", "⿰女子")
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziTranslation(t, s, "女", "de", "Frau")
	past := time.Now().Add(-24 * time.Hour)
	if err := s.InitComponentsForWord(context.Background(), int64(2), "好", past); err != nil {
		t.Fatalf("InitComponentsForWord: %v", err)
	}

	card, err := s.GetNextComponentCard(context.Background(), int64(2), []string{"en", "de"})
	if err != nil {
		t.Fatalf("GetNextComponentCard: %v", err)
	}
	if card == nil {
		t.Fatal("want card, got nil")
	}
	if card.Definitions["en"] == "" {
		t.Error("want non-empty en definition")
	}
	if card.Definitions["de"] != "Frau" {
		t.Errorf("want de=Frau, got %q", card.Definitions["de"])
	}
}

// TestGetNextComponentCard_DoesNotServeFutureComponent verifies that a component
// with a due_date in the future (even if within 24 hours) is not returned.
// Regression test for issue #238: GetNextComponentCard used datetime('now', '+1 day')
// instead of date('now', '+1 day'), so a component answered correctly (new due_date =
// now+22-24h via SM-2) was immediately re-served in the same session while the stats
// counter correctly showed 0.
func TestGetNextComponentCard_DoesNotServeFutureComponent(t *testing.T) {
	s := openTestDB(t)
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziTranslation(t, s, "女", "en", "woman")
	// Simulate a component that comes due just after the next midnight — the
	// earliest instant with tomorrow's date. This is within the 22-24h window
	// SM-2 produces after a correct answer (interval=1 day + jitter), so the
	// pre-fix datetime('now', '+1 day') comparison would have served it, while
	// the fixed date('now', '+1 day') bound must not. A fixed clock offset like
	// now+23h is wrong here: between 00:00 and 01:00 it still lands on today's
	// date, which the date-based bound correctly serves, making the test flaky.
	now := time.Now().UTC()
	future := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 1, 0, time.UTC)
	s.InsertComponentProgressForTest(context.Background(), int64(2), "女", future)

	card, err := s.GetNextComponentCard(context.Background(), int64(2), []string{"en"})
	if err != nil {
		t.Fatalf("GetNextComponentCard: %v", err)
	}
	if card != nil {
		t.Errorf("want nil for future-due component (due in ~23h), got card for %q", card.Character)
	}
}

func TestDetectComponentConfusion_MatchesOtherComponent(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "扑", "to rap, to tap; script; to let go")
	seedHanziDef(t, s, "去", "to go")
	s.InsertComponentProgressForTest(ctx, int64(2), "扑", time.Now())
	s.InsertComponentProgressForTest(ctx, int64(2), "去", time.Now())

	wordID, comp, found, err := s.DetectComponentConfusion(ctx, int64(2), "扑", "to go", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected a confusion to be found")
	}
	if wordID != 0 {
		t.Errorf("wordID: want 0, got %d", wordID)
	}
	if comp != "去" {
		t.Errorf("component: want 去, got %q", comp)
	}
}

func TestDetectComponentConfusion_MatchesWordTranslation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "扑", "to rap, to tap; script; to let go")
	s.InsertComponentProgressForTest(ctx, int64(2), "扑", time.Now())
	wordID := seedWord(t, s, "去", "qù", []string{"to go"})

	confusedWordID, comp, found, err := s.DetectComponentConfusion(ctx, int64(2), "扑", "to go", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected a confusion to be found")
	}
	if confusedWordID != wordID {
		t.Errorf("confusedWordID: want %d, got %d", wordID, confusedWordID)
	}
	if comp != "" {
		t.Errorf("component: want empty, got %q", comp)
	}
}

func TestDetectComponentConfusion_NoMatch(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "扑", "to rap, to tap; script; to let go")
	s.InsertComponentProgressForTest(ctx, int64(2), "扑", time.Now())

	_, _, found, err := s.DetectComponentConfusion(ctx, int64(2), "扑", "completely unrelated", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected no confusion to be found")
	}
}

// TestDetectComponentConfusion_ExcludesOwnWordCounterpart ensures that when a
// component character is also a standalone zh word for the user (is_also_word),
// its own translation is never reported as a "confusion" with itself.
func TestDetectComponentConfusion_ExcludesOwnWordCounterpart(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "去", "to go")
	s.InsertComponentProgressForTest(ctx, int64(2), "去", time.Now())
	seedWord(t, s, "去", "qù", []string{"to go"})

	_, _, found, err := s.DetectComponentConfusion(ctx, int64(2), "去", "to go", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("should not report a component's own word counterpart as a confusion")
	}
}

func TestUpsertComponentConfusion_IncrementsCount(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if err := s.UpsertComponentConfusion(ctx, int64(2), "扑", 0, "去", "zh_pinyin_to_transl"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertComponentConfusion(ctx, int64(2), "扑", 0, "去", "zh_pinyin_to_transl"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 confusion, got %d", len(items))
	}
	if items[0].Count != 2 {
		t.Errorf("count: want 2, got %d", items[0].Count)
	}
	if items[0].ZhKind != models.ConfusionKindComponent || items[0].ZhComponent != "扑" {
		t.Errorf("zh side: want component 扑, got kind=%s component=%q", items[0].ZhKind, items[0].ZhComponent)
	}
	if items[0].ConfusedWithKind != models.ConfusionKindComponent || items[0].ConfusedWithComponent != "去" {
		t.Errorf("confused_with side: want component 去, got kind=%s component=%q", items[0].ConfusedWithKind, items[0].ConfusedWithComponent)
	}
}

func TestGetComponentConfusionDetail_ReturnsRow_ComponentVsWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "扑", "to rap, to tap; script; to let go")
	wordID := seedWord(t, s, "去", "qù", []string{"to go"})

	if err := s.UpsertComponentConfusion(ctx, int64(2), "扑", wordID, "", "zh_pinyin_to_transl"); err != nil {
		t.Fatal(err)
	}

	d, err := s.GetComponentConfusionDetail(ctx, int64(2), "扑", wordID, "", "zh_pinyin_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("expected a row, got nil")
	}
	if d.ZhKind != models.ConfusionKindComponent || d.ZhComponent != "扑" || d.ZhText != "扑" {
		t.Errorf("zh side: got kind=%s component=%q text=%q", d.ZhKind, d.ZhComponent, d.ZhText)
	}
	if d.ConfusedWithKind != models.ConfusionKindWord || d.ConfusedWithID != wordID || d.ConfusedWithText != "去" {
		t.Errorf("confused_with side: got kind=%s id=%d text=%q", d.ConfusedWithKind, d.ConfusedWithID, d.ConfusedWithText)
	}
	if len(d.ConfusedWithTranslations["en"]) == 0 || d.ConfusedWithTranslations["en"][0] != "to go" {
		t.Errorf("expected confused_with translations to include 'to go', got %v", d.ConfusedWithTranslations)
	}
}

func TestGetComponentConfusionDetail_MissingReturnsNil(t *testing.T) {
	s := openTestDB(t)
	d, err := s.GetComponentConfusionDetail(context.Background(), int64(2), "扑", 99999, "", "zh_pinyin_to_transl", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if d != nil {
		t.Errorf("expected nil for missing row, got %+v", d)
	}
}

func TestGetComponentList_BasicAndSearch(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziDef(t, s, "日", "sun; day")
	past := time.Now().Add(-time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)
	s.InsertComponentProgressForTest(ctx, int64(2), "日", past)

	// All components
	items, total, err := s.GetComponentList(ctx, int64(2), "", 1, 20, false)
	if err != nil {
		t.Fatalf("GetComponentList: %v", err)
	}
	if total != 2 {
		t.Errorf("want total=2, got %d", total)
	}
	if len(items) != 2 {
		t.Errorf("want 2 items, got %d", len(items))
	}

	// Search by definition
	items, total, err = s.GetComponentList(ctx, int64(2), "sun", 1, 20, false)
	if err != nil {
		t.Fatalf("GetComponentList search: %v", err)
	}
	if total != 1 {
		t.Errorf("want total=1 for 'sun', got %d", total)
	}
	if len(items) != 1 || items[0].Character != "日" {
		t.Errorf("want 日, got %+v", items)
	}
	if items[0].DefinitionEN != "sun; day" {
		t.Errorf("want definition_en='sun; day', got %q", items[0].DefinitionEN)
	}
}

// TestGetComponentList_UsesCharLangIndex guards against the search-is-slow
// regression: the LEFT JOINs to hanzi_decomposition_translation must be able
// to use an index on (character, lang), not fall back to a full table scan.
// The v59 indexes are partial (WHERE user_id IS [NOT] NULL) and can't serve a
// join that doesn't filter on user_id, so this needs its own plain index.
func TestGetComponentList_UsesCharLangIndex(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = 'hanzi_decomposition_translation' AND sql LIKE '%(character, lang)%'`,
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count == 0 {
		t.Fatal("want a plain index on hanzi_decomposition_translation(character, lang)")
	}

	rows, err := s.db.QueryContext(ctx, `EXPLAIN QUERY PLAN
		SELECT cp.character
		FROM component_progress cp
		LEFT JOIN hanzi_decomposition_translation hdt_en
		       ON hdt_en.character = cp.character AND hdt_en.lang = 'EN'
		WHERE cp.user_id = ? AND LOWER(hdt_en.definition) LIKE LOWER(?)`,
		int64(2), "%sun%",
	)
	if err != nil {
		t.Fatalf("explain query plan: %v", err)
	}
	defer rows.Close()
	var sawIndexedJoin bool
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan plan row: %v", err)
		}
		if strings.Contains(detail, "hdt_en") && strings.Contains(detail, "idx_hanzi_trans_char_lang") {
			sawIndexedJoin = true
		}
	}
	if !sawIndexedJoin {
		t.Error("want the hdt_en join to use idx_hanzi_trans_char_lang")
	}
}

func TestGetComponentList_IsAlsoWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "关", "close")
	seedHanziDef(t, s, "女", "woman")
	past := time.Now().Add(-time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "关", past)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)
	seedWord(t, s, "关", "guān", []string{"close"})

	items, _, err := s.GetComponentList(ctx, int64(2), "", 1, 20, false)
	if err != nil {
		t.Fatalf("GetComponentList: %v", err)
	}
	byChar := map[string]bool{}
	for _, it := range items {
		byChar[it.Character] = it.IsAlsoWord
	}
	if !byChar["关"] {
		t.Error("want 关 flagged as also a word")
	}
	if byChar["女"] {
		t.Error("女 should not be flagged as also a word")
	}
}

func TestStoreComponentTranslation_UpsertAndRetrieve(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "女", "woman")

	if err := s.StoreComponentTranslation(context.Background(), 2, "女", "de", "Frau"); err != nil {
		t.Fatalf("StoreComponentTranslation: %v", err)
	}
	defs, err := s.GetComponentDefinitions(ctx, 2, "女", []string{"de"})
	if err != nil {
		t.Fatalf("GetComponentDefinitions after store: %v", err)
	}
	if defs["de"] != "Frau" {
		t.Errorf("want de=Frau, got %q", defs["de"])
	}
}

func TestStoreComponentTranslation_UpdateExisting(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "女", "woman")
	seedHanziTranslation(t, s, "女", "de", "alt")

	if err := s.StoreComponentTranslation(context.Background(), 2, "女", "de", "Frau neu"); err != nil {
		t.Fatalf("StoreComponentTranslation update: %v", err)
	}
	defs, err := s.GetComponentDefinitions(ctx, 2, "女", []string{"de"})
	if err != nil {
		t.Fatalf("GetComponentDefinitions: %v", err)
	}
	if defs["de"] != "Frau neu" {
		t.Errorf("want de=Frau neu, got %q", defs["de"])
	}
}

func TestGetComponentTranslations_ReturnsAllLangs(t *testing.T) {
	s := openTestDB(t)
	seedHanziDef(t, s, "女", "woman")
	seedHanziTranslation(t, s, "女", "en", "woman")
	seedHanziTranslation(t, s, "女", "de", "Frau")

	got, err := s.GetComponentTranslations(context.Background(), 2, "女")
	if err != nil {
		t.Fatalf("GetComponentTranslations: %v", err)
	}
	if got["en"] != "woman" {
		t.Errorf("want en=woman, got %q", got["en"])
	}
	if got["de"] != "Frau" {
		t.Errorf("want de=Frau, got %q", got["de"])
	}
}

func TestGetComponentTranslations_EmptyForUnknownChar(t *testing.T) {
	s := openTestDB(t)
	got, err := s.GetComponentTranslations(context.Background(), 2, "X")
	if err != nil {
		t.Fatalf("GetComponentTranslations: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty map, got %v", got)
	}
}

func TestGetComponentDefinitions_ENFromTranslationTable(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	// Seed EN only in translation table, NOT in hanzi_decomposition.definition.
	_, err := s.db.Exec(`INSERT INTO hanzi_decomposition (character) VALUES (?) ON CONFLICT DO NOTHING`, "水")
	if err != nil {
		t.Fatalf("seed bare hanzi: %v", err)
	}
	seedHanziTranslation(t, s, "水", "en", "water")

	defs, err := s.GetComponentDefinitions(ctx, 2, "水", []string{"en"})
	if err != nil {
		t.Fatalf("GetComponentDefinitions: %v", err)
	}
	if defs["en"] != "water" {
		t.Errorf("want en=water from translation table, got %q", defs["en"])
	}
}

func TestMarkComponentForReview_SetsFlag(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "女", "woman")
	past := time.Now().Add(-time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)

	if err := s.MarkComponentForReview(int64(2), "女"); err != nil {
		t.Fatalf("MarkComponentForReview: %v", err)
	}

	var flag int
	err := s.db.QueryRowContext(ctx,
		`SELECT needs_review FROM component_progress WHERE user_id = ? AND character = ?`,
		int64(2), "女",
	).Scan(&flag)
	if err != nil {
		t.Fatalf("scan needs_review: %v", err)
	}
	if flag != 1 {
		t.Errorf("want needs_review=1, got %d", flag)
	}
}

func TestGetComponentList_ReviewOnly(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "女", "woman")
	seedHanziTranslation(t, s, "女", "en", "woman")
	seedHanziDef(t, s, "日", "sun")
	seedHanziTranslation(t, s, "日", "en", "sun")
	past := time.Now().Add(-time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)
	s.InsertComponentProgressForTest(ctx, int64(2), "日", past)

	if err := s.MarkComponentForReview(int64(2), "女"); err != nil {
		t.Fatalf("MarkComponentForReview: %v", err)
	}

	items, total, err := s.GetComponentList(ctx, int64(2), "", 1, 20, true)
	if err != nil {
		t.Fatalf("GetComponentList reviewOnly: %v", err)
	}
	if total != 1 {
		t.Errorf("want total=1 with reviewOnly, got %d", total)
	}
	if len(items) != 1 || items[0].Character != "女" {
		t.Errorf("want only 女, got %+v", items)
	}
}

func TestGetComponentList_ReviewOnlyFalse(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "女", "woman")
	seedHanziTranslation(t, s, "女", "en", "woman")
	seedHanziDef(t, s, "日", "sun")
	seedHanziTranslation(t, s, "日", "en", "sun")
	past := time.Now().Add(-time.Hour)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", past)
	s.InsertComponentProgressForTest(ctx, int64(2), "日", past)
	if err := s.MarkComponentForReview(int64(2), "女"); err != nil {
		t.Fatalf("MarkComponentForReview: %v", err)
	}

	items, total, err := s.GetComponentList(ctx, int64(2), "", 1, 20, false)
	if err != nil {
		t.Fatalf("GetComponentList not reviewOnly: %v", err)
	}
	if total != 2 {
		t.Errorf("want total=2 without reviewOnly, got %d", total)
	}
	if len(items) != 2 {
		t.Errorf("want 2 items, got %d", len(items))
	}
}

func TestComponentPrevState_RoundTrip(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, 2, "女", time.Now().Add(-time.Hour))

	p := models.ComponentProgress{
		Repetitions:   3,
		Easiness:      2.5,
		IntervalDays:  6,
		TotalCorrect:  3,
		TotalAttempts: 4,
	}
	if err := s.SaveComponentPrevState(ctx, 2, "女", p); err != nil {
		t.Fatalf("SaveComponentPrevState: %v", err)
	}

	got, err := s.GetComponentPrevState(ctx, 2, "女")
	if err != nil {
		t.Fatalf("GetComponentPrevState: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil prev state")
	}
	if got.Repetitions != p.Repetitions {
		t.Errorf("Repetitions: want %d, got %d", p.Repetitions, got.Repetitions)
	}
	if got.Easiness != p.Easiness {
		t.Errorf("Easiness: want %f, got %f", p.Easiness, got.Easiness)
	}
	if got.IntervalDays != p.IntervalDays {
		t.Errorf("IntervalDays: want %d, got %d", p.IntervalDays, got.IntervalDays)
	}
	if got.TotalCorrect != p.TotalCorrect {
		t.Errorf("TotalCorrect: want %d, got %d", p.TotalCorrect, got.TotalCorrect)
	}
	if got.TotalAttempts != p.TotalAttempts {
		t.Errorf("TotalAttempts: want %d, got %d", p.TotalAttempts, got.TotalAttempts)
	}
}

func TestComponentPrevState_NilWhenAbsent(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, 2, "女", time.Now().Add(-time.Hour))

	got, err := s.GetComponentPrevState(ctx, 2, "女")
	if err != nil {
		t.Fatalf("GetComponentPrevState: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for fresh component, got %+v", got)
	}
}

func TestComponentPrevState_ClearAfterAccept(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.SeedHanziDecompositionForTest(ctx, "女", "woman"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.InsertComponentProgressForTest(ctx, 2, "女", time.Now().Add(-time.Hour))

	p := models.ComponentProgress{Repetitions: 1, Easiness: 2.5, IntervalDays: 1}
	if err := s.SaveComponentPrevState(ctx, 2, "女", p); err != nil {
		t.Fatalf("SaveComponentPrevState: %v", err)
	}
	if err := s.ClearComponentPrevState(ctx, 2, "女"); err != nil {
		t.Fatalf("ClearComponentPrevState: %v", err)
	}
	got, err := s.GetComponentPrevState(ctx, 2, "女")
	if err != nil {
		t.Fatalf("GetComponentPrevState after clear: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after clear, got %+v", got)
	}
}

func TestComponentTranslation_FallsBackToGlobal(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "女", "woman") // seeds the shared global EN default

	defs, err := s.GetComponentDefinitions(ctx, 2, "女", []string{"en"})
	if err != nil {
		t.Fatalf("GetComponentDefinitions: %v", err)
	}
	if defs["en"] != "woman" {
		t.Errorf("want global fallback 'woman', got %q", defs["en"])
	}
}

func TestComponentTranslation_UserOverride(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "女", "woman")

	uid, err := s.CreateUserWithSettings(ctx, "override@example.de", "h", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.StoreComponentTranslation(ctx, uid, "女", "en", "female"); err != nil {
		t.Fatalf("StoreComponentTranslation: %v", err)
	}

	// The editing user sees their override...
	defs, err := s.GetComponentDefinitions(ctx, uid, "女", []string{"en"})
	if err != nil {
		t.Fatalf("GetComponentDefinitions: %v", err)
	}
	if defs["en"] != "female" {
		t.Errorf("want user override 'female', got %q", defs["en"])
	}
	// ...and the shared default is unchanged for everyone else.
	other, _ := s.GetComponentDefinitions(ctx, 999999, "女", []string{"en"})
	if other["en"] != "woman" {
		t.Errorf("global default must be untouched, got %q", other["en"])
	}
	// GetComponentTranslations applies the same overlay.
	tr, err := s.GetComponentTranslations(ctx, uid, "女")
	if err != nil {
		t.Fatalf("GetComponentTranslations: %v", err)
	}
	if tr["en"] != "female" {
		t.Errorf("GetComponentTranslations override: want 'female', got %q", tr["en"])
	}
}

func TestComponentTranslation_UserIsolation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "女", "woman")

	uA, err := s.CreateUserWithSettings(ctx, "a@example.de", "h", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create user A: %v", err)
	}
	uB, err := s.CreateUserWithSettings(ctx, "b@example.de", "h", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create user B: %v", err)
	}
	if err := s.StoreComponentTranslation(ctx, uA, "女", "en", "A-def"); err != nil {
		t.Fatalf("store A: %v", err)
	}
	if err := s.StoreComponentTranslation(ctx, uB, "女", "en", "B-def"); err != nil {
		t.Fatalf("store B: %v", err)
	}

	a, _ := s.GetComponentDefinitions(ctx, uA, "女", []string{"en"})
	b, _ := s.GetComponentDefinitions(ctx, uB, "女", []string{"en"})
	g, _ := s.GetComponentDefinitions(ctx, 999999, "女", []string{"en"})
	if a["en"] != "A-def" {
		t.Errorf("user A should see own def, got %q", a["en"])
	}
	if b["en"] != "B-def" {
		t.Errorf("user B should see own def, got %q", b["en"])
	}
	if g["en"] != "woman" {
		t.Errorf("a user without an override should see the global default, got %q", g["en"])
	}
}

// seedHanziDef inserts or updates only the definition for a character in hanzi_decomposition.
func seedHanziDef(t *testing.T, s *Store, character, definition string) {
	t.Helper()
	_, err := s.db.Exec(
		`INSERT INTO hanzi_decomposition (character, definition) VALUES (?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition`,
		character, definition,
	)
	if err != nil {
		t.Fatalf("seedHanziDef %q: %v", character, err)
	}
	// Also seed EN in translation table since GetComponentDefinitions reads from there.
	_, err = s.db.Exec(
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, 'EN', ?)
		 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
		character, definition,
	)
	if err != nil {
		t.Fatalf("seedHanziDef translation %q: %v", character, err)
	}
}

// seedHanziDecomp inserts or updates only the decomposition for a character in hanzi_decomposition.
func seedHanziDecomp(t *testing.T, s *Store, character, decomp string) {
	t.Helper()
	_, err := s.db.Exec(
		`INSERT INTO hanzi_decomposition (character, decomposition) VALUES (?, ?)
		 ON CONFLICT(character) DO UPDATE SET decomposition = excluded.decomposition`,
		character, decomp,
	)
	if err != nil {
		t.Fatalf("seedHanziDecomp %q: %v", character, err)
	}
}

// seedHanziTranslation inserts a row into hanzi_decomposition_translation for testing.
func seedHanziTranslation(t *testing.T, s *Store, character, lang, definition string) {
	t.Helper()
	_, err := s.db.Exec(
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, ?, ?)
		 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
		character, strings.ToUpper(lang), definition,
	)
	if err != nil {
		t.Fatalf("seedHanziTranslation %q %s: %v", character, lang, err)
	}
}
