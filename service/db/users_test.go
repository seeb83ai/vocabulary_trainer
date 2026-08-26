package db

import (
	"context"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

func TestCreateUser_ReturnsID(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateUser(context.Background(), "testuser@example.com", "hash", "token123", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Errorf("expected positive user ID, got %d", id)
	}
}

func TestCreateUser_EmailNotVerified(t *testing.T) {
	s := openTestDB(t)
	_, err := s.CreateUser(context.Background(), "unverified@example.com", "hash", "tok", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.GetUserByEmail(context.Background(), "unverified@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil {
		t.Fatal("user not found after creation")
	}
	if user.EmailVerified {
		t.Error("new user should not be email_verified")
	}
}

func TestGetUserByID_Found(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateUser(context.Background(), "byid@example.com", "hash", "tok2", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.GetUserByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.Email != "byid@example.com" {
		t.Errorf("email: want byid@example.com, got %q", user.Email)
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	s := openTestDB(t)
	user, err := s.GetUserByID(context.Background(), 99999)
	if err != nil {
		t.Fatal(err)
	}
	if user != nil {
		t.Error("expected nil for missing user ID")
	}
}

func TestSetUserEmailVerified_OK(t *testing.T) {
	s := openTestDB(t)
	token := "validtoken12345678901234567890ab"
	_, err := s.CreateUser(context.Background(), "verify@example.com", "hash", token, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	user, err := s.SetUserEmailVerified(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if user == nil {
		t.Fatal("expected user after verification, got nil")
	}
	if !user.EmailVerified {
		t.Error("user should be email_verified after verification")
	}

	// Token must be consumed — second call returns nil
	user2, err := s.SetUserEmailVerified(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if user2 != nil {
		t.Error("second verification with same token should return nil")
	}
}

func TestSetUserEmailVerified_UnknownToken(t *testing.T) {
	s := openTestDB(t)
	user, err := s.SetUserEmailVerified(context.Background(), "nosuchtoken")
	if err != nil {
		t.Fatal(err)
	}
	if user != nil {
		t.Error("expected nil for unknown token")
	}
}

func TestSetUserEmailVerified_ExpiredToken(t *testing.T) {
	s := openTestDB(t)
	token := "expiredtoken1234567890123456789"
	_, err := s.CreateUser(context.Background(), "expired@example.com", "hash", token, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	user, err := s.SetUserEmailVerified(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if user != nil {
		t.Error("expected nil for expired token")
	}
}

func TestUpdateUserPassword_OK(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateUser(context.Background(), "pwchange@example.com", "oldhash", "tok3", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateUserPassword(context.Background(), id, "newhash"); err != nil {
		t.Fatal(err)
	}

	user, err := s.GetUserByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if user.PasswordHash != "newhash" {
		t.Errorf("expected newhash, got %q", user.PasswordHash)
	}
}

func TestGetUserRole_SeedAdmin(t *testing.T) {
	s := openTestDB(t)
	role, err := s.GetUserRole(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if role != "admin" {
		t.Errorf("user 1: want role admin, got %q", role)
	}
}

func TestGetUserRole_SeedPlus(t *testing.T) {
	s := openTestDB(t)
	role, err := s.GetUserRole(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if role != "plus" {
		t.Errorf("user 2: want role plus, got %q", role)
	}
}

func TestGetUserRole_NewUserDefaultsFree(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateUser(context.Background(), "new@example.com", "hash", "tok-new", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.GetUserRole(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if role != "free" {
		t.Errorf("new user: want role free, got %q", role)
	}
}

func TestGetUserRole_NotFound(t *testing.T) {
	s := openTestDB(t)
	role, err := s.GetUserRole(context.Background(), 99999)
	if err != nil {
		t.Fatal(err)
	}
	if role != "free" {
		t.Errorf("unknown user: want free, got %q", role)
	}
}

func TestGetUserByEmail_IncludesRole(t *testing.T) {
	s := openTestDB(t)
	user, err := s.GetUserByEmail(context.Background(), "admin@example.de")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil {
		t.Fatal("admin user not found")
	}
	if user.Role != "admin" {
		t.Errorf("admin user: want role admin, got %q", user.Role)
	}
}

func TestGetUserSettings_Defaults(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	const userID = int64(2)

	st, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	if st.PrimaryLang != "en" {
		t.Errorf("want primary_lang=en, got %q", st.PrimaryLang)
	}
	if st.SecondaryLang != "de" {
		t.Errorf("want secondary_lang=de, got %q", st.SecondaryLang)
	}
	if st.ProgNew != "transl_to_zh" {
		t.Errorf("want prog_new=transl_to_zh, got %q", st.ProgNew)
	}
	if st.ProgTierStruggling != "transl_to_zh" {
		t.Errorf("want prog_tier_struggling=transl_to_zh, got %q", st.ProgTierStruggling)
	}
	if st.ProgTierLearning != "zh_pinyin_to_transl" {
		t.Errorf("want prog_tier_learning=zh_pinyin_to_transl, got %q", st.ProgTierLearning)
	}
	if st.ProgTierPracticing != "zh_to_transl" {
		t.Errorf("want prog_tier_practicing=zh_to_transl, got %q", st.ProgTierPracticing)
	}
	if st.ProgTierMastered != "random" {
		t.Errorf("want prog_tier_mastered=random, got %q", st.ProgTierMastered)
	}
	if st.NewWordMode0 != "transl_to_zh" {
		t.Errorf("want new_word_mode_0=transl_to_zh, got %q", st.NewWordMode0)
	}
	if st.NewWordMode1 != "transl_to_zh" {
		t.Errorf("want new_word_mode_1=transl_to_zh, got %q", st.NewWordMode1)
	}
	if st.NewWordMode2 != "zh_to_transl" {
		t.Errorf("want new_word_mode_2=zh_to_transl, got %q", st.NewWordMode2)
	}
	if !st.NewWordRequireZh {
		t.Error("want new_word_require_zh=true by default")
	}
	if !st.NewWordRequireTrans {
		t.Error("want new_word_require_trans=true by default")
	}
	if !st.ExtendSessionWithExtraWords {
		t.Error("want extend_session_with_extra_words=true by default (preserves pre-existing behaviour)")
	}
	if st.BlurPinyin {
		t.Error("want blur_pinyin=false by default")
	}
	if st.NoAutoVoiceOnBlur {
		t.Error("want no_auto_voice_on_blur=false by default")
	}
	if st.CelebrateBucketChange {
		t.Error("want celebrate_bucket_change=false by default")
	}
	if st.VoiceUnavailable {
		t.Error("want voice_unavailable=false by default")
	}
	if st.RetypeOnWrong {
		t.Error("want retype_on_wrong=false by default")
	}
	if st.ShowImagesWithChineseText {
		t.Error("want show_images_with_chinese_text=false by default")
	}
}

func TestUpdateUserSettings_RoundTrip(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	const userID = int64(2)

	in := models.UserSettings{
		PrimaryLang:                 "de",
		SecondaryLang:               "en",
		ProgNew:                     "zh_to_transl",
		ProgTierStruggling:          "zh_pinyin_to_transl",
		ProgTierLearning:            "mask_pinyin",
		ProgTierPracticing:          "random",
		ProgTierMastered:            "random",
		NewWordMode0:                "zh_pinyin_to_transl",
		NewWordMode1:                "zh_to_transl",
		NewWordMode2:                "random",
		NewWordRequireZh:            false,
		NewWordRequireTrans:         true,
		ExtendSessionWithExtraWords: false,
		BlurPinyin:                  true,
		NoAutoVoiceOnBlur:           true,
		CelebrateBucketChange:       true,
		VoiceUnavailable:            true,
		RetypeOnWrong:               true,
		ShowImagesWithChineseText:   true,
	}
	if err := s.UpdateUserSettings(ctx, userID, in); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	out, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettings after update: %v", err)
	}
	if out.PrimaryLang != "de" {
		t.Errorf("primary_lang: want de, got %q", out.PrimaryLang)
	}
	if out.ProgTierStruggling != "zh_pinyin_to_transl" {
		t.Errorf("prog_tier_struggling: want zh_pinyin_to_transl, got %q", out.ProgTierStruggling)
	}
	if out.NewWordMode0 != "zh_pinyin_to_transl" {
		t.Errorf("new_word_mode_0: want zh_pinyin_to_transl, got %q", out.NewWordMode0)
	}
	if out.NewWordRequireZh {
		t.Error("new_word_require_zh: want false after update")
	}
	if !out.NewWordRequireTrans {
		t.Error("new_word_require_trans: want true after update")
	}
	if out.ExtendSessionWithExtraWords {
		t.Error("extend_session_with_extra_words: want false after update")
	}
	if !out.BlurPinyin {
		t.Error("blur_pinyin: want true after update")
	}
	if !out.NoAutoVoiceOnBlur {
		t.Error("no_auto_voice_on_blur: want true after update")
	}
	if !out.CelebrateBucketChange {
		t.Error("celebrate_bucket_change: want true after update")
	}
	if !out.VoiceUnavailable {
		t.Error("voice_unavailable: want true after update")
	}
	if !out.RetypeOnWrong {
		t.Error("retype_on_wrong: want true after update")
	}
	if !out.ShowImagesWithChineseText {
		t.Error("show_images_with_chinese_text: want true after update")
	}
}

func TestGetUserSettings_RandomModeRangeDefaults(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	const userID = int64(2)

	st, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	if st.RandomModeRangeTranslToZh != "" {
		t.Errorf("want random_mode_range_transl_to_zh='' by default, got %q", st.RandomModeRangeTranslToZh)
	}
	if st.RandomModeRangeZhToTransl != "" {
		t.Errorf("want random_mode_range_zh_to_transl='' by default, got %q", st.RandomModeRangeZhToTransl)
	}
	if st.RandomModeRangeZhPinyinToTransl != "" {
		t.Errorf("want random_mode_range_zh_pinyin_to_transl='' by default, got %q", st.RandomModeRangeZhPinyinToTransl)
	}
	if st.RandomModeRangeZhToTranslNoSound != "" {
		t.Errorf("want random_mode_range_zh_to_transl_no_sound='' by default, got %q", st.RandomModeRangeZhToTranslNoSound)
	}
	if st.RandomModeRangeVoiceToTransl != "" {
		t.Errorf("want random_mode_range_voice_to_transl='' by default, got %q", st.RandomModeRangeVoiceToTransl)
	}
}

func TestUpdateUserSettings_RandomModeRangeRoundTrip(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	const userID = int64(2)

	in := models.UserSettings{
		PrimaryLang:                      "en",
		ProgNew:                          "transl_to_zh",
		ProgTierStruggling:               "transl_to_zh",
		ProgTierLearning:                 "zh_pinyin_to_transl",
		ProgTierPracticing:               "zh_to_transl",
		ProgTierMastered:                 "random",
		NewWordMode0:                     "transl_to_zh",
		NewWordMode1:                     "transl_to_zh",
		NewWordMode2:                     "zh_to_transl",
		RandomModeRangeTranslToZh:        "new,50-69",
		RandomModeRangeZhToTransl:        "off",
		RandomModeRangeZhPinyinToTransl:  "new,70-84",
		RandomModeRangeZhToTranslNoSound: "50-69,85-100",
		RandomModeRangeVoiceToTransl:     "70-84,85-100",
	}
	if err := s.UpdateUserSettings(ctx, userID, in); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	out, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettings after update: %v", err)
	}
	if out.RandomModeRangeTranslToZh != "new,50-69" {
		t.Errorf("random_mode_range_transl_to_zh: want %q, got %q", "new,50-69", out.RandomModeRangeTranslToZh)
	}
	if out.RandomModeRangeZhToTransl != "off" {
		t.Errorf("random_mode_range_zh_to_transl: want %q, got %q", "off", out.RandomModeRangeZhToTransl)
	}
	if out.RandomModeRangeZhPinyinToTransl != "new,70-84" {
		t.Errorf("random_mode_range_zh_pinyin_to_transl: want %q, got %q", "new,70-84", out.RandomModeRangeZhPinyinToTransl)
	}
	if out.RandomModeRangeZhToTranslNoSound != "50-69,85-100" {
		t.Errorf("random_mode_range_zh_to_transl_no_sound: want %q, got %q", "50-69,85-100", out.RandomModeRangeZhToTranslNoSound)
	}
	if out.RandomModeRangeVoiceToTransl != "70-84,85-100" {
		t.Errorf("random_mode_range_voice_to_transl: want %q, got %q", "70-84,85-100", out.RandomModeRangeVoiceToTransl)
	}
}

func TestUpdateUserAPIKeys_RoundTrip(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	const userID = int64(2)

	// Store encrypted blobs (plaintext here for simplicity — DB just stores the string)
	if err := s.UpdateUserAPIKeys(ctx, userID, "enc-deepl", "openai", "enc-llm", "http://local"); err != nil {
		t.Fatalf("UpdateUserAPIKeys: %v", err)
	}

	st, salt, deeplEnc, llmEnc, err := s.GetUserSettingsRaw(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettingsRaw: %v", err)
	}
	if salt == "" {
		t.Error("want non-empty salt")
	}
	if deeplEnc != "enc-deepl" {
		t.Errorf("want deeplEnc=enc-deepl, got %q", deeplEnc)
	}
	if llmEnc != "enc-llm" {
		t.Errorf("want llmEnc=enc-llm, got %q", llmEnc)
	}
	if st.LLMProvider != "openai" {
		t.Errorf("want llm_provider=openai, got %q", st.LLMProvider)
	}
	if st.LLMLocalURL != "http://local" {
		t.Errorf("want llm_local_url=http://local, got %q", st.LLMLocalURL)
	}
}

func TestAcceptCorrectModeDefault(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	settings, err := s.GetUserSettings(ctx, 2)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	if settings.AcceptCorrectMode != "typo" {
		t.Errorf("default AcceptCorrectMode: want %q, got %q", "typo", settings.AcceptCorrectMode)
	}
}

func TestAcceptCorrectModeRoundTrip(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	settings, err := s.GetUserSettings(ctx, 2)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	settings.AcceptCorrectMode = "always"
	if err := s.UpdateUserSettings(ctx, 2, *settings); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	got, err := s.GetUserSettings(ctx, 2)
	if err != nil {
		t.Fatalf("GetUserSettings after update: %v", err)
	}
	if got.AcceptCorrectMode != "always" {
		t.Errorf("AcceptCorrectMode: want %q, got %q", "always", got.AcceptCorrectMode)
	}
}

// TestGetUserSettings_CachesAndInvalidates verifies the short-TTL settings cache
// (issue 04 / plan item 1.10): a second read within the TTL returns the cached
// snapshot (same pointer = no second DB load), and a settings write invalidates it.
func TestGetUserSettings_CachesAndInvalidates(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if err := s.UpdateUserSettings(ctx, 2, models.UserSettings{PrimaryLang: "en"}); err != nil {
		t.Fatal(err)
	}

	p1, err := s.GetUserSettings(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.GetUserSettings(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 {
		t.Fatal("second GetUserSettings within TTL should return the cached pointer (single DB load)")
	}

	// A settings write must invalidate the cache so the next read reloads.
	if err := s.UpdateUserSettings(ctx, 2, models.UserSettings{PrimaryLang: "de"}); err != nil {
		t.Fatal(err)
	}
	p3, err := s.GetUserSettings(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if p3 == p2 {
		t.Fatal("GetUserSettings after a write should reload (a new pointer), not serve the stale cache")
	}
	if p3.PrimaryLang != "de" {
		t.Fatalf("reloaded settings should reflect the write, got PrimaryLang=%q", p3.PrimaryLang)
	}
}

func TestUpdateTrainingFilters_PersistsAndReloads(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	if err := s.UpdateTrainingFilters(ctx, userID, "cycle", "0-49",
		[]string{"de", "en"}, false, true, []string{"HSK1", "HSK2"}); err != nil {
		t.Fatalf("UpdateTrainingFilters: %v", err)
	}

	st, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	if st.TrainMode != "cycle" {
		t.Errorf("want train_mode=cycle, got %q", st.TrainMode)
	}
	if st.TrainBucket != "0-49" {
		t.Errorf("want train_bucket=0-49, got %q", st.TrainBucket)
	}
	if len(st.TrainLangs) != 2 || st.TrainLangs[0] != "de" || st.TrainLangs[1] != "en" {
		t.Errorf("want train_langs=[de en], got %v", st.TrainLangs)
	}
	if st.TrainMnemonics {
		t.Error("want train_mnemonics=false")
	}
	if !st.TrainComponents {
		t.Error("want train_components=true")
	}
	if len(st.TrainTags) != 2 || st.TrainTags[0] != "HSK1" || st.TrainTags[1] != "HSK2" {
		t.Errorf("want train_tags=[HSK1 HSK2], got %v", st.TrainTags)
	}
}

func TestUpdateTrainingFilters_Defaults(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// Without calling UpdateTrainingFilters, defaults should apply
	st, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	if st.TrainMode != "random" {
		t.Errorf("want default train_mode=random, got %q", st.TrainMode)
	}
	if len(st.TrainLangs) == 0 {
		t.Error("want default train_langs non-empty")
	}
	if !st.TrainMnemonics {
		t.Error("want default train_mnemonics=true")
	}
	if !st.TrainComponents {
		t.Error("want default train_components=true")
	}
}

func TestUserSettings_GameModeColumns_DefaultOnAndRoundTrip(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	st, err := s.GetUserSettings(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if !st.GameModeMismatch || !st.GameModeNewest || !st.GameModeHardest || !st.GameModeLastMistakes {
		t.Fatalf("expected all 4 game modes enabled by default, got %+v", st)
	}

	st.GameModeMismatch = false
	st.GameModeNewest = false
	st.GameModeHardest = true
	st.GameModeLastMistakes = false
	if err := s.UpdateUserSettings(ctx, int64(2), *st); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetUserSettings(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if got.GameModeMismatch || got.GameModeNewest || !got.GameModeHardest || got.GameModeLastMistakes {
		t.Errorf("game mode round-trip mismatch, got %+v", got)
	}
}
