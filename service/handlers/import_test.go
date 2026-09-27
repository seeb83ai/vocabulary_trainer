package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"vocabulary_trainer/models"
)

func TestImportSourceTags_ReturnsTags(t *testing.T) {
	s := openTestDB(t)
	// User 1 is a zh-words-and-tags-only template; translations for import
	// come from cedict_entries, not from translations stored on user 1.
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"HSK1"})
	seedWordFull(t, s, 1, "谢谢", "xiè xie", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")
	seedCedictEntry(t, s, "谢谢", "en", "thank you")
	// User 2 has a different tag — should not appear
	seedWordFull(t, s, 2, "再见", "zài jiàn", []string{"goodbye"}, nil, []string{"HSK2"})

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/import/source-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 1 || tags[0].Name != "HSK1" {
		t.Errorf("want [{Name:HSK1 ...}], got %v", tags)
	}
	if !tags[0].Importable {
		t.Errorf("expected importable=true by default")
	}
	hasEn := false
	for _, l := range tags[0].AvailableLangs {
		if l == "en" {
			hasEn = true
		}
	}
	if !hasEn {
		t.Errorf("expected available_langs to include 'en' for tag with EN translations")
	}
	for _, l := range tags[0].AvailableLangs {
		if l == "de" {
			t.Errorf("expected 'de' not in available_langs when no DE translations")
		}
	}
}

func TestImportSourceTags_WithDeFlag(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"greetings"})
	seedWordFull(t, s, 1, "再见", "zài jiàn", nil, nil, []string{"greetings"})
	seedCedictEntry(t, s, "你好", "en", "hello")
	seedCedictEntry(t, s, "你好", "de", "hallo")
	seedCedictEntry(t, s, "再见", "en", "goodbye")

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/import/source-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 1 {
		t.Fatalf("want 1 tag, got %d", len(tags))
	}
	hasEn, hasDe := false, false
	for _, l := range tags[0].AvailableLangs {
		if l == "en" {
			hasEn = true
		}
		if l == "de" {
			hasDe = true
		}
	}
	if !hasEn {
		t.Errorf("expected available_langs to include 'en'")
	}
	if !hasDe {
		t.Errorf("expected available_langs to include 'de' when at least one word has DE")
	}
}

func TestImportSourceTags_EmptyWhenNoWords(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/import/source-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 0 {
		t.Errorf("want empty, got %v", tags)
	}
}

func TestImportSourceTags_HidesNonImportable(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"public"})
	seedWordFull(t, s, 1, "秘密", "", nil, nil, []string{"private"})
	seedCedictEntry(t, s, "你好", "en", "hello")
	seedCedictEntry(t, s, "秘密", "en", "secret")
	// Mark private tag as not importable.
	if err := s.UpsertTagMeta(context.Background(), int64(1), "private", "", false); err != nil {
		t.Fatalf("UpsertTagMeta: %v", err)
	}

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/import/source-tags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var tags []models.TagDetail
	decodeJSON(t, rec, &tags)
	if len(tags) != 1 || tags[0].Name != "public" {
		t.Errorf("want only [public], got %v", tags)
	}
}

func TestImportPreview_ValidTag(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"HSK1"})
	seedWordFull(t, s, 1, "谢谢", "xiè xie", nil, nil, []string{"HSK1"})
	seedWordFull(t, s, 1, "再见", "zài jiàn", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")
	seedCedictEntry(t, s, "你好", "de", "hallo")
	seedCedictEntry(t, s, "谢谢", "en", "thank you")
	seedCedictEntry(t, s, "再见", "en", "goodbye")

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/import/preview?tag=HSK1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Tag            string         `json:"tag"`
		Total          int            `json:"total"`
		AvailableLangs map[string]int `json:"available_langs"`
		Examples       []struct {
			ZhText       string              `json:"zh_text"`
			Pinyin       string              `json:"pinyin"`
			Translations map[string][]string `json:"translations"`
		} `json:"examples"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Tag != "HSK1" {
		t.Errorf("want tag HSK1, got %q", resp.Tag)
	}
	if resp.Total != 3 {
		t.Errorf("want total 3, got %d", resp.Total)
	}
	if resp.AvailableLangs["en"] != 3 {
		t.Errorf("want available_langs[en]=3, got %d", resp.AvailableLangs["en"])
	}
	if resp.AvailableLangs["de"] != 1 {
		t.Errorf("want available_langs[de]=1, got %d", resp.AvailableLangs["de"])
	}
	if len(resp.Examples) != 3 {
		t.Errorf("want 3 examples, got %d", len(resp.Examples))
	}
	if len(resp.Examples) > 50 {
		t.Errorf("want at most 50 examples, got %d", len(resp.Examples))
	}
	if resp.Examples[0].ZhText == "" {
		t.Error("expected non-empty zh_text in first example")
	}
	if len(resp.Examples[0].Translations["en"]) == 0 {
		t.Error("expected en translations in first example")
	}
}

func TestImportPreview_MultipleTagsCountsUniqueWords(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "一", "yī", nil, nil, []string{"hsk2-1", "hsk3-1"})
	seedWordFull(t, s, 1, "人", "rén", nil, nil, []string{"hsk3-1"})
	seedWordFull(t, s, 1, "时间", "shí jiān", nil, nil, []string{"hsk2-1"})
	seedWordFull(t, s, 1, "已经", "yǐ jīng", nil, nil, []string{"hsk3-3"})
	seedCedictEntry(t, s, "一", "en", "one")

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/import/preview?tag=hsk2-1&tag=hsk3-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Total int `json:"total"`
	}
	decodeJSON(t, rec, &resp)
	// 一 is in both lists but counts once.
	if resp.Total != 3 {
		t.Errorf("want total 3, got %d", resp.Total)
	}
}

func TestImportPreview_UnknownTag(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/import/preview?tag=nonexistent", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Total int `json:"total"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Total != 0 {
		t.Errorf("want total 0, got %d", resp.Total)
	}
}

func TestImportPreview_MissingTag(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "GET", "/api/import/preview", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestImport_Basic(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"HSK1"})
	seedWordFull(t, s, 1, "谢谢", "xiè xie", nil, nil, []string{"HSK1"})
	seedWordFull(t, s, 1, "再见", "zài jiàn", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")
	seedCedictEntry(t, s, "谢谢", "en", "thank you")
	seedCedictEntry(t, s, "再见", "en", "goodbye")

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{"HSK1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Imported int `json:"imported"`
		Skipped  int `json:"skipped"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Imported != 3 {
		t.Errorf("want imported=3, got %d", resp.Imported)
	}
	if resp.Skipped != 0 {
		t.Errorf("want skipped=0, got %d", resp.Skipped)
	}

	// Verify words now exist for user 2
	listRec := do(t, r, "GET", "/api/words/?tags=HSK1", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d: %s", listRec.Code, listRec.Body)
	}
	var listResp struct {
		Total int `json:"total"`
	}
	decodeJSON(t, listRec, &listResp)
	if listResp.Total != 3 {
		t.Errorf("want 3 words in user list, got %d", listResp.Total)
	}
}

func TestImport_ExistingWordGetsImportTags(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"hsk3-1"})
	seedWordFull(t, s, 1, "再见", "zài jiàn", nil, nil, []string{"hsk3-1"})
	seedCedictEntry(t, s, "你好", "en", "hello")
	seedCedictEntry(t, s, "你好", "en", "hi")
	seedCedictEntry(t, s, "再见", "en", "goodbye")
	// User 2 already has 你好 from the HSK 2.0 list.
	seedWordFull(t, s, 2, "你好", "nǐ hǎo", []string{"hello"}, nil, []string{"hsk2-1"})

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":          "hsk3-1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{"hsk3-1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Imported int `json:"imported"`
		Tagged   int `json:"tagged"`
		Skipped  int `json:"skipped"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Imported != 1 || resp.Tagged != 1 || resp.Skipped != 0 {
		t.Errorf("want imported=1 tagged=1 skipped=0, got %+v", resp)
	}

	listRec := do(t, r, "GET", "/api/words/?tags=hsk2-1", nil)
	var listResp struct {
		Total int `json:"total"`
		Words []struct {
			ZhText       string              `json:"zh_text"`
			Tags         []string            `json:"tags"`
			Translations map[string][]string `json:"translations"`
		} `json:"words"`
	}
	decodeJSON(t, listRec, &listResp)
	if listResp.Total != 1 {
		t.Fatalf("want the existing word only once, got %d", listResp.Total)
	}
	w := listResp.Words[0]
	if len(w.Tags) != 2 || w.Tags[0] != "hsk2-1" || w.Tags[1] != "hsk3-1" {
		t.Errorf("want tags [hsk2-1 hsk3-1], got %v", w.Tags)
	}
	// The user's own translations stay as they are.
	if en := w.Translations["en"]; len(en) != 1 || en[0] != "hello" {
		t.Errorf("want translations unchanged [hello], got %v", en)
	}
}

func TestImport_ExistingWordWithoutApplyTagsIsSkipped(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"hsk3-1"})
	seedCedictEntry(t, s, "你好", "en", "hello")
	seedWordFull(t, s, 2, "你好", "nǐ hǎo", []string{"hello"}, nil, nil)

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":        "hsk3-1",
		"apply_tags": []string{},
	})
	var resp struct {
		Imported int `json:"imported"`
		Tagged   int `json:"tagged"`
		Skipped  int `json:"skipped"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Imported != 0 || resp.Tagged != 0 || resp.Skipped != 1 {
		t.Errorf("want imported=0 tagged=0 skipped=1, got %+v", resp)
	}
}

func TestImport_DeFlag(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")
	seedCedictEntry(t, s, "你好", "de", "Hallo")

	r := newRouter(s)
	// Import with DE
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en", "de"},
		"apply_tags":   []string{"HSK1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Imported int `json:"imported"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Imported != 1 {
		t.Fatalf("want imported=1, got %d", resp.Imported)
	}

	// Fetch the word and verify DE translation is present
	listRec := do(t, r, "GET", "/api/words/?tags=HSK1", nil)
	var listResp struct {
		Words []struct {
			Translations map[string][]string `json:"translations"`
		} `json:"words"`
	}
	decodeJSON(t, listRec, &listResp)
	if len(listResp.Words) == 0 {
		t.Fatal("no words returned")
	}
	if len(listResp.Words[0].Translations["de"]) == 0 {
		t.Error("expected DE translations to be imported")
	}
}

func TestImport_DeFlagFalse(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")
	seedCedictEntry(t, s, "你好", "de", "Hallo")

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	listRec := do(t, r, "GET", "/api/words/", nil)
	var listResp struct {
		Words []struct {
			Translations map[string][]string `json:"translations"`
		} `json:"words"`
	}
	decodeJSON(t, listRec, &listResp)
	if len(listResp.Words) == 0 {
		t.Fatal("no words returned")
	}
	if len(listResp.Words[0].Translations["de"]) != 0 {
		t.Errorf("expected no DE translations, got %v", listResp.Words[0].Translations["de"])
	}
}

func TestImport_ApplyCustomTags(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{"HSK1", "my-review"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	// Verify both tags are on the imported word
	listRec := do(t, r, "GET", "/api/words/?tags=my-review", nil)
	var listResp struct {
		Total int `json:"total"`
	}
	decodeJSON(t, listRec, &listResp)
	if listResp.Total != 1 {
		t.Errorf("want 1 word tagged my-review, got %d", listResp.Total)
	}
}

func TestImport_MissingTag(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, "POST", "/api/import", map[string]any{
		"import_langs": []string{"en"},
		"apply_tags":   []string{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}
