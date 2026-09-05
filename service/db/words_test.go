package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

func TestCreateWord_ReturnsID(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestCreateWord_Idempotent(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	id2 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if id1 != id2 {
		t.Errorf("re-creating the same word should return the same ID: %d vs %d", id1, id2)
	}
}

func TestCreateWord_MultipleTranslations(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "吃饭", "chī fàn", []string{"eat", "have a meal"})
	wd, err := s.GetWordByID(context.Background(), int64(2), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(wd.Translations["en"]) != 2 {
		t.Errorf("expected 2 en_texts, got %d: %v", len(wd.Translations["en"]), wd.Translations["en"])
	}
}

func TestGetWordByID_NotFound(t *testing.T) {
	s := openTestDB(t)
	wd, err := s.GetWordByID(context.Background(), int64(2), 9999)
	if err != nil {
		t.Fatal(err)
	}
	if wd != nil {
		t.Error("expected nil for missing word")
	}
}

func TestGetWordByID_ContainsZhAndPinyin(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "谢谢", "xiè xiè", []string{"thank you"})
	wd, err := s.GetWordByID(context.Background(), int64(2), id)
	if err != nil {
		t.Fatal(err)
	}
	if wd.ZhText != "谢谢" {
		t.Errorf("ZhText: want 谢谢, got %q", wd.ZhText)
	}
	if wd.Pinyin == nil || *wd.Pinyin != "xiè xiè" {
		t.Errorf("Pinyin: want xiè xiè, got %v", wd.Pinyin)
	}
}

func TestGetWordByID_SM2FieldsPresent(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "再见", "zàijiàn", []string{"goodbye"})
	wd, err := s.GetWordByID(context.Background(), int64(2), id)
	if err != nil {
		t.Fatal(err)
	}
	if wd.Easiness != 2.5 {
		t.Errorf("default easiness should be 2.5, got %f", wd.Easiness)
	}
	if wd.Repetitions != 0 {
		t.Errorf("default repetitions should be 0, got %d", wd.Repetitions)
	}
}

func TestGetWords_ReturnsAll(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	seedWord(t, s, "谢谢", "xiè xiè", []string{"thank you"})
	words, total, err := s.GetWords(context.Background(), int64(2), "", 1, 20, "", "", nil, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("total: want 2, got %d", total)
	}
	if len(words) != 2 {
		t.Errorf("len(words): want 2, got %d", len(words))
	}
}

func TestGetWords_SearchByZh(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	seedWord(t, s, "谢谢", "xiè xiè", []string{"thank you"})
	words, total, err := s.GetWords(context.Background(), int64(2), "你好", 1, 20, "", "", nil, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(words) != 1 {
		t.Errorf("search by zh: want 1 result, got %d/%d", total, len(words))
	}
	if words[0].ZhText != "你好" {
		t.Errorf("wrong word returned: %q", words[0].ZhText)
	}
}

func TestGetWords_IsAlsoComponent(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedWord(t, s, "关", "guān", []string{"close"})
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	s.InsertComponentProgressForTest(ctx, int64(2), "关", time.Now())

	words, _, err := s.GetWords(ctx, int64(2), "", 1, 20, "zh", "asc", nil, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	byText := map[string]bool{}
	for _, w := range words {
		byText[w.ZhText] = w.IsAlsoComponent
	}
	if !byText["关"] {
		t.Error("want 关 flagged as also a component")
	}
	if byText["你好"] {
		t.Error("你好 should not be flagged as also a component")
	}
}

func TestGetWords_SearchByEnText(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	seedWord(t, s, "谢谢", "xiè xiè", []string{"thank you"})
	words, total, err := s.GetWords(context.Background(), int64(2), "thank", 1, 20, "", "", nil, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(words) != 1 {
		t.Errorf("search by en: want 1 result, got %d/%d", total, len(words))
	}
}

func TestGetWords_Pagination(t *testing.T) {
	s := openTestDB(t)
	for i := 0; i < 5; i++ {
		seedWord(t, s, string(rune(0x4e00+i)), "", []string{"word"})
	}
	words, total, err := s.GetWords(context.Background(), int64(2), "", 1, 3, "", "", nil, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("total: want 5, got %d", total)
	}
	if len(words) != 3 {
		t.Errorf("page 1 per_page 3: want 3 results, got %d", len(words))
	}

	words2, _, err := s.GetWords(context.Background(), int64(2), "", 2, 3, "", "", nil, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(words2) != 2 {
		t.Errorf("page 2 per_page 3: want 2 results, got %d", len(words2))
	}
}

func TestUpdateWord_ChangesZhText(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	err := s.UpdateWord(context.Background(), int64(2), id, models.UpdateWordRequest{
		ZhText:       "妳好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello (female)"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2), id)
	if wd.ZhText != "妳好" {
		t.Errorf("ZhText: want 妳好, got %q", wd.ZhText)
	}
	if len(wd.Translations["en"]) != 1 || wd.Translations["en"][0] != "hello (female)" {
		t.Errorf("Translations[en]: want [hello (female)], got %v", wd.Translations["en"])
	}
}

func TestUpdateWord_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.UpdateWord(context.Background(), int64(2), 9999, models.UpdateWordRequest{
		ZhText:       "test",
		Translations: map[string][]string{"en": {"test"}},
	})
	if err == nil {
		t.Error("expected error for unknown id")
	}
}

func TestDeleteWord_Removes(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.DeleteWord(context.Background(), int64(2), id); err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2), id)
	if wd != nil {
		t.Error("word should be gone after delete")
	}
}

func TestDeleteWord_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.DeleteWord(context.Background(), int64(2), 9999)
	if err == nil {
		t.Error("expected error when deleting non-existent word")
	}
}

func TestAddTranslation_AddsNewEN(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.AddTranslation(context.Background(), int64(2), id, "en", "hi"); err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2), id)
	found := false
	for _, e := range wd.Translations["en"] {
		if e == "hi" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'hi' in Translations[en], got %v", wd.Translations["en"])
	}
}

func TestAddTranslation_Idempotent(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	s.AddTranslation(context.Background(), int64(2), id, "en", "hi")
	s.AddTranslation(context.Background(), int64(2), id, "en", "hi") // second call is no-op
	wd, _ := s.GetWordByID(context.Background(), int64(2), id)
	count := 0
	for _, e := range wd.Translations["en"] {
		if e == "hi" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'hi' to appear exactly once, got %d", count)
	}
}

func TestAddTranslation_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.AddTranslation(context.Background(), int64(2), 9999, "en", "hello")
	if err == nil {
		t.Error("expected error for unknown zh word id")
	}
}

func TestGetNextCard_NilWhenEmpty(t *testing.T) {
	s := openTestDB(t)
	w, p, _, err := s.GetNextCard(context.Background(), int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w != nil || p != nil {
		t.Error("expected nil word and progress when DB is empty")
	}
}

func TestGetNextCard_ReturnsZhWord(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	w, p, _, err := s.GetNextCard(context.Background(), int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a word, got nil")
	}
	if w.Language != "zh" {
		t.Errorf("GetNextCard should always return zh words, got language=%q", w.Language)
	}
	if p == nil {
		t.Error("expected progress, got nil")
	}
}

func TestGetNextCard_DoesNotStampFirstSeenDate(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	// GetNextCard should return the word but NOT set first_seen_at.
	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil || w.ID != id {
		t.Fatalf("expected word id=%d, got %v", id, w)
	}

	var firstSeen *string
	s.db.QueryRowContext(ctx, `SELECT first_seen_at FROM sm2_progress WHERE word_id = ?`, id).Scan(&firstSeen)
	if firstSeen != nil {
		t.Errorf("GetNextCard should not set first_seen_at, but got %q", *firstSeen)
	}
}

func TestGetNextCard_MostOverduFirst(t *testing.T) {
	s := openTestDB(t)
	// Use words absent from the (now auto-imported, see issue #340) bundled
	// word_frequency list — "一"/"二" would otherwise be picked by frequency
	// rank regardless of due_date, since unseen-word ordering prefers a lower
	// frequency rank over due_date (see TestGetNextCard_FrequencyRankTiebreak).
	// This test is specifically about the due_date fallback when neither
	// candidate has a frequency ranking.
	id1 := seedWord(t, s, "罕见词甲", "", []string{"rare word a"})
	id2 := seedWord(t, s, "罕见词乙", "", []string{"rare word b"})

	// Set id2's due_date far in the past so it's more overdue
	ctx := context.Background()
	past := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ? WHERE word_id = ?`, past, id2)
	_ = id1

	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w.ID != id2 {
		t.Errorf("expected most-overdue word (id=%d), got id=%d", id2, w.ID)
	}
}

func TestGetNextCard_DailyNewWordLimit(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed 3 words; none have been seen yet (first_seen_at IS NULL).
	id1 := seedWord(t, s, "一", "", []string{"one"})
	seedWord(t, s, "二", "", []string{"two"})
	seedWord(t, s, "三", "", []string{"three"})

	// Simulate having already introduced 1 word today by stamping its first_seen_at.
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET first_seen_at = date('now') WHERE word_id = ?`, id1)

	// With maxNew=1 the daily cap is reached; only id1 (already introduced) should be returned.
	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 1, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a card even when new-word cap is reached")
	}
	if w.ID != id1 {
		t.Errorf("expected already-seen word (id=%d) when cap is reached, got id=%d", id1, w.ID)
	}

	// With maxNew=5 new words are still allowed; any of the three words may be returned.
	w2, _, _, err := s.GetNextCard(ctx, int64(2), nil, 5, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w2 == nil {
		t.Fatal("expected a card when cap is not yet reached")
	}
}

func TestGetNextCard_SkipNewExcludesUnseenWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// id1: already introduced (first_seen_at set).
	id1 := seedWord(t, s, "一", "", []string{"one"})
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET first_seen_at = date('now') WHERE word_id = ?`, id1)

	// id2: never presented (first_seen_at IS NULL).
	seedWord(t, s, "二", "", []string{"two"})

	// With skipNew=true, only the already-seen word should be returned.
	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", true, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected the already-seen word to be returned")
	}
	if w.ID != id1 {
		t.Errorf("expected already-seen word (id=%d), got id=%d", id1, w.ID)
	}
}

func TestGetNextCard_BlocksUnseenWhenLearningWordsExist(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// idLearning: already seen today, still in learning phase (learning_new_word=1).
	idLearning := seedWord(t, s, "一", "", []string{"one"})
	s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET first_seen_at = date('now'), learning_new_word = 1 WHERE word_id = ?`,
		idLearning)

	// idUnseen: never presented (first_seen_at IS NULL).
	seedWord(t, s, "二", "", []string{"two"})

	// Even though the daily cap (100) is not reached, the unseen word must not
	// be returned while a learning word exists.
	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a card to be returned")
	}
	if w.ID != idLearning {
		t.Errorf("expected learning word (id=%d), got id=%d — unseen word was returned while learning words existed", idLearning, w.ID)
	}
}

func TestGetNextCard_LearningWordOutsideTagFilterDoesNotBlockUnseen(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// A learning-phase word tagged "other" — outside the active tag filter.
	idOther := seedWordWithTags(t, s, "一", "", []string{"one"}, []string{"other"})
	s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET first_seen_at = date('now'), learning_new_word = 1 WHERE word_id = ?`,
		idOther)

	// An unseen word tagged "active" — matches the session tag filter.
	idActive := seedWordWithTags(t, s, "二", "", []string{"two"}, []string{"active"})

	// With the "active" tag filter, the learning word (tagged "other") must not
	// block the unseen word from being returned.
	w, _, _, err := s.GetNextCard(ctx, int64(2), []string{"active"}, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected the unseen active-tagged word to be returned")
	}
	if w.ID != idActive {
		t.Errorf("expected unseen word (id=%d), got id=%d — learning word outside tag filter should not block new introductions", idActive, w.ID)
	}
}

func TestGetNextCard_BaselineNewBucket_OutsideTagFilterDoesNotBlockUnseen(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// A word in the New bucket (learning_new_word=1, first_seen_at set) tagged
	// "hsk4" — a tag the user is not currently training.
	idHSK4 := seedWordWithTags(t, s, "水", "", []string{"water"}, []string{"hsk4"})
	if err := s.AcknowledgeWord(ctx, int64(2), idHSK4); err != nil {
		t.Fatal(err)
	}

	// An unseen word tagged "active" — matches the session tag filter.
	idActive := seedWordWithTags(t, s, "火", "", []string{"fire"}, []string{"active"})

	// The New-bucket baseline threshold is 1. The only New-bucket word is
	// tagged "hsk4", outside the "active" tag filter, so it must not count
	// against this session's cap.
	baselines := &NewWordBaselines{NewBucketEnabled: true, NewBucketValue: 1}
	w, _, _, err := s.GetNextCard(ctx, int64(2), []string{"active"}, 100, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected the unseen active-tagged word to be returned")
	}
	if w.ID != idActive {
		t.Errorf("expected unseen word (id=%d), got id=%d — new-bucket word outside tag filter should not block new introductions", idActive, w.ID)
	}
}

func TestGetTranslationsForWord_EN(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello", "hi"})
	words, err := s.GetTranslationsForWord(context.Background(), id, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 2 {
		t.Errorf("expected 2 EN translations, got %d", len(words))
	}
}

func TestGetTranslationsForWord_EmptyWhenNone(t *testing.T) {
	s := openTestDB(t)
	// Manually insert a zh word with no en links
	s.db.Exec(`INSERT INTO words (text, language) VALUES ('孤独', 'zh')`)
	var id int64
	s.db.QueryRow(`SELECT id FROM words WHERE text='孤独'`).Scan(&id)

	words, err := s.GetTranslationsForWord(context.Background(), id, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 0 {
		t.Errorf("expected 0 translations, got %d", len(words))
	}
}

func TestCreateWord_WithTags(t *testing.T) {
	s := openTestDB(t)
	id := seedWordWithTags(t, s, "你好", "nǐ hǎo", []string{"hello"}, []string{"greetings", "HSK1"})
	wd, err := s.GetWordByID(context.Background(), int64(2), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(wd.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(wd.Tags), wd.Tags)
	}
	if wd.Tags[0] != "HSK1" || wd.Tags[1] != "greetings" {
		t.Errorf("tags should be sorted alphabetically, got %v", wd.Tags)
	}
}

func TestUpdateWord_ReplacesTags(t *testing.T) {
	s := openTestDB(t)
	id := seedWordWithTags(t, s, "你好", "nǐ hǎo", []string{"hello"}, []string{"old-tag"})
	err := s.UpdateWord(context.Background(), int64(2), id, models.UpdateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello"}},
		Tags:         []string{"new-tag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2), id)
	if len(wd.Tags) != 1 || wd.Tags[0] != "new-tag" {
		t.Errorf("expected [new-tag], got %v", wd.Tags)
	}
	tags, _ := s.GetAllTags(context.Background(), int64(2))
	for _, tg := range tags {
		if tg == "old-tag" {
			t.Error("orphan tag 'old-tag' should have been cleaned up")
		}
	}
}

func TestGetWords_FilterByTag(t *testing.T) {
	s := openTestDB(t)
	seedWordWithTags(t, s, "你好", "nǐ hǎo", []string{"hello"}, []string{"greetings"})
	seedWordWithTags(t, s, "吃饭", "chī fàn", []string{"eat"}, []string{"food"})
	seedWordWithTags(t, s, "谢谢", "xiè xiè", []string{"thanks"}, []string{"greetings"})

	words, total, err := s.GetWords(context.Background(), int64(2), "", 1, 20, "", "", []string{"greetings"}, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("tag filter: want 2, got %d", total)
	}
	if len(words) != 2 {
		t.Errorf("tag filter: want 2 words, got %d", len(words))
	}
}

func TestGetWords_FilterByMultipleTags_OR(t *testing.T) {
	s := openTestDB(t)
	seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"greetings"})
	seedWordWithTags(t, s, "吃饭", "", []string{"eat"}, []string{"food"})
	seedWordWithTags(t, s, "书", "", []string{"book"}, []string{"school"})

	words, total, err := s.GetWords(context.Background(), int64(2), "", 1, 20, "", "", []string{"greetings", "food"}, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("multi-tag OR filter: want 2, got %d", total)
	}
	if len(words) != 2 {
		t.Errorf("multi-tag OR filter: want 2 words, got %d", len(words))
	}
}

func TestGetNextCard_DoesNotReturnFutureCards(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	id := seedWord(t, s, "一", "", []string{"one"})

	// Mark the word as seen (first_seen_at set) and place its due_date
	// 2 days in the future — it should NOT be returned by GetNextCard.
	future := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ?, first_seen_at = date('now') WHERE word_id = ?`, future, id)

	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w != nil {
		t.Errorf("expected nil for a card due in the future (id=%d), but got id=%d", id, w.ID)
	}
}

func TestGetNextCard_ReturnsTodayNotYetOverdue(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	id := seedWord(t, s, "一", "", []string{"one"})

	// Place due_date 5 minutes from now (today but not yet overdue).
	soon := time.Now().UTC().Add(5 * time.Minute).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ?, first_seen_at = date('now') WHERE word_id = ?`, soon, id)

	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a card due today (in 5 min) to be returned")
	}
	if w.ID != id {
		t.Errorf("expected word id=%d, got id=%d", id, w.ID)
	}
}

func TestGetNextCard_FilterByTag(t *testing.T) {
	s := openTestDB(t)
	seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"greetings"})
	id2 := seedWordWithTags(t, s, "吃饭", "", []string{"eat"}, []string{"food"})

	w, _, _, err := s.GetNextCard(context.Background(), int64(2), []string{"food"}, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a card")
	}
	if w.ID != id2 {
		t.Errorf("expected food-tagged word (id=%d), got id=%d", id2, w.ID)
	}
}

func TestGetNextCard_NoMatchingTag_ReturnsNil(t *testing.T) {
	s := openTestDB(t)
	seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"greetings"})

	w, _, _, err := s.GetNextCard(context.Background(), int64(2), []string{"nonexistent"}, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w != nil {
		t.Error("expected nil when no words match tag filter")
	}
}

func TestGetNextCard_ExcludeIDs_SkipsWhenOthersAvailable(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	idA := seedWord(t, s, "一", "", []string{"one"})
	idB := seedWord(t, s, "二", "", []string{"two"})
	// Both words are seeded with due_date = now (unseen); make them seen and due.
	past := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ?, first_seen_at = date('now'), total_attempts = 1 WHERE word_id IN (?, ?)`, past, idA, idB)

	// Excluding idA should return idB.
	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, []int64{idA}, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a word, got nil")
	}
	if w.ID != idB {
		t.Errorf("expected excluded word to be skipped: want id=%d, got id=%d", idB, w.ID)
	}
}

func TestGetNextCard_ExcludeIDs_FallsBackToExcludedWhenNoOthers(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	idA := seedWord(t, s, "一", "", []string{"one"})
	past := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ?, first_seen_at = date('now'), total_attempts = 1 WHERE word_id = ?`, past, idA)

	// With only one word, excluding it must still return it (no other option).
	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, []int64{idA}, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected fallback to excluded word, got nil")
	}
	if w.ID != idA {
		t.Errorf("expected fallback to excluded word id=%d, got id=%d", idA, w.ID)
	}
}

// TestGetNextCard_ExcludeIDs_PrefersFarFutureOverExcluded reproduces a bug where
// a non-excluded word due beyond today's bound was skipped in favor of repeating
// an excluded word, because the final fallback tier dropped the exclusion filter
// but kept the todayBound restriction.
func TestGetNextCard_ExcludeIDs_PrefersFarFutureOverExcluded(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	idA := seedWord(t, s, "一", "", []string{"one"})
	idB := seedWord(t, s, "二", "", []string{"two"})
	idC := seedWord(t, s, "三", "", []string{"three"})

	past := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	farFuture := time.Now().UTC().Add(30 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = ?, first_seen_at = date('now'), total_attempts = 1 WHERE word_id IN (?, ?)`,
		past, idA, idB); err != nil {
		t.Fatal(err)
	}
	// idC was answered correctly and its interval pushed it weeks into the future.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = ?, first_seen_at = date('now'), total_attempts = 1 WHERE word_id = ?`,
		farFuture, idC); err != nil {
		t.Fatal(err)
	}

	// Excluding idA and idB should still return idC rather than repeating an
	// excluded word, even though idC's due_date is beyond today's bound.
	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, []int64{idA, idB}, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a word, got nil")
	}
	if w.ID != idC {
		t.Errorf("expected non-excluded far-future word to win over excluded words: want id=%d, got id=%d", idC, w.ID)
	}
}

// TestGetNextCard_SessionExtension_FlagsFutureCardAsExtended reproduces the
// due_today display bug (#186): when the only due-today card is excluded to
// avoid immediate repetition, GetNextCard widens the search beyond today and
// serves a genuinely non-due future card. Callers must be told this happened
// (via the extended return value) so the displayed due-today count can be
// corrected to match what the user will actually be asked.
func TestGetNextCard_SessionExtension_FlagsFutureCardAsExtended(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	idA := seedWord(t, s, "一", "", []string{"one"}) // due today, will be excluded
	idB := seedWord(t, s, "二", "", []string{"two"}) // due in the far future

	past := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	farFuture := time.Now().UTC().Add(30 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = ?, first_seen_at = date('now'), total_attempts = 1 WHERE word_id = ?`,
		past, idA); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = ?, first_seen_at = date('now'), total_attempts = 1 WHERE word_id = ?`,
		farFuture, idB); err != nil {
		t.Fatal(err)
	}

	// idA is the only due-today card but it is excluded (just answered) — with
	// session extension allowed, GetNextCard must fall back to idB and report
	// extended=true so the frontend can inflate its due-today display by 1.
	w, _, extended, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, []int64{idA}, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a word, got nil")
	}
	if w.ID != idB {
		t.Errorf("expected the non-due future word: want id=%d, got id=%d", idB, w.ID)
	}
	if !extended {
		t.Error("expected extended=true when a non-due card was served to avoid repetition")
	}
}

// TestGetNextCard_SessionExtensionDisabled_NeverServesFutureCard verifies the
// new user setting: when allowSessionExtension=false, GetNextCard must never
// widen beyond today's due-date bound. It should repeat the excluded (but
// genuinely due) word rather than serve a not-yet-due word.
func TestGetNextCard_SessionExtensionDisabled_NeverServesFutureCard(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	idA := seedWord(t, s, "一", "", []string{"one"}) // due today, will be excluded
	idB := seedWord(t, s, "二", "", []string{"two"}) // due in the far future

	past := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	farFuture := time.Now().UTC().Add(30 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = ?, first_seen_at = date('now'), total_attempts = 1 WHERE word_id = ?`,
		past, idA); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = ?, first_seen_at = date('now'), total_attempts = 1 WHERE word_id = ?`,
		farFuture, idB); err != nil {
		t.Fatal(err)
	}

	w, _, extended, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, []int64{idA}, false)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a word, got nil")
	}
	if w.ID != idA {
		t.Errorf("expected the genuinely due-today word to repeat rather than pulling in a future word: want id=%d, got id=%d", idA, w.ID)
	}
	if extended {
		t.Error("expected extended=false when session extension is disabled")
	}
}

// A user must not be able to attach a translation to another user's word.
func TestAddTranslation_UserIsolation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// User 3 owns a word.
	user3ID, err := s.CreateUser(ctx, "user3@example.com", "hash", "tok-u3", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	victimID, err := s.CreateWord(ctx, user3ID, models.CreateWordRequest{
		ZhText: "再见", Translations: map[string][]string{"en": {"goodbye"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// User 2 (created by openTestDB) tries to attach a translation to user 3's word.
	err = s.AddTranslation(ctx, int64(2), victimID, "en", "intruder")
	if err == nil {
		t.Fatal("expected AddTranslation to reject a word the caller does not own")
	}

	// User 3's word must be untouched.
	wd, err := s.GetWordByID(ctx, user3ID, victimID)
	if err != nil {
		t.Fatal(err)
	}
	for _, txt := range wd.Translations["en"] {
		if txt == "intruder" {
			t.Errorf("victim word gained an unauthorized translation: %v", wd.Translations["en"])
		}
	}
}

func TestGetNextCard_BaselineStruggling_BlocksNewWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// Create a seen word and put it in the struggling bucket:
	// learning_new_word=0, total_attempts=5, total_correct=1 → accuracy=20%
	wordID, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "水", Translations: map[string][]string{"en": {"water"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET first_seen_at = date('now'), learning_new_word = 0,
		 total_attempts = 5, total_correct = 1, due_date = CURRENT_TIMESTAMP
		 WHERE word_id = ?`, wordID); err != nil {
		t.Fatal(err)
	}

	// Create an unseen word.
	if _, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "火", Translations: map[string][]string{"en": {"fire"}},
	}); err != nil {
		t.Fatal(err)
	}

	// Struggling count is 1; threshold is 1 → block new words.
	baselines := &NewWordBaselines{StrugglingEnabled: true, StrugglingValue: 1}
	w, _, _, err := s.GetNextCard(ctx, userID, nil, 100, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	// The unseen word should be blocked; GetNextCard returns the seen struggling word instead.
	if w == nil {
		t.Fatal("expected a word, got nil")
	}
	if w.Text == "火" {
		t.Error("baseline struggling should have blocked the unseen word 火")
	}
}

func TestGetNextCard_BaselineDueToday_BlocksNewWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// Create and acknowledge a seen word so there is 1 due word today.
	wordID, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "水", Translations: map[string][]string{"en": {"water"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeWord(ctx, userID, wordID); err != nil {
		t.Fatal(err)
	}

	// Create an unseen word.
	if _, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "火", Translations: map[string][]string{"en": {"fire"}},
	}); err != nil {
		t.Fatal(err)
	}

	// Threshold is 1 — due_at_day_start (1) >= 1 → block new words.
	baselines := &NewWordBaselines{DueTodayEnabled: true, DueTodayValue: 1}
	w, _, _, err := s.GetNextCard(ctx, userID, nil, 100, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w != nil && w.Text == "火" {
		t.Error("baseline due-today should have blocked the unseen word 火")
	}
}

func TestGetNextCard_BaselineNewBucket_BlocksNewWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// Create and acknowledge a word so it lands in the New bucket:
	// learning_new_word=1, first_seen_at IS NOT NULL.
	wordID, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "水", Translations: map[string][]string{"en": {"water"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeWord(ctx, userID, wordID); err != nil {
		t.Fatal(err)
	}

	// Create an unseen word (first_seen_at IS NULL).
	if _, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "火", Translations: map[string][]string{"en": {"fire"}},
	}); err != nil {
		t.Fatal(err)
	}

	// New-bucket count is 1; threshold is 1 → block new words.
	baselines := &NewWordBaselines{NewBucketEnabled: true, NewBucketValue: 1}
	w, _, _, err := s.GetNextCard(ctx, userID, nil, 100, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a word, got nil")
	}
	if w.Text == "火" {
		t.Error("baseline new-bucket should have blocked the unseen word 火")
	}
}

// TestGetNextCard_BaselineNewBucket_NotYetDueDoesNotBlockNewWords guards
// against issue #398: a new-bucket word whose due_date has been pushed days
// into the future (e.g. by the match-game bug that used to strand words in
// learning_new_word=1 with a real SM2-scale due date) must not count against
// the NewBucketEnabled cap, since it isn't actually reachable today anyway.
// Without this, such a word permanently occupies a "new bucket" slot and
// blocks introduction of genuinely new words until its bogus due date passes.
func TestGetNextCard_BaselineNewBucket_NotYetDueDoesNotBlockNewWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// Create and acknowledge a word so it lands in the New bucket, then push
	// its due_date days into the future — as if it were stranded there by
	// the match-game bug rather than being freshly introduced today.
	wordID, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "水", Translations: map[string][]string{"en": {"water"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeWord(ctx, userID, wordID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = datetime('now', '+7 days') WHERE word_id = ?`, wordID); err != nil {
		t.Fatal(err)
	}

	// Create an unseen word.
	if _, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "火", Translations: map[string][]string{"en": {"fire"}},
	}); err != nil {
		t.Fatal(err)
	}

	// New-bucket count would be 1 with threshold 1 — but the stuck word isn't
	// due today, so it shouldn't count, and the unseen word should still show.
	baselines := &NewWordBaselines{NewBucketEnabled: true, NewBucketValue: 1}
	w, _, _, err := s.GetNextCard(ctx, userID, nil, 100, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a word, got nil")
	}
	if w.Text != "火" {
		t.Errorf("expected unseen word 火 to still be introducible since the stuck new-bucket word isn't due, got %s", w.Text)
	}
}

func TestGetNextCard_BaselineNewBucket_BelowThreshold_StillShowsNewWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// Create and acknowledge a word so it lands in the New bucket, then push its
	// due_date into the future so it doesn't compete as the "most overdue" card
	// (see the learningDue check in GetNextCard) — only its bucket membership matters here.
	wordID, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "水", Translations: map[string][]string{"en": {"water"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeWord(ctx, userID, wordID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = datetime('now', '+30 days') WHERE word_id = ?`, wordID); err != nil {
		t.Fatal(err)
	}

	// Create an unseen word.
	if _, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "火", Translations: map[string][]string{"en": {"fire"}},
	}); err != nil {
		t.Fatal(err)
	}

	// New-bucket count is 1; threshold is 3 → new word still introducible.
	baselines := &NewWordBaselines{NewBucketEnabled: true, NewBucketValue: 3}
	w, _, _, err := s.GetNextCard(ctx, userID, nil, 100, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a word, got nil")
	}
	if w.Text != "火" {
		t.Errorf("expected unseen word 火 to still be introducible below threshold, got %s", w.Text)
	}
}

func TestGetNextCard_Baselines_AllDisabled_StillShowsNewWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	if _, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "火", Translations: map[string][]string{"en": {"fire"}},
	}); err != nil {
		t.Fatal(err)
	}

	baselines := &NewWordBaselines{} // all disabled
	w, _, _, err := s.GetNextCard(ctx, userID, nil, 100, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected the unseen word to be returned when baselines are disabled")
	}
	if w.Text != "火" {
		t.Errorf("expected 火, got %s", w.Text)
	}
}

func TestGetNextCard_Cooldown_BlocksSecondNewWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// Seed and acknowledge enough words to be past the new-learner cooldown
	// bypass threshold, so the cooldown baseline actually applies below.
	seedIntroducedWords(t, s, userID, newLearnerCooldownBypassThreshold)

	// Create one more unseen word.
	if _, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "火", Translations: map[string][]string{"en": {"fire"}},
	}); err != nil {
		t.Fatal(err)
	}

	// With a 60-minute cooldown, the new unseen word should be blocked.
	baselines := &NewWordBaselines{CooldownMinutes: 60}
	w, _, _, err := s.GetNextCard(ctx, userID, nil, 100, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w != nil && w.Text == "火" {
		t.Error("cooldown should have blocked the new unseen word 火")
	}
}

// seedIntroducedWords creates and acknowledges n distinct zh words for userID,
// used to push the account past newLearnerCooldownBypassThreshold in tests.
func seedIntroducedWords(t *testing.T, s *Store, userID int64, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		id, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
			ZhText: fmt.Sprintf("字%d", i), Translations: map[string][]string{"en": {fmt.Sprintf("char%d", i)}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.AcknowledgeWord(ctx, userID, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGetNextCard_Cooldown_BypassedForNewLearner(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// Only one introduced word — well under newLearnerCooldownBypassThreshold.
	id1, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "水", Translations: map[string][]string{"en": {"water"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeWord(ctx, userID, id1); err != nil {
		t.Fatal(err)
	}
	// Push 水's due date into the future so it isn't itself due for review —
	// otherwise GetNextCard would return it regardless of the cooldown baseline,
	// since a due learning card always takes priority over an unseen word.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET due_date = datetime('now', '+1 day') WHERE word_id = ?`, id1); err != nil {
		t.Fatal(err)
	}

	// Create a second unseen word.
	if _, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "火", Translations: map[string][]string{"en": {"fire"}},
	}); err != nil {
		t.Fatal(err)
	}

	// Even with a 60-minute cooldown, a new learner (few introduced words)
	// should not be blocked from seeing the next new word.
	baselines := &NewWordBaselines{CooldownMinutes: 60}
	w, _, _, err := s.GetNextCard(ctx, userID, nil, 100, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil || w.Text != "火" {
		t.Errorf("expected cooldown to be bypassed for a new learner and return 火, got %v", w)
	}
}

func TestGetNextCard_Cooldown_Zero_DoesNotBlock(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	userID := int64(2)

	// Acknowledge a first word.
	id1, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "水", Translations: map[string][]string{"en": {"water"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeWord(ctx, userID, id1); err != nil {
		t.Fatal(err)
	}

	// Create a second unseen word.
	if _, err := s.CreateWord(ctx, userID, models.CreateWordRequest{
		ZhText: "火", Translations: map[string][]string{"en": {"fire"}},
	}); err != nil {
		t.Fatal(err)
	}

	// CooldownMinutes=0 means disabled — second unseen word should appear.
	baselines := &NewWordBaselines{CooldownMinutes: 0}
	w, _, _, err := s.GetNextCard(ctx, userID, nil, 100, "", false, baselines, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a word with cooldown disabled")
	}
}

func TestDeleteWord_CleansOrphanTags(t *testing.T) {
	s := openTestDB(t)
	id := seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"unique-tag"})
	if err := s.DeleteWord(context.Background(), int64(2), id); err != nil {
		t.Fatal(err)
	}
	tags, _ := s.GetAllTags(context.Background(), int64(2))
	if len(tags) != 0 {
		t.Errorf("expected no tags after deleting only word, got %v", tags)
	}
}

func TestDeleteWord_SharedTagRetained(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	id1 := seedWordWithTags(t, s, "一", "", []string{"one"}, []string{"shared-tag"})
	seedWordWithTags(t, s, "二", "", []string{"two"}, []string{"shared-tag"})

	if err := s.DeleteWord(ctx, int64(2), id1); err != nil {
		t.Fatal(err)
	}

	tags, _ := s.GetAllTags(ctx, int64(2))
	found := false
	for _, tg := range tags {
		if tg == "shared-tag" {
			found = true
		}
	}
	if !found {
		t.Error("shared-tag should be retained when another word still uses it")
	}
}

func TestDeleteWord_CascadesToConfusionPairs(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), int64(2), idA, idB, "zh_to_transl"); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteWord(context.Background(), int64(2), idA); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("confusion_pairs should be cascade-deleted, got %d rows", len(items))
	}
}

func TestMarkWordForReview_SetsFlag(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	if err := s.MarkWordForReview(context.Background(), int64(2), id); err != nil {
		t.Fatalf("MarkWordForReview: %v", err)
	}

	wd, err := s.GetWordByID(context.Background(), int64(2), id)
	if err != nil {
		t.Fatal(err)
	}
	if !wd.NeedsReview {
		t.Error("expected NeedsReview = true after marking")
	}
}

func TestMarkWordForReview_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.MarkWordForReview(context.Background(), int64(2), 9999)
	if err == nil {
		t.Error("expected error for missing word, got nil")
	}
}

func TestUpdateWord_ClearsReviewFlag(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	if err := s.MarkWordForReview(context.Background(), int64(2), id); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateWord(context.Background(), int64(2), id, models.UpdateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello"}},
	}); err != nil {
		t.Fatalf("UpdateWord: %v", err)
	}

	wd, err := s.GetWordByID(context.Background(), int64(2), id)
	if err != nil {
		t.Fatal(err)
	}
	if wd.NeedsReview {
		t.Error("expected NeedsReview = false after update")
	}
}

func TestGetWords_ReviewOnlyFilter(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	_ = seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})

	if err := s.MarkWordForReview(context.Background(), int64(2), id1); err != nil {
		t.Fatal(err)
	}

	words, total, err := s.GetWords(context.Background(), int64(2), "", 1, 20, "", "desc", nil, true, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("expected 1 review word, got %d", total)
	}
	if len(words) != 1 || words[0].ID != id1 {
		t.Errorf("expected word id %d in review filter result", id1)
	}
}

func TestGetWords_HideUnseenFilter(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1 := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	_ = seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})

	// Mark id1 as seen by incrementing total_attempts
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET total_attempts = 1 WHERE word_id = ?`, id1); err != nil {
		t.Fatal(err)
	}

	words, total, err := s.GetWords(ctx, int64(2), "", 1, 20, "", "desc", nil, false, true, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("hide unseen filter: want total=1, got %d", total)
	}
	if len(words) != 1 || words[0].ID != id1 {
		t.Errorf("hide unseen filter: expected word %d, got %v", id1, words)
	}
}

func TestGetZhTextByID_Found(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	req := models.CreateWordRequest{ZhText: "你好", Translations: map[string][]string{"en": {"hello"}}}
	id, err := s.CreateWord(ctx, 2, req)
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}
	text, err := s.GetZhTextByID(ctx, 2, id)
	if err != nil {
		t.Fatalf("GetZhTextByID: %v", err)
	}
	if text != "你好" {
		t.Errorf("want 你好, got %q", text)
	}
}

func TestGetZhTextByID_NotFound(t *testing.T) {
	s := openTestDB(t)
	text, err := s.GetZhTextByID(context.Background(), 2, 9999)
	if err != nil {
		t.Fatalf("want no error for missing word, got: %v", err)
	}
	if text != "" {
		t.Errorf("want empty string for missing word, got %q", text)
	}
}

func TestIsZhWordForUser_True(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	_, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText: "女", Translations: map[string][]string{"en": {"woman"}},
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}
	ok, err := s.IsZhWordForUser(ctx, 2, "女")
	if err != nil {
		t.Fatalf("IsZhWordForUser: %v", err)
	}
	if !ok {
		t.Error("want true for existing zh word")
	}
}

func TestIsZhWordForUser_False(t *testing.T) {
	s := openTestDB(t)
	ok, err := s.IsZhWordForUser(context.Background(), 2, "女")
	if err != nil {
		t.Fatalf("IsZhWordForUser: %v", err)
	}
	if ok {
		t.Error("want false for non-existent zh word")
	}
}

func TestIsZhWordForUser_DifferentUser(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	_, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText: "女", Translations: map[string][]string{"en": {"woman"}},
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}
	ok, err := s.IsZhWordForUser(ctx, 99, "女")
	if err != nil {
		t.Fatalf("IsZhWordForUser: %v", err)
	}
	if ok {
		t.Error("want false for word owned by a different user")
	}
}

func TestIsComponentForUser_True(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now())

	ok, err := s.IsComponentForUser(ctx, 2, "女")
	if err != nil {
		t.Fatalf("IsComponentForUser: %v", err)
	}
	if !ok {
		t.Error("want true for existing component")
	}
}

func TestIsComponentForUser_False(t *testing.T) {
	s := openTestDB(t)
	ok, err := s.IsComponentForUser(context.Background(), 2, "女")
	if err != nil {
		t.Fatalf("IsComponentForUser: %v", err)
	}
	if ok {
		t.Error("want false for non-existent component")
	}
}

func TestIsComponentForUser_DifferentUser(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	s.InsertComponentProgressForTest(ctx, int64(2), "女", time.Now())

	ok, err := s.IsComponentForUser(ctx, 99, "女")
	if err != nil {
		t.Fatalf("IsComponentForUser: %v", err)
	}
	if ok {
		t.Error("want false for component owned by a different user")
	}
}

func TestGetTranslationLanguages_EmptyDB(t *testing.T) {
	s := openTestDB(t)
	langs, err := s.GetTranslationLanguages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(langs) != 0 {
		t.Errorf("expected empty slice, got %v", langs)
	}
}

func TestGetTranslationLanguages_OnlyEN(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	langs, err := s.GetTranslationLanguages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(langs) != 1 || langs[0] != "en" {
		t.Errorf("expected [en], got %v", langs)
	}
}

func TestGetTranslationLanguages_ENandDE(t *testing.T) {
	s := openTestDB(t)
	// Create a word with both EN and DE translations.
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello"}, "de": {"hallo"}},
	})
	if err != nil || id <= 0 {
		t.Fatalf("CreateWord: %v / id=%d", err, id)
	}
	langs, err := s.GetTranslationLanguages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(langs) != 2 {
		t.Fatalf("expected 2 languages, got %v", langs)
	}
	// Results are ORDER BY language, so "de" < "en".
	if langs[0] != "de" || langs[1] != "en" {
		t.Errorf("expected [de en], got %v", langs)
	}
}

func TestGetTranslationsForWord_DE(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       "再见",
		Pinyin:       "zàijiàn",
		Translations: map[string][]string{"en": {"goodbye"}, "de": {"auf Wiedersehen", "tschüss"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	words, err := s.GetTranslationsForWord(context.Background(), id, "de")
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 2 {
		t.Errorf("expected 2 DE translations, got %d: %v", len(words), words)
	}
	for _, w := range words {
		if w.Language != "de" {
			t.Errorf("expected language=de, got %q", w.Language)
		}
	}
}

func TestGetTranslationsForWord_DEvsEN_NoMix(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       "吃",
		Pinyin:       "chī",
		Translations: map[string][]string{"en": {"eat"}, "de": {"essen"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	enWords, err := s.GetTranslationsForWord(context.Background(), id, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(enWords) != 1 || enWords[0].Text != "eat" {
		t.Errorf("EN: expected [eat], got %v", enWords)
	}
	deWords, err := s.GetTranslationsForWord(context.Background(), id, "de")
	if err != nil {
		t.Fatal(err)
	}
	if len(deWords) != 1 || deWords[0].Text != "essen" {
		t.Errorf("DE: expected [essen], got %v", deWords)
	}
}

func TestGetWords_MissingLangEN(t *testing.T) {
	s := openTestDB(t)
	// Word with EN only (no DE).
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	// Word with both EN and DE.
	_, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       "再见",
		Pinyin:       "zàijiàn",
		Translations: map[string][]string{"en": {"goodbye"}, "de": {"auf Wiedersehen"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Filter words missing DE — should return only 你好.
	words, total, err := s.GetWords(context.Background(), int64(2), "", 1, 20, "", "", nil, false, false, "", "", "de")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(words) != 1 {
		t.Errorf("missing_lang=de: want 1 result, got total=%d len=%d", total, len(words))
	}
	if words[0].ZhText != "你好" {
		t.Errorf("expected 你好, got %q", words[0].ZhText)
	}
}

func TestGetWords_MissingLangDE(t *testing.T) {
	s := openTestDB(t)
	// Word missing EN (raw insert to bypass CreateWord EN requirement).
	s.db.Exec(`INSERT INTO words (text, language, user_id) VALUES ('孤独', 'zh', 2)`)
	var zhID int64
	s.db.QueryRow(`SELECT id FROM words WHERE text = '孤独'`).Scan(&zhID)
	s.db.Exec(`INSERT INTO sm2_progress (word_id, repetitions, easiness, interval_days, due_date, total_correct, total_attempts, streak_bonus) VALUES (?, 0, 2.5, 1, CURRENT_TIMESTAMP, 0, 0, 0)`, zhID)
	// DE word linked to it.
	s.db.Exec(`INSERT INTO words (text, language, user_id) VALUES ('Einsamkeit', 'de', 2)`)
	var deID int64
	s.db.QueryRow(`SELECT id FROM words WHERE text = 'Einsamkeit'`).Scan(&deID)
	s.db.Exec(`INSERT INTO translations (translation_word_id, zh_word_id) VALUES (?, ?)`, deID, zhID)

	// Word with both EN and DE.
	s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello"}, "de": {"hallo"}},
	})

	// Filter missing EN — should return only 孤独.
	words, total, err := s.GetWords(context.Background(), int64(2), "", 1, 20, "", "", nil, false, false, "", "", "en")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(words) != 1 {
		t.Errorf("missing_lang=en: want 1 result, got total=%d len=%d", total, len(words))
	}
	if words[0].ZhText != "孤独" {
		t.Errorf("expected 孤独, got %q", words[0].ZhText)
	}
}

func TestGetWords_MissingLangEmpty_ReturnsAll(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "", []string{"hello"})
	seedWord(t, s, "再见", "", []string{"goodbye"})
	words, total, err := s.GetWords(context.Background(), int64(2), "", 1, 20, "", "", nil, false, false, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(words) != 2 {
		t.Errorf("empty missingLang: want 2 results, got total=%d len=%d", total, len(words))
	}
}

func TestUpdateWord_UnchangedZhText_NoError(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	// Save with the exact same ZhText — should not cause a UNIQUE constraint error.
	err := s.UpdateWord(context.Background(), int64(2), id, models.UpdateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello", "hi"}},
	})
	if err != nil {
		t.Fatalf("UpdateWord with unchanged ZhText should not fail: %v", err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2), id)
	if wd.ZhText != "你好" {
		t.Errorf("ZhText should be unchanged, got %q", wd.ZhText)
	}
	if len(wd.Translations["en"]) != 2 {
		t.Errorf("expected 2 EnTexts after update, got %d: %v", len(wd.Translations["en"]), wd.Translations["en"])
	}
}

func TestCreateWord_WithDeTexts(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       "你好",
		Pinyin:       "nǐ hǎo",
		Translations: map[string][]string{"en": {"hello"}, "de": {"hallo", "guten tag"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, err := s.GetWordByID(context.Background(), int64(2), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(wd.Translations["de"]) != 2 {
		t.Errorf("expected 2 DeTexts, got %d: %v", len(wd.Translations["de"]), wd.Translations["de"])
	}
}

func TestUpdateWord_ReplacesDeTexts(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       "再见",
		Pinyin:       "zàijiàn",
		Translations: map[string][]string{"en": {"goodbye"}, "de": {"auf Wiedersehen"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = s.UpdateWord(context.Background(), int64(2), id, models.UpdateWordRequest{
		ZhText:       "再见",
		Pinyin:       "zàijiàn",
		Translations: map[string][]string{"en": {"goodbye"}, "de": {"tschüss", "ciao"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2), id)
	if len(wd.Translations["de"]) != 2 {
		t.Errorf("expected 2 DeTexts after update, got %d: %v", len(wd.Translations["de"]), wd.Translations["de"])
	}
	for _, dt := range wd.Translations["de"] {
		if dt == "auf Wiedersehen" {
			t.Error("old DE translation should have been removed")
		}
	}
}

func TestGetNextCard_PrefersUnseenOverAdvancedSeen(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seen word: shift due_date 30 days into the past (simulates a high-interval
	// word that was advanced), clear learning_new_word so it doesn't block the
	// unseen-priority path.
	idSeen := seedWord(t, s, "一", "", []string{"one"})
	s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET first_seen_at = date('now'), due_date = datetime('now', '-30 days'), learning_new_word = 0 WHERE word_id = ?`,
		idSeen)

	// Unseen word whose due_date is the default CURRENT_TIMESTAMP (recent).
	idUnseen := seedWord(t, s, "二", "", []string{"two"})

	// cap=100, no learning_new_word=1 words due → unseen should be preferred
	// even though the seen word has an older due_date.
	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a card")
	}
	if w.ID != idUnseen {
		t.Errorf("want unseen word (id=%d), got id=%d — advanced seen word took priority", idUnseen, w.ID)
	}
}

func TestGetWordIDByZhText_Found(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	req := models.CreateWordRequest{ZhText: "你好", Translations: map[string][]string{"en": {"hello"}}}
	id, err := s.CreateWord(ctx, 2, req)
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}
	got, err := s.GetWordIDByZhText(ctx, 2, "你好")
	if err != nil {
		t.Fatalf("GetWordIDByZhText: %v", err)
	}
	if got != id {
		t.Errorf("want id=%d, got %d", id, got)
	}
}

func TestGetWordIDByZhText_NotFound(t *testing.T) {
	s := openTestDB(t)
	got, err := s.GetWordIDByZhText(context.Background(), 2, "你好")
	if err != nil {
		t.Fatalf("want no error for missing word, got: %v", err)
	}
	if got != 0 {
		t.Errorf("want 0 for missing word, got %d", got)
	}
}

func TestGetPinyinByZhText_Found(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedWord(t, s, "看书", "kàn shū", []string{"to read"})
	got, err := s.GetPinyinByZhText(ctx, 2, "看书")
	if err != nil {
		t.Fatalf("GetPinyinByZhText: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil pinyin")
	}
	if *got != "kàn shū" {
		t.Errorf("want %q, got %q", "kàn shū", *got)
	}
}

func TestGetPinyinByZhText_NoPinyin(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedWord(t, s, "看书", "", []string{"to read"})
	got, err := s.GetPinyinByZhText(ctx, 2, "看书")
	if err != nil {
		t.Fatalf("GetPinyinByZhText: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for empty pinyin, got %q", *got)
	}
}

func TestGetPinyinByZhText_NotFound(t *testing.T) {
	s := openTestDB(t)
	got, err := s.GetPinyinByZhText(context.Background(), 2, "不存在")
	if err != nil {
		t.Fatalf("want no error for missing word, got: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for missing word, got %q", *got)
	}
}

// TestCreateWord_StartTraining covers issue 13 (5.5): CreateWord with
// StartTraining=true atomically acknowledges the word (first_seen_date set) and
// initialises its component cards, inside the same transaction.
func TestCreateWord_StartTraining(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDecomp(t, s, "好", "⿰女子")
	seedHanziDef(t, s, "女", "woman; female")
	seedHanziDef(t, s, "子", "child; son")

	id, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:        "好",
		Translations:  map[string][]string{"en": {"good"}},
		StartTraining: true,
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	var firstSeen *string
	if err := s.db.QueryRowContext(ctx,
		`SELECT first_seen_at FROM sm2_progress WHERE word_id = ?`, id).Scan(&firstSeen); err != nil {
		t.Fatalf("read first_seen_date: %v", err)
	}
	if firstSeen == nil {
		t.Errorf("StartTraining=true should set first_seen_date, got NULL")
	}
	var comps int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM component_progress WHERE user_id = 2`).Scan(&comps)
	if comps != 2 {
		t.Errorf("StartTraining=true should create 2 component rows (女, 子), got %d", comps)
	}
}

// TestCreateWord_NoStartTraining verifies StartTraining=false leaves the word
// unseen (first_seen_date NULL) and creates no component rows.
func TestCreateWord_NoStartTraining(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDecomp(t, s, "明", "⿰日月")
	seedHanziDef(t, s, "日", "sun; day")
	seedHanziDef(t, s, "月", "moon; month")

	id, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:        "明",
		Translations:  map[string][]string{"en": {"bright"}},
		StartTraining: false,
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	var firstSeen *string
	if err := s.db.QueryRowContext(ctx,
		`SELECT first_seen_at FROM sm2_progress WHERE word_id = ?`, id).Scan(&firstSeen); err != nil {
		t.Fatalf("read first_seen_date: %v", err)
	}
	if firstSeen != nil {
		t.Errorf("StartTraining=false should leave first_seen_date NULL, got %q", *firstSeen)
	}
	var comps int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM component_progress WHERE user_id = 2`).Scan(&comps)
	if comps != 0 {
		t.Errorf("StartTraining=false should create no component rows, got %d", comps)
	}
}

// setWordFrequency inserts (or replaces) a row in word_frequency for use in
// ordering tests — mirrors what cmd/import-frequency would populate.
func setWordFrequency(t *testing.T, s *Store, word string, rank int) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		`INSERT INTO word_frequency (word, rank) VALUES (?, ?)
		 ON CONFLICT(word) DO UPDATE SET rank = excluded.rank`, word, rank); err != nil {
		t.Fatalf("setWordFrequency(%q, %d): %v", word, rank, err)
	}
}

// TestGetNextCard_PrefersComponentWordsOverCompound verifies the compound-prerequisite
// rule from issue #340: when an unseen compound word (e.g. 还可以) is made up of
// substrings that are themselves existing, not-yet-introduced zh words for the same
// user (还 and 可以), GetNextCard should introduce a component word first rather than
// the compound.
func TestGetNextCard_PrefersComponentWordsOverCompound(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	compoundID := seedWord(t, s, "还可以", "hái kěyǐ", []string{"okay"})
	part1ID := seedWord(t, s, "还", "hái", []string{"still"})
	part2ID := seedWord(t, s, "可以", "kěyǐ", []string{"can"})

	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil {
		t.Fatal("expected a card, got nil")
	}
	if w.ID == compoundID {
		t.Errorf("expected a component word to be introduced before the compound, got compound %q", w.Text)
	}
	if w.ID != part1ID && w.ID != part2ID {
		t.Errorf("expected one of the compound's component words (id=%d or id=%d), got id=%d (%q)", part1ID, part2ID, w.ID, w.Text)
	}
}

// TestGetNextCard_IntroducesCompoundOnceComponentsAreSeen verifies that once all
// of a compound word's component words have been introduced (first_seen_at set),
// the compound is no longer deprioritized.
func TestGetNextCard_IntroducesCompoundOnceComponentsAreSeen(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	compoundID := seedWord(t, s, "还可以", "hái kěyǐ", []string{"okay"})
	part1ID := seedWord(t, s, "还", "hái", []string{"still"})
	part2ID := seedWord(t, s, "可以", "kěyǐ", []string{"can"})
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET first_seen_at = datetime('now') WHERE word_id IN (?, ?)`, part1ID, part2ID)

	w, _, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil || w.ID != compoundID {
		t.Fatalf("expected the compound word (id=%d) once components are seen, got %v", compoundID, w)
	}
}

// TestGetNextCard_FrequencyRankTiebreak verifies that among unseen words with no
// compound-prerequisite relationship, the word with the lower (more frequent)
// frequency_rank is introduced first.
func TestGetNextCard_FrequencyRankTiebreak(t *testing.T) {
	s := openTestDB(t)

	rareID := seedWord(t, s, "罕见词", "hǎn jiàn cí", []string{"rare word"})
	commonID := seedWord(t, s, "常见词", "cháng jiàn cí", []string{"common word"})
	setWordFrequency(t, s, "罕见词", 5000)
	setWordFrequency(t, s, "常见词", 10)

	w, _, _, err := s.GetNextCard(context.Background(), int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil || w.ID != commonID {
		t.Fatalf("expected the more frequent word (id=%d) to be introduced first, got %v (rare id=%d)", commonID, w, rareID)
	}
}

// TestGetNextCard_UnrankedWordsSortAfterRankedWords verifies that a word with no
// frequency_rank entry (e.g. not present in the imported frequency list) is not
// preferred over a ranked word.
func TestGetNextCard_UnrankedWordsSortAfterRankedWords(t *testing.T) {
	s := openTestDB(t)

	unrankedID := seedWord(t, s, "生僻字", "shēng pì zì", []string{"obscure character"})
	rankedID := seedWord(t, s, "常见词", "cháng jiàn cí", []string{"common word"})
	setWordFrequency(t, s, "常见词", 42)

	w, _, _, err := s.GetNextCard(context.Background(), int64(2), nil, 100, "", false, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil || w.ID != rankedID {
		t.Fatalf("expected the ranked word (id=%d) before the unranked word (id=%d), got %v", rankedID, unrankedID, w)
	}
}

func wordTagsForText(t *testing.T, s *Store, userID int64, text string) []string {
	t.Helper()
	rows, err := s.db.Query(`
		SELECT tg.name FROM words w
		JOIN word_tags wt ON wt.word_id = w.id
		JOIN tags tg ON tg.id = wt.tag_id
		WHERE w.user_id = ? AND w.text = ? AND w.language = 'zh'
		ORDER BY tg.name`, userID, text)
	if err != nil {
		t.Fatalf("query word tags: %v", err)
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			t.Fatalf("scan tag: %v", err)
		}
		tags = append(tags, tag)
	}
	return tags
}

func enTranslationForText(t *testing.T, s *Store, userID int64, text string) string {
	t.Helper()
	var en string
	err := s.db.QueryRow(`
		SELECT ew.text FROM words w
		JOIN translations t ON t.zh_word_id = w.id
		JOIN words ew ON ew.id = t.translation_word_id AND ew.language = 'en'
		WHERE w.user_id = ? AND w.text = ? AND w.language = 'zh'`, userID, text).Scan(&en)
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("query en translation: %v", err)
	}
	return en
}

func TestCreateWord_StartTraining_MultiCharWord_AddsTopLevelCharAndSubword(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "踢", "to kick")
	if err := s.SeedCedictEntryForTest(ctx, "足球", "en", "zú qiú", "football"); err != nil {
		t.Fatal(err)
	}

	_, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:        "踢足球",
		Translations:  map[string][]string{"en": {"play football"}},
		Tags:          []string{"HSK1"},
		StartTraining: true,
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	var compCount int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM component_progress WHERE user_id = 2 AND character = '踢'`).Scan(&compCount)
	if compCount != 1 {
		t.Errorf("want a component_progress row for 踢, got count=%d", compCount)
	}

	exists, err := s.IsZhWordForUser(ctx, 2, "足球")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("want 足球 auto-created as a zh word")
	}
	if got := wordTagsForText(t, s, 2, "足球"); len(got) != 1 || got[0] != "HSK1" {
		t.Errorf("want 足球 tagged [HSK1], got %v", got)
	}
	if got := enTranslationForText(t, s, 2, "足球"); got != "football" {
		t.Errorf("want 足球's EN translation \"football\", got %q", got)
	}

	var firstSeen *string
	s.db.QueryRowContext(ctx, `
		SELECT p.first_seen_at FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.user_id = 2 AND w.text = '足球' AND w.language = 'zh'`).Scan(&firstSeen)
	if firstSeen != nil {
		t.Errorf("auto-created subword must stay inert (unacknowledged), got first_seen_at=%q", *firstSeen)
	}
}

func TestCreateWord_StartTraining_Subword_MultipleParentTags(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "踢", "to kick")
	if err := s.SeedCedictEntryForTest(ctx, "足球", "en", "zú qiú", "football"); err != nil {
		t.Fatal(err)
	}

	_, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:        "踢足球",
		Translations:  map[string][]string{"en": {"play football"}},
		Tags:          []string{"HSK1", "sports"},
		StartTraining: true,
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	got := wordTagsForText(t, s, 2, "足球")
	want := map[string]bool{"HSK1": true, "sports": true}
	if len(got) != 2 || !want[got[0]] || !want[got[1]] {
		t.Errorf("want 足球 tagged [HSK1 sports], got %v", got)
	}
}

func TestCreateWord_StartTraining_Subword_NoParentTags_GetsGenericSubTag(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "踢", "to kick")
	if err := s.SeedCedictEntryForTest(ctx, "足球", "en", "zú qiú", "football"); err != nil {
		t.Fatal(err)
	}

	_, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:        "踢足球",
		Translations:  map[string][]string{"en": {"play football"}},
		StartTraining: true,
	})
	if err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	if got := wordTagsForText(t, s, 2, "足球"); len(got) != 0 {
		t.Errorf("want 足球 untagged (parent has no tags), got %v", got)
	}
}

func TestCreateWord_StartTraining_Subword_SkippedWhenAlreadyOwnWord(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "踢", "to kick")
	if err := s.SeedCedictEntryForTest(ctx, "足球", "en", "zú qiú", "football"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:        "足球",
		Translations:  map[string][]string{"en": {"soccer"}},
		Tags:          []string{"favourites"},
		StartTraining: false,
	}); err != nil {
		t.Fatalf("seed existing 足球: %v", err)
	}

	if _, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:        "踢足球",
		Translations:  map[string][]string{"en": {"play football"}},
		Tags:          []string{"HSK1"},
		StartTraining: true,
	}); err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	var count int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM words WHERE user_id = 2 AND text = '足球' AND language = 'zh'`).Scan(&count)
	if count != 1 {
		t.Errorf("want exactly 1 zh word for 足球 (no duplicate created), got %d", count)
	}
	if got := wordTagsForText(t, s, 2, "足球"); len(got) != 1 || got[0] != "favourites" {
		t.Errorf("existing 足球's tags must be untouched, got %v", got)
	}
}

func TestCreateWord_StartTraining_NoCedictData_TopLevelCharsBecomeComponents(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDecomp(t, s, "明", "⿰日月")
	seedHanziDef(t, s, "明", "bright")
	seedHanziDef(t, s, "日", "sun; day")
	seedHanziDef(t, s, "月", "moon; month")

	if _, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:        "明月",
		Translations:  map[string][]string{"en": {"bright moon"}},
		StartTraining: true,
	}); err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	for _, char := range []string{"明", "月", "日"} {
		var count int
		s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM component_progress WHERE user_id = 2 AND character = ?`, char).Scan(&count)
		if count != 1 {
			t.Errorf("want a component_progress row for %q, got count=%d", char, count)
		}
	}

	exists, err := s.IsZhWordForUser(ctx, 2, "月")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("no cedict data means no sub-word should be auto-created")
	}
}

func TestAcknowledgeRandomWords_CreatesSubwords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedHanziDef(t, s, "踢", "to kick")
	if err := s.SeedCedictEntryForTest(ctx, "足球", "en", "zú qiú", "football"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText:       "踢足球",
		Translations: map[string][]string{"en": {"play football"}},
		Tags:         []string{"HSK1"},
	}); err != nil {
		t.Fatalf("seed word: %v", err)
	}

	n, err := s.AcknowledgeRandomWords(ctx, 2, 1)
	if err != nil {
		t.Fatalf("AcknowledgeRandomWords: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 word acknowledged, got %d", n)
	}

	exists, err := s.IsZhWordForUser(ctx, 2, "足球")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("want 足球 auto-created via AcknowledgeRandomWords")
	}
	if got := wordTagsForText(t, s, 2, "足球"); len(got) != 1 || got[0] != "HSK1" {
		t.Errorf("want 足球 tagged [HSK1], got %v", got)
	}
}
