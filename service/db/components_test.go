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

func TestComponentWordCoverage_CountsAndTotal(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "明", "bright", "⿰日月", `{"type":"ideographic","hint":"sun+moon"}`, "日", "")
	seedHanziDef(t, s, "日", "sun; day")
	seedHanziDef(t, s, "月", "moon; month")

	const userID = int64(2)
	seedZhWord(t, s, userID, "妈") // no decomposition seeded for 妈 here -> contributes nothing
	seedZhWord(t, s, userID, "明")
	seedZhWord(t, s, userID, "明月")

	counts, total, err := componentWordCoverage(ctx, s.db, userID)
	if err != nil {
		t.Fatalf("componentWordCoverage: %v", err)
	}
	if total != 3 {
		t.Errorf("want total=3, got %d", total)
	}
	if counts["日"] != 2 || counts["月"] != 2 {
		t.Errorf("want 日:2 月:2, got %v", counts)
	}
	if len(counts) != 2 {
		t.Errorf("want exactly 2 distinct components, got %v", counts)
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

func TestInitComponentsForWord_ThresholdExcludesLowCoverageComponent(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "明", "bright", "⿰日月", `{"type":"ideographic","hint":"sun+moon"}`, "日", "")
	seedHanziDef(t, s, "日", "sun; day")
	seedHanziDef(t, s, "月", "moon; month")
	seedHanziFull(t, s, "好", "good", "⿰女子", `{"type":"ideographic","hint":"woman+child"}`, "女", "")
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziDef(t, s, "子", "child; son")

	const userID = int64(2)
	// 5 pre-existing words use 明 (-> 日/月 at ~83% coverage), 1 uses 好 (-> 女/子
	// at ~17% coverage). Neither the 明/好 combo word about to be created is in
	// `words` yet, so the tally below is over these 6 pre-existing words only.
	for _, filler := range []string{"甲", "乙", "丙", "丁", "戊"} {
		seedZhWord(t, s, userID, "明"+filler)
	}
	seedZhWord(t, s, userID, "好己")

	setComponentThreshold(t, s, userID, 20) // % — excludes anything below 20%

	if err := s.InitComponentsForWord(ctx, userID, "明好", time.Now()); err != nil {
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

	if len(chars) != 2 || chars[0] != "日" || chars[1] != "月" {
		t.Errorf("want [日 月] (high-coverage components kept, low-coverage 女/子 excluded), got %v", chars)
	}
}

func TestGetComponentCoverage_SortingPctAndAlreadyTrained(t *testing.T) {
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
	// 日:2/3, 月:2/3, 女:1/3 (马 excluded — phonetic-only).

	// Mark 月 as already in training.
	s.InsertComponentProgressForTest(ctx, userID, "月", time.Now())

	items, total, err := s.GetComponentCoverage(ctx, userID)
	if err != nil {
		t.Fatalf("GetComponentCoverage: %v", err)
	}
	if total != 3 {
		t.Errorf("want total_words=3, got %d", total)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 components (日, 月, 女), got %d: %+v", len(items), items)
	}
	// Sorted by word_count desc, then character asc: 日(2), 月(2), 女(1).
	if items[0].Character != "日" || items[1].Character != "月" || items[2].Character != "女" {
		t.Errorf("want order [日 月 女], got [%s %s %s]", items[0].Character, items[1].Character, items[2].Character)
	}
	if items[0].WordCount != 2 || items[2].WordCount != 1 {
		t.Errorf("want word counts 2 and 1, got %d and %d", items[0].WordCount, items[2].WordCount)
	}
	wantPct := 2.0 / 3.0 * 100
	if diff := items[0].CoveragePct - wantPct; diff > 0.01 || diff < -0.01 {
		t.Errorf("want coverage_pct ~%.2f, got %v", wantPct, items[0].CoveragePct)
	}
	if !items[1].AlreadyTrained {
		t.Errorf("want 月 marked already_trained")
	}
	if items[0].AlreadyTrained || items[2].AlreadyTrained {
		t.Errorf("want 日 and 女 not already_trained")
	}
	if items[0].DefinitionEN != "sun; day" {
		t.Errorf("want definition_en %q for 日, got %q", "sun; day", items[0].DefinitionEN)
	}
}
