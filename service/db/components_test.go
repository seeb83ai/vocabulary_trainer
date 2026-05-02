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

func TestGetComponentList_IncludesPinyin(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziFull(t, s, "女", "woman", "", "", "", `["nǚ"]`)
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now().Add(time.Hour))

	items, _, err := s.GetComponentList(ctx, int64(2), "", 1, 50)
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

	items, _, err := s.GetComponentList(ctx, int64(2), "", 1, 50)
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

	items, _, err := s.GetComponentList(ctx, int64(2), "", 1, 50)
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
