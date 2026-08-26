package handlers_test

import (
	"net/http"
	"strings"
	"testing"
	"vocabulary_trainer/models"
)

func validSettingsPayload() map[string]string {
	return map[string]string{
		"primary_lang":         "en",
		"secondary_lang":       "de",
		"prog_new":             "zh_to_transl",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "zh_pinyin_to_transl",
		"new_word_mode_2":      "zh_to_transl",
	}
}

func TestGetSettings_Defaults(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var st models.UserSettings
	decodeJSON(t, rec, &st)

	if st.PrimaryLang != "en" {
		t.Errorf("want primary_lang=en, got %q", st.PrimaryLang)
	}
	if st.SecondaryLang != "de" {
		t.Errorf("want secondary_lang=de, got %q", st.SecondaryLang)
	}
	if st.ProgNew != "transl_to_zh" {
		t.Errorf("want prog_new=transl_to_zh, got %q", st.ProgNew)
	}
	if st.ProgTierLearning != "zh_pinyin_to_transl" {
		t.Errorf("want prog_tier_learning=zh_pinyin_to_transl, got %q", st.ProgTierLearning)
	}
	if st.ProgTierMastered != "random" {
		t.Errorf("want prog_tier_mastered=random, got %q", st.ProgTierMastered)
	}
	if st.NewWordMode2 != "zh_to_transl" {
		t.Errorf("want new_word_mode_2=zh_to_transl, got %q", st.NewWordMode2)
	}
	if st.DeeplKeySet {
		t.Error("want deepl_key_set=false by default")
	}
	if !st.NewWordRequireZh {
		t.Error("want new_word_require_zh=true by default")
	}
	if !st.NewWordRequireTrans {
		t.Error("want new_word_require_trans=true by default")
	}
	if !st.ExtendSessionWithExtraWords {
		t.Error("want extend_session_with_extra_words=true by default")
	}
}

// PutAPIKeys must reject a local LLM URL that points at an internal address
// (SSRF guard) before doing anything else.
func TestPutAPIKeys_RejectsInternalLLMURL(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	body := map[string]any{
		"llm_provider":  "local",
		"llm_local_url": "http://169.254.169.254/latest/meta-data/",
		"llm_key":       "x",
	}
	rec := do(t, r, http.MethodPut, "/api/settings/api-keys", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for internal llm_local_url, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "llm_local_url") {
		t.Errorf("want llm_local_url error, got %s", rec.Body.String())
	}
}

func TestPatchSettings_Valid(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":         "de",
		"secondary_lang":       "en",
		"prog_new":             "zh_to_transl",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "zh_pinyin_to_transl",
		"new_word_mode_2":      "zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify by reading back
	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.PrimaryLang != "de" {
		t.Errorf("want primary_lang=de after patch, got %q", st.PrimaryLang)
	}
	if st.SecondaryLang != "en" {
		t.Errorf("want secondary_lang=en after patch, got %q", st.SecondaryLang)
	}
	if st.ProgNew != "zh_to_transl" {
		t.Errorf("want prog_new=zh_to_transl after patch, got %q", st.ProgNew)
	}
	if st.NewWordMode1 != "zh_pinyin_to_transl" {
		t.Errorf("want new_word_mode_1=zh_pinyin_to_transl, got %q", st.NewWordMode1)
	}
}

func TestPatchSettings_ExtendSessionWithExtraWords_RoundTrip(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := baseSettingsPatch()
	payload["extend_session_with_extra_words"] = false
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.ExtendSessionWithExtraWords {
		t.Error("want extend_session_with_extra_words=false after patch")
	}
}

func TestPatchSettings_RandomModeRange_RoundTrip(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := baseSettingsPatch()
	payload["random_mode_range_transl_to_zh"] = "new,50-69"
	payload["random_mode_range_zh_to_transl"] = "off"
	payload["random_mode_range_zh_pinyin_to_transl"] = "new,70-84"
	payload["random_mode_range_zh_to_transl_no_sound"] = "50-69,85-100"
	payload["random_mode_range_voice_to_transl"] = "70-84,85-100"

	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.RandomModeRangeTranslToZh != "new,50-69" {
		t.Errorf("random_mode_range_transl_to_zh: want %q, got %q", "new,50-69", st.RandomModeRangeTranslToZh)
	}
	if st.RandomModeRangeZhToTransl != "off" {
		t.Errorf("random_mode_range_zh_to_transl: want %q, got %q", "off", st.RandomModeRangeZhToTransl)
	}
	if st.RandomModeRangeZhPinyinToTransl != "new,70-84" {
		t.Errorf("random_mode_range_zh_pinyin_to_transl: want %q, got %q", "new,70-84", st.RandomModeRangeZhPinyinToTransl)
	}
	if st.RandomModeRangeZhToTranslNoSound != "50-69,85-100" {
		t.Errorf("random_mode_range_zh_to_transl_no_sound: want %q, got %q", "50-69,85-100", st.RandomModeRangeZhToTranslNoSound)
	}
	if st.RandomModeRangeVoiceToTransl != "70-84,85-100" {
		t.Errorf("random_mode_range_voice_to_transl: want %q, got %q", "70-84,85-100", st.RandomModeRangeVoiceToTransl)
	}
}

func TestPatchSettings_RandomModeRange_InvalidFormatRejected(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := baseSettingsPatch()
	payload["random_mode_range_transl_to_zh"] = "bogus"

	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for malformed random_mode_range_transl_to_zh, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Turning every mode "off" leaves every bucket with zero eligible modes —
// the save must be rejected (400) and nothing must be persisted.
func TestPatchSettings_RandomModeRange_UncoveredBucketRejected(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// Confirm the pre-existing (default) value before the rejected attempt.
	before := do(t, r, http.MethodGet, "/api/settings", nil)
	var beforeSt models.UserSettings
	decodeJSON(t, before, &beforeSt)

	payload := baseSettingsPatch()
	payload["random_mode_range_transl_to_zh"] = "off"
	payload["random_mode_range_zh_to_transl"] = "off"
	payload["random_mode_range_zh_pinyin_to_transl"] = "off"
	payload["random_mode_range_zh_to_transl_no_sound"] = "off"
	payload["random_mode_range_voice_to_transl"] = "off"

	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when every bucket is left uncovered, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify nothing was persisted.
	after := do(t, r, http.MethodGet, "/api/settings", nil)
	var afterSt models.UserSettings
	decodeJSON(t, after, &afterSt)
	if afterSt.RandomModeRangeTranslToZh != beforeSt.RandomModeRangeTranslToZh {
		t.Errorf("rejected PATCH must not persist: random_mode_range_transl_to_zh changed from %q to %q",
			beforeSt.RandomModeRangeTranslToZh, afterSt.RandomModeRangeTranslToZh)
	}
}

// A narrow but non-empty per-bucket coverage (a partial ladder that still
// covers every bucket) must be accepted.
func TestPatchSettings_RandomModeRange_PartialCoverageAccepted(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := baseSettingsPatch()
	// Single mode spanning the full ladder covers every bucket on its own.
	payload["random_mode_range_transl_to_zh"] = "new,85-100"
	payload["random_mode_range_zh_to_transl"] = "off"
	payload["random_mode_range_zh_pinyin_to_transl"] = "off"
	payload["random_mode_range_zh_to_transl_no_sound"] = "off"
	payload["random_mode_range_voice_to_transl"] = "off"

	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for a config that still covers every bucket, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchTrainingFilters_Valid(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]any{
		"mode":       "cycle",
		"bucket":     "50-69",
		"langs":      []string{"de", "en"},
		"mnemonics":  false,
		"components": true,
		"tags":       []string{"HSK1"},
	}
	rec := do(t, r, http.MethodPatch, "/api/training-filters", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify via GET /api/settings
	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.TrainMode != "cycle" {
		t.Errorf("want train_mode=cycle, got %q", st.TrainMode)
	}
	if st.TrainBucket != "50-69" {
		t.Errorf("want train_bucket=50-69, got %q", st.TrainBucket)
	}
	if len(st.TrainLangs) != 2 || st.TrainLangs[0] != "de" {
		t.Errorf("want train_langs=[de en], got %v", st.TrainLangs)
	}
	if st.TrainMnemonics {
		t.Error("want train_mnemonics=false")
	}
	if !st.TrainComponents {
		t.Error("want train_components=true")
	}
	if len(st.TrainTags) != 1 || st.TrainTags[0] != "HSK1" {
		t.Errorf("want train_tags=[HSK1], got %v", st.TrainTags)
	}
}

func TestPatchTrainingFilters_ZhToTranslNoSoundAccepted(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]any{
		"mode":  "zh_to_transl_no_sound",
		"langs": []string{"en"},
	}
	rec := do(t, r, http.MethodPatch, "/api/training-filters", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.TrainMode != "zh_to_transl_no_sound" {
		t.Errorf("want train_mode=zh_to_transl_no_sound, got %q", st.TrainMode)
	}
}

func TestPatchTrainingFilters_InvalidMode(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]any{
		"mode":  "invalid_mode",
		"langs": []string{"en"},
	}
	rec := do(t, r, http.MethodPatch, "/api/training-filters", payload)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid mode, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSettings_NewWordRequire(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]interface{}{
		"primary_lang":           "en",
		"secondary_lang":         "de",
		"prog_new":               "transl_to_zh",
		"prog_tier_struggling":   "transl_to_zh",
		"prog_tier_learning":     "zh_pinyin_to_transl",
		"prog_tier_practicing":   "zh_to_transl",
		"prog_tier_mastered":     "random",
		"new_word_mode_0":        "transl_to_zh",
		"new_word_mode_1":        "transl_to_zh",
		"new_word_mode_2":        "zh_to_transl",
		"new_word_require_zh":    false,
		"new_word_require_trans": true,
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.NewWordRequireZh {
		t.Error("want new_word_require_zh=false after patch")
	}
	if !st.NewWordRequireTrans {
		t.Error("want new_word_require_trans=true after patch")
	}
}

func TestPatchSettings_ZhToTranslNoSoundAccepted(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]interface{}{
		"primary_lang":         "en",
		"secondary_lang":       "de",
		"prog_new":             "transl_to_zh",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_to_transl_no_sound",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "transl_to_zh",
		"new_word_mode_2":      "zh_to_transl_no_sound",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.ProgTierLearning != "zh_to_transl_no_sound" {
		t.Errorf("want prog_tier_learning=zh_to_transl_no_sound, got %q", st.ProgTierLearning)
	}
	if st.NewWordMode2 != "zh_to_transl_no_sound" {
		t.Errorf("want new_word_mode_2=zh_to_transl_no_sound, got %q", st.NewWordMode2)
	}
}

func TestPatchSettings_InvalidMode(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":         "en",
		"secondary_lang":       "de",
		"prog_new":             "invalid_mode",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "transl_to_zh",
		"new_word_mode_2":      "zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid mode, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSettings_SameLang(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":         "en",
		"secondary_lang":       "en", // same as primary — invalid
		"prog_new":             "transl_to_zh",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "transl_to_zh",
		"new_word_mode_2":      "zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 when primary=secondary lang, got %d", rec.Code)
	}
}

func TestPatchSettings_EmptySecondaryLang(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":         "en",
		"secondary_lang":       "", // no secondary — valid
		"prog_new":             "transl_to_zh",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "transl_to_zh",
		"new_word_mode_2":      "zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for empty secondary_lang, got %d: %s", rec.Code, rec.Body.String())
	}

	// Read back and verify secondary_lang is stored as empty
	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.SecondaryLang != "" {
		t.Errorf("want secondary_lang empty after patch, got %q", st.SecondaryLang)
	}
}

func TestSettingsCycleSequence(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	// Default cycle_sequence should be the canonical 3-step sequence.
	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET settings: want 200, got %d", rec.Code)
	}
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	want := "zh_pinyin_to_transl,transl_to_zh,zh_to_transl"
	if st.CycleSequence != want {
		t.Errorf("default cycle_sequence: want %q, got %q", want, st.CycleSequence)
	}

	// PATCH with a custom sequence.
	payload := map[string]string{
		"primary_lang":         "en",
		"secondary_lang":       "",
		"prog_new":             "transl_to_zh",
		"prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning":   "zh_pinyin_to_transl",
		"prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered":   "random",
		"new_word_mode_0":      "transl_to_zh",
		"new_word_mode_1":      "transl_to_zh",
		"new_word_mode_2":      "zh_to_transl",
		"cycle_sequence":       "transl_to_zh,zh_to_transl",
	}
	rec = do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Read back and verify.
	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st2 models.UserSettings
	decodeJSON(t, rec, &st2)
	if st2.CycleSequence != "transl_to_zh,zh_to_transl" {
		t.Errorf("after PATCH cycle_sequence: want %q, got %q", "transl_to_zh,zh_to_transl", st2.CycleSequence)
	}
}

func TestSettingsCycleSequence_InvalidMode(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":   "en",
		"secondary_lang": "",
		"prog_new":       "transl_to_zh", "prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning": "zh_pinyin_to_transl", "prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered": "random",
		"new_word_mode_0":    "transl_to_zh", "new_word_mode_1": "transl_to_zh", "new_word_mode_2": "zh_to_transl",
		"cycle_sequence": "transl_to_zh,invalid_mode",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid cycle mode, got %d", rec.Code)
	}
}

func TestSettingsCycleSequence_TooFewSteps(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":   "en",
		"secondary_lang": "",
		"prog_new":       "transl_to_zh", "prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning": "zh_pinyin_to_transl", "prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered": "random",
		"new_word_mode_0":    "transl_to_zh", "new_word_mode_1": "transl_to_zh", "new_word_mode_2": "zh_to_transl",
		"cycle_sequence": "transl_to_zh",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for cycle_sequence with only 1 step, got %d", rec.Code)
	}
}

func TestSettingsCycleSequence_TooManySteps(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]string{
		"primary_lang":   "en",
		"secondary_lang": "",
		"prog_new":       "transl_to_zh", "prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning": "zh_pinyin_to_transl", "prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered": "random",
		"new_word_mode_0":    "transl_to_zh", "new_word_mode_1": "transl_to_zh", "new_word_mode_2": "zh_to_transl",
		"cycle_sequence": "transl_to_zh,zh_to_transl,zh_pinyin_to_transl,mask_pinyin,transl_to_zh,zh_to_transl",
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for cycle_sequence with 6 steps, got %d", rec.Code)
	}
}

func TestSettingsCycleSequence_FiveSteps(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	fiveStep := "transl_to_zh,zh_to_transl,zh_pinyin_to_transl,mask_pinyin,transl_to_zh"
	payload := map[string]string{
		"primary_lang":   "en",
		"secondary_lang": "",
		"prog_new":       "transl_to_zh", "prog_tier_struggling": "transl_to_zh",
		"prog_tier_learning": "zh_pinyin_to_transl", "prog_tier_practicing": "zh_to_transl",
		"prog_tier_mastered": "random",
		"new_word_mode_0":    "transl_to_zh", "new_word_mode_1": "transl_to_zh", "new_word_mode_2": "zh_to_transl",
		"cycle_sequence": fiveStep,
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for 5-step cycle_sequence, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.CycleSequence != fiveStep {
		t.Errorf("after PATCH 5-step cycle_sequence: want %q, got %q", fiveStep, st.CycleSequence)
	}
}

func TestSettingsCycleAdvanceOnSuccessOnly_DefaultFalse(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET settings: want 200, got %d", rec.Code)
	}
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.CycleAdvanceOnSuccessOnly {
		t.Error("default cycle_advance_on_success_only: want false, got true")
	}
}

func TestSettingsCycleAdvanceOnSuccessOnly_RoundTrip(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	payload := map[string]interface{}{
		"primary_lang":                  "en",
		"secondary_lang":                "",
		"prog_new":                      "transl_to_zh",
		"prog_tier_struggling":          "transl_to_zh",
		"prog_tier_learning":            "zh_pinyin_to_transl",
		"prog_tier_practicing":          "zh_to_transl",
		"prog_tier_mastered":            "random",
		"new_word_mode_0":               "transl_to_zh",
		"new_word_mode_1":               "transl_to_zh",
		"new_word_mode_2":               "zh_to_transl",
		"cycle_sequence":                "zh_pinyin_to_transl,transl_to_zh,zh_to_transl",
		"cycle_advance_on_success_only": true,
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if !st.CycleAdvanceOnSuccessOnly {
		t.Error("after PATCH: want cycle_advance_on_success_only=true, got false")
	}
}

func TestSettingsPatchAcceptCorrectMode(t *testing.T) {
	r := newRouter(openTestDB(t))

	payload := validSettingsPayload()
	payload["accept_correct_mode"] = "always"
	rec := do(t, r, "PATCH", "/api/settings", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH settings: want 200, got %d: %s", rec.Code, rec.Body)
	}

	rec2 := do(t, r, "GET", "/api/settings", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET settings: want 200, got %d", rec2.Code)
	}
	var st models.UserSettings
	decodeJSON(t, rec2, &st)
	if st.AcceptCorrectMode != "always" {
		t.Errorf("AcceptCorrectMode: want %q, got %q", "always", st.AcceptCorrectMode)
	}
}

func TestSettingsPatchAcceptCorrectModeInvalid(t *testing.T) {
	r := newRouter(openTestDB(t))
	payload := validSettingsPayload()
	payload["accept_correct_mode"] = "banana"
	rec := do(t, r, "PATCH", "/api/settings", payload)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid accept_correct_mode, got %d", rec.Code)
	}
}

func TestGetSettings_DailyLearningDefaults(t *testing.T) {
	r := newRouter(openTestDB(t))

	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var st models.UserSettings
	decodeJSON(t, rec, &st)

	if st.MaxNewWordsPerDay < 1 {
		t.Errorf("want MaxNewWordsPerDay >= 1, got %d", st.MaxNewWordsPerDay)
	}
	if !st.SkipNewWordsVisible {
		t.Error("want SkipNewWordsVisible=true by default")
	}
	if st.BaselineDueTodayEnabled {
		t.Error("want BaselineDueTodayEnabled=false by default")
	}
	if st.BaselineDueTodayValue <= 0 {
		t.Errorf("want BaselineDueTodayValue > 0, got %d", st.BaselineDueTodayValue)
	}
	if st.BaselineStrugglingEnabled {
		t.Error("want BaselineStrugglingEnabled=false by default")
	}
	if st.BaselineStrugglingValue <= 0 {
		t.Errorf("want BaselineStrugglingValue > 0, got %d", st.BaselineStrugglingValue)
	}
	if st.BaselineLearningEnabled {
		t.Error("want BaselineLearningEnabled=false by default")
	}
	if st.BaselineLearningValue <= 0 {
		t.Errorf("want BaselineLearningValue > 0, got %d", st.BaselineLearningValue)
	}
	if st.BaselineNewBucketEnabled {
		t.Error("want BaselineNewBucketEnabled=false by default")
	}
	if st.BaselineNewBucketValue <= 0 {
		t.Errorf("want BaselineNewBucketValue > 0, got %d", st.BaselineNewBucketValue)
	}
}

func TestPatchSettings_DailyLearning(t *testing.T) {
	r := newRouter(openTestDB(t))

	payload := validSettingsPayload()
	// Overlay daily learning fields using a combined map
	type dailyPayload struct {
		PrimaryLang               string `json:"primary_lang"`
		SecondaryLang             string `json:"secondary_lang"`
		ProgNew                   string `json:"prog_new"`
		ProgTierStruggling        string `json:"prog_tier_struggling"`
		ProgTierLearning          string `json:"prog_tier_learning"`
		ProgTierPracticing        string `json:"prog_tier_practicing"`
		ProgTierMastered          string `json:"prog_tier_mastered"`
		NewWordMode0              string `json:"new_word_mode_0"`
		NewWordMode1              string `json:"new_word_mode_1"`
		NewWordMode2              string `json:"new_word_mode_2"`
		MaxNewWordsPerDay         int    `json:"max_new_words_per_day"`
		SkipNewWordsVisible       bool   `json:"skip_new_words_visible"`
		BaselineDueTodayEnabled   bool   `json:"baseline_due_today_enabled"`
		BaselineDueTodayValue     int    `json:"baseline_due_today_value"`
		BaselineStrugglingEnabled bool   `json:"baseline_struggling_enabled"`
		BaselineStrugglingValue   int    `json:"baseline_struggling_value"`
		BaselineLearningEnabled   bool   `json:"baseline_learning_enabled"`
		BaselineLearningValue     int    `json:"baseline_learning_value"`
		BaselineNewBucketEnabled  bool   `json:"baseline_new_bucket_enabled"`
		BaselineNewBucketValue    int    `json:"baseline_new_bucket_value"`
	}
	req := dailyPayload{
		PrimaryLang:               payload["primary_lang"],
		SecondaryLang:             payload["secondary_lang"],
		ProgNew:                   payload["prog_new"],
		ProgTierStruggling:        payload["prog_tier_struggling"],
		ProgTierLearning:          payload["prog_tier_learning"],
		ProgTierPracticing:        payload["prog_tier_practicing"],
		ProgTierMastered:          payload["prog_tier_mastered"],
		NewWordMode0:              payload["new_word_mode_0"],
		NewWordMode1:              payload["new_word_mode_1"],
		NewWordMode2:              payload["new_word_mode_2"],
		MaxNewWordsPerDay:         3,
		SkipNewWordsVisible:       false,
		BaselineDueTodayEnabled:   true,
		BaselineDueTodayValue:     15,
		BaselineStrugglingEnabled: true,
		BaselineStrugglingValue:   8,
		BaselineLearningEnabled:   false,
		BaselineLearningValue:     20,
		BaselineNewBucketEnabled:  true,
		BaselineNewBucketValue:    3,
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)

	if st.MaxNewWordsPerDay != 3 {
		t.Errorf("want MaxNewWordsPerDay=3, got %d", st.MaxNewWordsPerDay)
	}
	if st.SkipNewWordsVisible {
		t.Error("want SkipNewWordsVisible=false after patch")
	}
	if !st.BaselineDueTodayEnabled {
		t.Error("want BaselineDueTodayEnabled=true after patch")
	}
	if st.BaselineDueTodayValue != 15 {
		t.Errorf("want BaselineDueTodayValue=15, got %d", st.BaselineDueTodayValue)
	}
	if !st.BaselineStrugglingEnabled {
		t.Error("want BaselineStrugglingEnabled=true after patch")
	}
	if st.BaselineStrugglingValue != 8 {
		t.Errorf("want BaselineStrugglingValue=8, got %d", st.BaselineStrugglingValue)
	}
	if st.BaselineLearningEnabled {
		t.Error("want BaselineLearningEnabled=false after patch")
	}
	if !st.BaselineNewBucketEnabled {
		t.Error("want BaselineNewBucketEnabled=true after patch")
	}
	if st.BaselineNewBucketValue != 3 {
		t.Errorf("want BaselineNewBucketValue=3, got %d", st.BaselineNewBucketValue)
	}
}

func TestPatchSettings_BaselineNewBucketValue_Invalid(t *testing.T) {
	r := newRouter(openTestDB(t))

	payload := validSettingsPayload()
	type dailyPayload struct {
		PrimaryLang            string `json:"primary_lang"`
		SecondaryLang          string `json:"secondary_lang"`
		ProgNew                string `json:"prog_new"`
		ProgTierStruggling     string `json:"prog_tier_struggling"`
		ProgTierLearning       string `json:"prog_tier_learning"`
		ProgTierPracticing     string `json:"prog_tier_practicing"`
		ProgTierMastered       string `json:"prog_tier_mastered"`
		NewWordMode0           string `json:"new_word_mode_0"`
		NewWordMode1           string `json:"new_word_mode_1"`
		NewWordMode2           string `json:"new_word_mode_2"`
		BaselineNewBucketValue int    `json:"baseline_new_bucket_value"`
	}
	req := dailyPayload{
		PrimaryLang:            payload["primary_lang"],
		SecondaryLang:          payload["secondary_lang"],
		ProgNew:                payload["prog_new"],
		ProgTierStruggling:     payload["prog_tier_struggling"],
		ProgTierLearning:       payload["prog_tier_learning"],
		ProgTierPracticing:     payload["prog_tier_practicing"],
		ProgTierMastered:       payload["prog_tier_mastered"],
		NewWordMode0:           payload["new_word_mode_0"],
		NewWordMode1:           payload["new_word_mode_1"],
		NewWordMode2:           payload["new_word_mode_2"],
		BaselineNewBucketValue: -1,
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for negative baseline_new_bucket_value, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchSettings_MaxNewWordsPerDay_Invalid(t *testing.T) {
	r := newRouter(openTestDB(t))

	type payload struct {
		PrimaryLang        string `json:"primary_lang"`
		SecondaryLang      string `json:"secondary_lang"`
		ProgNew            string `json:"prog_new"`
		ProgTierStruggling string `json:"prog_tier_struggling"`
		ProgTierLearning   string `json:"prog_tier_learning"`
		ProgTierPracticing string `json:"prog_tier_practicing"`
		ProgTierMastered   string `json:"prog_tier_mastered"`
		NewWordMode0       string `json:"new_word_mode_0"`
		NewWordMode1       string `json:"new_word_mode_1"`
		NewWordMode2       string `json:"new_word_mode_2"`
		MaxNewWordsPerDay  int    `json:"max_new_words_per_day"`
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload{
		PrimaryLang:        "en",
		SecondaryLang:      "de",
		ProgNew:            "zh_to_transl",
		ProgTierStruggling: "transl_to_zh",
		ProgTierLearning:   "zh_pinyin_to_transl",
		ProgTierPracticing: "zh_to_transl",
		ProgTierMastered:   "random",
		NewWordMode0:       "transl_to_zh",
		NewWordMode1:       "zh_pinyin_to_transl",
		NewWordMode2:       "zh_to_transl",
		MaxNewWordsPerDay:  0,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for max_new_words_per_day=0, got %d", rec.Code)
	}
}

func TestGetSettings_CooldownDefault(t *testing.T) {
	r := newRouter(openTestDB(t))
	rec := do(t, r, http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.NewWordCooldownMinutes < 0 {
		t.Errorf("want NewWordCooldownMinutes >= 0, got %d", st.NewWordCooldownMinutes)
	}
}

func TestPatchSettings_Cooldown(t *testing.T) {
	r := newRouter(openTestDB(t))

	type payload struct {
		PrimaryLang            string `json:"primary_lang"`
		SecondaryLang          string `json:"secondary_lang"`
		ProgNew                string `json:"prog_new"`
		ProgTierStruggling     string `json:"prog_tier_struggling"`
		ProgTierLearning       string `json:"prog_tier_learning"`
		ProgTierPracticing     string `json:"prog_tier_practicing"`
		ProgTierMastered       string `json:"prog_tier_mastered"`
		NewWordMode0           string `json:"new_word_mode_0"`
		NewWordMode1           string `json:"new_word_mode_1"`
		NewWordMode2           string `json:"new_word_mode_2"`
		MaxNewWordsPerDay      int    `json:"max_new_words_per_day"`
		NewWordCooldownMinutes int    `json:"new_word_cooldown_minutes"`
	}
	rec := do(t, r, http.MethodPatch, "/api/settings", payload{
		PrimaryLang:            "en",
		SecondaryLang:          "de",
		ProgNew:                "zh_to_transl",
		ProgTierStruggling:     "transl_to_zh",
		ProgTierLearning:       "zh_pinyin_to_transl",
		ProgTierPracticing:     "zh_to_transl",
		ProgTierMastered:       "random",
		NewWordMode0:           "transl_to_zh",
		NewWordMode1:           "zh_pinyin_to_transl",
		NewWordMode2:           "zh_to_transl",
		MaxNewWordsPerDay:      5,
		NewWordCooldownMinutes: 30,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}

	rec = do(t, r, http.MethodGet, "/api/settings", nil)
	var st models.UserSettings
	decodeJSON(t, rec, &st)
	if st.NewWordCooldownMinutes != 30 {
		t.Errorf("want NewWordCooldownMinutes=30, got %d", st.NewWordCooldownMinutes)
	}
}

func TestSettingsPatch_GamificationFields(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	body := baseSettingsPatch()
	body["gamification_enabled"] = true
	body["gamification_frequency"] = 10
	rec := do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec2, &st)
	if st["gamification_enabled"] != true {
		t.Errorf("gamification_enabled: got %v", st["gamification_enabled"])
	}
	if st["gamification_frequency"].(float64) != 10 {
		t.Errorf("gamification_frequency: got %v", st["gamification_frequency"])
	}
}

// TestSettingsPatch_GamificationEnabled_OmittedPreservesExisting guards
// against a regression where saving a PATCH payload that doesn't include
// gamification_enabled (e.g. from the Daily Learning, Training Mode, Cycle
// Mode, Language, or Accept-as-correct save buttons in settings.js — none of
// which currently send this field) silently resets it to false, even though
// the user only intended to change a different section.
func TestSettingsPatch_GamificationEnabled_OmittedPreservesExisting(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	body := baseSettingsPatch()
	body["gamification_enabled"] = true
	rec := do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}

	// Save again without the field, as every non-Gamification settings card does.
	body2 := baseSettingsPatch()
	rec2 := do(t, r, "PATCH", "/api/settings", body2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec2.Code, rec2.Body.String())
	}

	rec3 := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec3, &st)
	if st["gamification_enabled"] != true {
		t.Errorf("want gamification_enabled preserved as true, got %v", st["gamification_enabled"])
	}
}

func TestSettingsPatch_GamificationFrequencyValidation(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	for _, freq := range []int{0, 1441} {
		body := baseSettingsPatch()
		body["gamification_frequency"] = freq
		rec := do(t, r, "PATCH", "/api/settings", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("frequency=%d: expected 400, got %d", freq, rec.Code)
		}
	}
}

func TestSettingsPatch_ComponentCoverageThreshold(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	body := baseSettingsPatch()
	body["component_coverage_threshold"] = 7.5
	rec := do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec2, &st)
	if st["component_coverage_threshold"].(float64) != 7.5 {
		t.Errorf("component_coverage_threshold: got %v", st["component_coverage_threshold"])
	}
}

func TestSettingsPatch_ComponentCoverageThresholdValidation(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	for _, v := range []float64{-1, 100.1, 150} {
		body := baseSettingsPatch()
		body["component_coverage_threshold"] = v
		rec := do(t, r, "PATCH", "/api/settings", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("threshold=%v: expected 400, got %d", v, rec.Code)
		}
	}
}

func TestSettingsPatch_ComponentCoverageThreshold_OmittedPreservesExisting(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)
	body := baseSettingsPatch()
	body["component_coverage_threshold"] = 5
	rec := do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}

	// Saving again without the field must preserve the previously-set value
	// rather than resetting it to 0 — the same nil-preserving pattern used by
	// max_new_words_per_day / gamification_frequency.
	body2 := baseSettingsPatch()
	rec2 := do(t, r, "PATCH", "/api/settings", body2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec2.Code, rec2.Body.String())
	}

	rec3 := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec3, &st)
	if st["component_coverage_threshold"].(float64) != 5 {
		t.Errorf("want threshold preserved at 5, got %v", st["component_coverage_threshold"])
	}
}

func TestSettingsPatch_BlurPinyin(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["blur_pinyin"] != false {
		t.Errorf("blur_pinyin: want false by default, got %v", st["blur_pinyin"])
	}

	body := baseSettingsPatch()
	body["blur_pinyin"] = true
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["blur_pinyin"] != true {
		t.Errorf("blur_pinyin: want true after update, got %v", st["blur_pinyin"])
	}
}

func TestSettingsPatch_NoAutoVoiceOnBlur(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["no_auto_voice_on_blur"] != false {
		t.Errorf("no_auto_voice_on_blur: want false by default, got %v", st["no_auto_voice_on_blur"])
	}

	body := baseSettingsPatch()
	body["no_auto_voice_on_blur"] = true
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["no_auto_voice_on_blur"] != true {
		t.Errorf("no_auto_voice_on_blur: want true after update, got %v", st["no_auto_voice_on_blur"])
	}
}

func TestSettingsPatch_SentenceBlank(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["sentence_blank_enabled"] != false {
		t.Errorf("sentence_blank_enabled: want false by default, got %v", st["sentence_blank_enabled"])
	}
	if st["sentence_blank_ratio"] != float64(20) {
		t.Errorf("sentence_blank_ratio: want 20 by default, got %v", st["sentence_blank_ratio"])
	}

	body := baseSettingsPatch()
	body["sentence_blank_enabled"] = true
	body["sentence_blank_ratio"] = 50
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["sentence_blank_enabled"] != true {
		t.Errorf("sentence_blank_enabled: want true after update, got %v", st["sentence_blank_enabled"])
	}
	if st["sentence_blank_ratio"] != float64(50) {
		t.Errorf("sentence_blank_ratio: want 50 after update, got %v", st["sentence_blank_ratio"])
	}
}

func TestSettingsPatch_SentenceBlankRatio_Invalid(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	body := baseSettingsPatch()
	body["sentence_blank_ratio"] = 101
	rec := do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for out-of-range ratio, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsPatch_CelebrateBucketChange(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["celebrate_bucket_change"] != false {
		t.Errorf("celebrate_bucket_change: want false by default, got %v", st["celebrate_bucket_change"])
	}

	body := baseSettingsPatch()
	body["celebrate_bucket_change"] = true
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["celebrate_bucket_change"] != true {
		t.Errorf("celebrate_bucket_change: want true after update, got %v", st["celebrate_bucket_change"])
	}
}

func TestSettingsPatch_RetypeOnWrong(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["retype_on_wrong"] != false {
		t.Errorf("retype_on_wrong: want false by default, got %v", st["retype_on_wrong"])
	}

	body := baseSettingsPatch()
	body["retype_on_wrong"] = true
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["retype_on_wrong"] != true {
		t.Errorf("retype_on_wrong: want true after update, got %v", st["retype_on_wrong"])
	}
}

func TestSettingsPatch_ShowImagesWithChineseText(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["show_images_with_chinese_text"] != false {
		t.Errorf("show_images_with_chinese_text: want false by default, got %v", st["show_images_with_chinese_text"])
	}

	body := baseSettingsPatch()
	body["show_images_with_chinese_text"] = true
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["show_images_with_chinese_text"] != true {
		t.Errorf("show_images_with_chinese_text: want true after update, got %v", st["show_images_with_chinese_text"])
	}
}

func TestSettingsPatch_VoiceUnavailable(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	if st["voice_unavailable"] != false {
		t.Errorf("voice_unavailable: want false by default, got %v", st["voice_unavailable"])
	}

	body := baseSettingsPatch()
	body["voice_unavailable"] = true
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["voice_unavailable"] != true {
		t.Errorf("voice_unavailable: want true after update, got %v", st["voice_unavailable"])
	}
}

func TestSettingsPatch_GameModeFields(t *testing.T) {
	s := openTestDB(t)
	r := newRouter(s)

	rec := do(t, r, "GET", "/api/settings", nil)
	var st map[string]any
	decodeJSON(t, rec, &st)
	for _, field := range []string{"game_mode_mismatch", "game_mode_newest", "game_mode_hardest", "game_mode_last_mistakes"} {
		if st[field] != true {
			t.Errorf("%s: want true by default, got %v", field, st[field])
		}
	}

	body := baseSettingsPatch()
	body["game_mode_mismatch"] = true
	body["game_mode_newest"] = false
	body["game_mode_hardest"] = true
	body["game_mode_last_mistakes"] = false
	rec = do(t, r, "PATCH", "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d: %s", rec.Code, rec.Body.String())
	}
	rec2 := do(t, r, "GET", "/api/settings", nil)
	decodeJSON(t, rec2, &st)
	if st["game_mode_mismatch"] != true || st["game_mode_newest"] != false ||
		st["game_mode_hardest"] != true || st["game_mode_last_mistakes"] != false {
		t.Errorf("game mode fields after update: %+v", st)
	}
}
