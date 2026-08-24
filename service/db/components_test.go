package db

import (
	"context"
	"testing"
	"time"
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

	items, _, total, err := s.GetComponentCoverage(ctx, userID)
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
}
