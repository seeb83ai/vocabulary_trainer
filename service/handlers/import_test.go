package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/models"
)

type importJobResp struct {
	ID       int64  `json:"id"`
	Status   string `json:"status"`
	Total    int    `json:"total"`
	Done     int    `json:"done"`
	Imported int    `json:"imported"`
	Tagged   int    `json:"tagged"`
	Skipped  int    `json:"skipped"`
	Error    string `json:"error"`
}

// runImport posts an import, lets the worker process the queued job to the
// end (the background loop does not run in tests) and returns the finished job.
func runImport(t *testing.T, s *db.Store, r http.Handler, body map[string]any) importJobResp {
	t.Helper()
	rec := do(t, r, "POST", "/api/import", body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", rec.Code, rec.Body)
	}
	var queued importJobResp
	decodeJSON(t, rec, &queued)
	if err := handlers.NewImportWorker(s).RunPending(context.Background()); err != nil {
		t.Fatalf("RunPending: %v", err)
	}
	jobRec := do(t, r, "GET", fmt.Sprintf("/api/import/jobs/%d", queued.ID), nil)
	if jobRec.Code != http.StatusOK {
		t.Fatalf("get job: want 200, got %d: %s", jobRec.Code, jobRec.Body)
	}
	var job importJobResp
	decodeJSON(t, jobRec, &job)
	if job.Status != "done" {
		t.Fatalf("job status = %q (error %q), want done", job.Status, job.Error)
	}
	return job
}

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
	resp := runImport(t, s, r, map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{"HSK1"},
	})
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
	resp := runImport(t, s, r, map[string]any{
		"tag":          "hsk3-1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{"hsk3-1"},
	})
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
	resp := runImport(t, s, r, map[string]any{
		"tag":        "hsk3-1",
		"apply_tags": []string{},
	})
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
	resp := runImport(t, s, r, map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en", "de"},
		"apply_tags":   []string{"HSK1"},
	})
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
	runImport(t, s, r, map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{},
	})

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
	runImport(t, s, r, map[string]any{
		"tag":          "HSK1",
		"import_langs": []string{"en"},
		"apply_tags":   []string{"HSK1", "my-review"},
	})

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

func TestImport_KnownModeHidesWordsFromQuiz(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")

	r := newRouter(s)
	resp := runImport(t, s, r, map[string]any{
		"tag": "HSK1", "import_langs": []string{"en"}, "apply_tags": []string{"HSK1"}, "import_mode": "known",
	})
	if resp.Imported != 1 {
		t.Fatalf("want imported=1, got %d", resp.Imported)
	}
	if rec := do(t, r, "GET", "/api/quiz/next", nil); rec.Code != http.StatusNotFound {
		t.Errorf("known words must not be quizzed, got %d: %s", rec.Code, rec.Body)
	}
}

func TestImport_ReviewModeSkipsNewWordIntro(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")

	r := newRouter(s)
	runImport(t, s, r, map[string]any{
		"tag": "HSK1", "import_langs": []string{"en"}, "apply_tags": []string{"HSK1"}, "import_mode": "review",
	})
	rec := do(t, r, "GET", "/api/quiz/next", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("review word must be due, got %d: %s", rec.Code, rec.Body)
	}
	var card struct {
		Mode string `json:"mode"`
	}
	decodeJSON(t, rec, &card)
	if card.Mode == "new_word" {
		t.Errorf("review words must skip the new-word introduction")
	}
}

func TestImport_IncludeModeLeavesWordsUnseen(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")

	r := newRouter(s)
	runImport(t, s, r, map[string]any{
		"tag": "HSK1", "import_langs": []string{"en"}, "import_mode": "include",
	})
	rec := do(t, r, "GET", "/api/quiz/next", nil)
	var card struct {
		Mode string `json:"mode"`
	}
	decodeJSON(t, rec, &card)
	if card.Mode != "new_word" {
		t.Errorf("included words start as new words, got mode %q", card.Mode)
	}
}

func TestImport_UnknownModeRejected(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{"tag": "HSK1", "import_mode": "bogus"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestImportPreview_MatchAllCountsWordsWithEveryTag(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "苹果", "píng guǒ", nil, nil, []string{"hsk3-1", "topic-food"})
	seedWordFull(t, s, 1, "人", "rén", nil, nil, []string{"hsk3-1"})
	seedWordFull(t, s, 1, "面包", "miàn bāo", nil, nil, []string{"topic-food"})
	seedCedictEntry(t, s, "苹果", "en", "apple")

	r := newRouter(s)
	rec := do(t, r, "GET", "/api/import/preview?tag=hsk3-1&tag=topic-food&match=all", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Total    int `json:"total"`
		Examples []struct {
			ZhText string `json:"zh_text"`
		} `json:"examples"`
	}
	decodeJSON(t, rec, &resp)
	if resp.Total != 1 || len(resp.Examples) != 1 || resp.Examples[0].ZhText != "苹果" {
		t.Errorf("match=all: want only 苹果, got total=%d examples=%v", resp.Total, resp.Examples)
	}
}

func TestImport_AndTagsImportsOnlyWordsWithEveryTag(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "苹果", "píng guǒ", nil, nil, []string{"hsk3-1", "topic-food"})
	seedWordFull(t, s, 1, "人", "rén", nil, nil, []string{"hsk3-1"})
	seedWordFull(t, s, 1, "面包", "miàn bāo", nil, nil, []string{"topic-food"})
	seedCedictEntry(t, s, "苹果", "en", "apple")
	seedCedictEntry(t, s, "人", "en", "person")
	seedCedictEntry(t, s, "面包", "en", "bread")

	r := newRouter(s)
	resp := runImport(t, s, r, map[string]any{
		"tag": "hsk3-1", "and_tags": []string{"topic-food"},
		"import_langs": []string{"en"}, "apply_tags": []string{"hsk3-1", "topic-food"},
	})
	if resp.Imported != 1 {
		t.Errorf("want imported=1, got %d", resp.Imported)
	}
	listRec := do(t, r, "GET", "/api/words/?per_page=50", nil)
	var list struct {
		Words []struct {
			ZhText string   `json:"zh_text"`
			Tags   []string `json:"tags"`
		} `json:"words"`
	}
	decodeJSON(t, listRec, &list)
	if len(list.Words) != 1 || list.Words[0].ZhText != "苹果" || len(list.Words[0].Tags) != 2 {
		t.Errorf("want only 苹果 tagged with both lists, got %+v", list.Words)
	}
}

func TestImport_ReturnsQueuedJobBeforeAnyWordIsImported(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "nǐ hǎo", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")

	r := newRouter(s)
	rec := do(t, r, "POST", "/api/import", map[string]any{"tag": "HSK1", "import_langs": []string{"en"}})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", rec.Code, rec.Body)
	}
	var job importJobResp
	decodeJSON(t, rec, &job)
	if job.ID == 0 || job.Status != "queued" {
		t.Errorf("job = %+v, want an id and status queued", job)
	}

	listRec := do(t, r, "GET", "/api/words/?tags=HSK1", nil)
	var listResp struct {
		Total int `json:"total"`
	}
	decodeJSON(t, listRec, &listResp)
	if listResp.Total != 0 {
		t.Errorf("no word may exist before the worker runs, got %d", listResp.Total)
	}
}

func TestImport_SecondRequestForSameListReusesActiveJob(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	var first, second importJobResp
	decodeJSON(t, do(t, r, "POST", "/api/import", map[string]any{"tag": "HSK1"}), &first)
	decodeJSON(t, do(t, r, "POST", "/api/import", map[string]any{"tag": "HSK1"}), &second)
	if first.ID == 0 || first.ID != second.ID {
		t.Errorf("job ids = %d, %d, want the same active job", first.ID, second.ID)
	}
}

func TestImportJob_ReportsProgressCounters(t *testing.T) {
	s := openTestDB(t)
	for _, w := range []string{"甲", "乙", "丙"} {
		seedWordFull(t, s, 1, w, "", nil, nil, []string{"HSK1"})
	}
	seedCedictEntry(t, s, "甲", "en", "a")
	seedCedictEntry(t, s, "乙", "en", "b")
	// 丙 has no dictionary entry, so it is skipped.

	job := runImport(t, s, newRouter(s), map[string]any{"tag": "HSK1", "import_langs": []string{"en"}})
	if job.Total != 3 || job.Done != 3 || job.Imported != 2 || job.Skipped != 1 {
		t.Errorf("job = %+v, want total=3 done=3 imported=2 skipped=1", job)
	}
}

func TestImportJob_IsHiddenFromOtherUsers(t *testing.T) {
	s := openTestDB(t)
	var job importJobResp
	decodeJSON(t, do(t, newRouter(s), "POST", "/api/import", map[string]any{"tag": "HSK1"}), &job)

	rec := do(t, newRouterWithUserID(s, 1), "GET", fmt.Sprintf("/api/import/jobs/%d", job.ID), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for another user's job, got %d", rec.Code)
	}
	rec = do(t, newRouter(s), "GET", "/api/import/jobs/999999", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for an unknown job, got %d", rec.Code)
	}
	rec = do(t, newRouter(s), "GET", "/api/import/jobs/abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for a malformed id, got %d", rec.Code)
	}
}

func TestImportJobs_ListsOnlyActiveJobsOfTheUser(t *testing.T) {
	s := openTestDB(t)
	seedWordFull(t, s, 1, "你好", "", nil, nil, []string{"HSK1"})
	seedCedictEntry(t, s, "你好", "en", "hello")
	r := newRouter(s)
	runImport(t, s, r, map[string]any{"tag": "HSK1", "import_langs": []string{"en"}}) // finished
	var pending importJobResp
	decodeJSON(t, do(t, r, "POST", "/api/import", map[string]any{"tag": "HSK2"}), &pending)
	if _, err := s.CreateImportJob(context.Background(), 1, models.ImportJob{Tag: "HSK3"}); err != nil {
		t.Fatal(err)
	}

	rec := do(t, r, "GET", "/api/import/jobs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var jobs []importJobResp
	decodeJSON(t, rec, &jobs)
	if len(jobs) != 1 || jobs[0].ID != pending.ID {
		t.Errorf("active jobs = %+v, want only job %d", jobs, pending.ID)
	}
}

func TestImportWorker_ResumesInterruptedJobWithoutDuplicates(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	for _, w := range []string{"甲", "乙"} {
		seedWordFull(t, s, 1, w, "", nil, nil, []string{"HSK1"})
		seedCedictEntry(t, s, w, "en", "gloss "+w)
	}
	// The server stopped after 甲 was imported; the job is still "running".
	seedWordFull(t, s, 2, "甲", "", []string{"gloss 甲"}, nil, []string{"HSK1"})
	job, err := s.CreateImportJob(ctx, 2, models.ImportJob{Tag: "HSK1", ImportLangs: []string{"en"}, ApplyTags: []string{"HSK1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StartImportJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}

	if err := handlers.NewImportWorker(s).RunPending(ctx); err != nil {
		t.Fatalf("RunPending: %v", err)
	}

	got, _ := s.GetImportJob(ctx, 2, job.ID)
	if got.Status != "done" || got.Imported != 1 {
		t.Errorf("job = %+v, want done with the one missing word imported", got)
	}
	words, total, err := s.GetWords(ctx, 2, "", 1, 0, "", "", []string{"HSK1"}, false, false, "", "", "")
	if err != nil || total != 2 {
		t.Fatalf("user words = %d (%v), want 2 without duplicates", len(words), err)
	}
}

func TestImportWorker_ImportsWordsInSourceOrder(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	order := []string{"丙", "甲", "乙"}
	for _, w := range order {
		seedWordFull(t, s, 1, w, "", nil, nil, []string{"HSK1"})
		seedCedictEntry(t, s, w, "en", "gloss "+w)
	}
	runImport(t, s, newRouter(s), map[string]any{"tag": "HSK1", "import_langs": []string{"en"}})

	var prev int64
	for _, w := range order {
		id, err := s.GetWordIDByZhText(ctx, 2, w)
		if err != nil {
			t.Fatalf("GetWordIDByZhText %q: %v", w, err)
		}
		if id <= prev {
			t.Errorf("%q was imported out of source order (id %d after %d)", w, id, prev)
		}
		prev = id
	}
}
