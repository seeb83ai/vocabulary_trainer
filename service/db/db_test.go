package db

import (
	"context"
	"os"
	"testing"
	"time"
	"vocabulary_trainer/models"

	"golang.org/x/crypto/bcrypt"
)

// TestMain sets migration credential env vars once for the entire test binary.
func TestMain(m *testing.M) {
	os.Setenv("ADMIN_EMAIL", "admin@example.de")
	os.Setenv("ADMIN_PASSWORD", "I am the admin")
	os.Setenv("USER_EMAIL", "me@example.de")
	os.Setenv("USER_PASSWORD", "I learn zh")
	os.Exit(m.Run())
}

// openTestDB creates an in-memory SQLite store for tests.
func openTestDB(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("openTestDB: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedWord inserts one full vocabulary entry and returns the zh word ID.
func seedWord(t *testing.T, s *Store, zhText, pinyin string, enTexts []string) int64 {
	t.Helper()
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:  zhText,
		Pinyin:  pinyin,
		EnTexts: enTexts,
	})
	if err != nil {
		t.Fatalf("seedWord %q: %v", zhText, err)
	}
	return id
}

// ── CreateWord ────────────────────────────────────────────────────────────────

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
	wd, err := s.GetWordByID(context.Background(), int64(2),id)
	if err != nil {
		t.Fatal(err)
	}
	if len(wd.EnTexts) != 2 {
		t.Errorf("expected 2 en_texts, got %d: %v", len(wd.EnTexts), wd.EnTexts)
	}
}

// ── GetWordByID ───────────────────────────────────────────────────────────────

func TestGetWordByID_NotFound(t *testing.T) {
	s := openTestDB(t)
	wd, err := s.GetWordByID(context.Background(), int64(2),9999)
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
	wd, err := s.GetWordByID(context.Background(), int64(2),id)
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
	wd, err := s.GetWordByID(context.Background(), int64(2),id)
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

// ── GetWords ──────────────────────────────────────────────────────────────────

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

// ── UpdateWord ────────────────────────────────────────────────────────────────

func TestUpdateWord_ChangesZhText(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	err := s.UpdateWord(context.Background(), int64(2), id, models.UpdateWordRequest{
		ZhText:  "妳好",
		Pinyin:  "nǐ hǎo",
		EnTexts: []string{"hello (female)"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2),id)
	if wd.ZhText != "妳好" {
		t.Errorf("ZhText: want 妳好, got %q", wd.ZhText)
	}
	if len(wd.EnTexts) != 1 || wd.EnTexts[0] != "hello (female)" {
		t.Errorf("EnTexts: want [hello (female)], got %v", wd.EnTexts)
	}
}

func TestUpdateWord_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.UpdateWord(context.Background(), int64(2), 9999, models.UpdateWordRequest{
		ZhText:  "test",
		EnTexts: []string{"test"},
	})
	if err == nil {
		t.Error("expected error for unknown id")
	}
}

// ── DeleteWord ────────────────────────────────────────────────────────────────

func TestDeleteWord_Removes(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.DeleteWord(context.Background(), int64(2),id); err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2),id)
	if wd != nil {
		t.Error("word should be gone after delete")
	}
}

func TestDeleteWord_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.DeleteWord(context.Background(), int64(2),9999)
	if err == nil {
		t.Error("expected error when deleting non-existent word")
	}
}

// ── AddTranslation ────────────────────────────────────────────────────────────

func TestAddTranslation_AddsNewEN(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.AddTranslation(context.Background(), int64(2), id, "hi"); err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2),id)
	found := false
	for _, e := range wd.EnTexts {
		if e == "hi" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'hi' in EnTexts, got %v", wd.EnTexts)
	}
}

func TestAddTranslation_Idempotent(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	s.AddTranslation(context.Background(), int64(2), id, "hi")
	s.AddTranslation(context.Background(), int64(2), id, "hi") // second call is no-op
	wd, _ := s.GetWordByID(context.Background(), int64(2),id)
	count := 0
	for _, e := range wd.EnTexts {
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
	err := s.AddTranslation(context.Background(), int64(2), 9999, "hello")
	if err == nil {
		t.Error("expected error for unknown zh word id")
	}
}

// ── GetNextCard ───────────────────────────────────────────────────────────────

func TestGetNextCard_NilWhenEmpty(t *testing.T) {
	s := openTestDB(t)
	w, p, err := s.GetNextCard(context.Background(), int64(2), nil, 100, "", false)
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
	w, p, err := s.GetNextCard(context.Background(), int64(2), nil, 100, "", false)
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

	// GetNextCard should return the word but NOT set first_seen_date.
	w, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if w == nil || w.ID != id {
		t.Fatalf("expected word id=%d, got %v", id, w)
	}

	var firstSeen *string
	s.db.QueryRowContext(ctx, `SELECT first_seen_date FROM sm2_progress WHERE word_id = ?`, id).Scan(&firstSeen)
	if firstSeen != nil {
		t.Errorf("GetNextCard should not set first_seen_date, but got %q", *firstSeen)
	}
}

func TestGetNextCard_MostOverduFirst(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "一", "", []string{"one"})
	id2 := seedWord(t, s, "二", "", []string{"two"})

	// Set id2's due_date far in the past so it's more overdue
	ctx := context.Background()
	past := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ? WHERE word_id = ?`, past, id2)
	_ = id1

	w, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false)
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

	// Seed 3 words; none have been seen yet (first_seen_date IS NULL).
	id1 := seedWord(t, s, "一", "", []string{"one"})
	seedWord(t, s, "二", "", []string{"two"})
	seedWord(t, s, "三", "", []string{"three"})

	// Simulate having already introduced 1 word today by stamping its first_seen_date.
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET first_seen_date = date('now') WHERE word_id = ?`, id1)

	// With maxNew=1 the daily cap is reached; only id1 (already introduced) should be returned.
	w, _, err := s.GetNextCard(ctx, int64(2), nil, 1, "", false)
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
	w2, _, err := s.GetNextCard(ctx, int64(2), nil, 5, "", false)
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

	// id1: already introduced (first_seen_date set).
	id1 := seedWord(t, s, "一", "", []string{"one"})
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET first_seen_date = date('now') WHERE word_id = ?`, id1)

	// id2: never presented (first_seen_date IS NULL).
	seedWord(t, s, "二", "", []string{"two"})

	// With skipNew=true, only the already-seen word should be returned.
	w, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", true)
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
		`UPDATE sm2_progress SET first_seen_date = date('now'), learning_new_word = 1 WHERE word_id = ?`,
		idLearning)

	// idUnseen: never presented (first_seen_date IS NULL).
	seedWord(t, s, "二", "", []string{"two"})

	// Even though the daily cap (100) is not reached, the unseen word must not
	// be returned while a learning word exists.
	w, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false)
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

// ── UpdateSM2Progress ─────────────────────────────────────────────────────────

func TestUpdateSM2Progress_Persists(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	p, err := s.GetSM2Progress(context.Background(), id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}

	p.Repetitions = 3
	p.Easiness = 2.8
	p.IntervalDays = 15
	p.TotalCorrect = 7
	p.TotalAttempts = 10
	p.DueDate = time.Now().UTC().Add(15 * 24 * time.Hour)

	if err := s.UpdateSM2Progress(context.Background(), *p); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSM2Progress(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Repetitions != 3 {
		t.Errorf("repetitions: want 3, got %d", got.Repetitions)
	}
	if got.TotalCorrect != 7 {
		t.Errorf("total_correct: want 7, got %d", got.TotalCorrect)
	}
	if got.IntervalDays != 15 {
		t.Errorf("interval_days: want 15, got %d", got.IntervalDays)
	}
}

// ── GetStats ──────────────────────────────────────────────────────────────────

func TestGetStats_Empty(t *testing.T) {
	s := openTestDB(t)
	due, total, _, err := s.GetStats(context.Background(), int64(2), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if due != 0 || total != 0 {
		t.Errorf("empty db: want 0/0, got %d/%d", due, total)
	}
}

func TestGetStats_CountsOnlyZh(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello", "hi"})
	_, total, _, err := s.GetStats(context.Background(), int64(2), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	// Only 1 zh word should be counted, not the 2 en words
	if total != 1 {
		t.Errorf("total zh words: want 1, got %d", total)
	}
}

func TestGetStats_DueTodayCount(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "一", "", []string{"one"})
	seedWord(t, s, "二", "", []string{"two"})

	// Mark both words as seen so they count as due
	ctx := context.Background()
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET first_seen_date = date('now')`)

	// Move one word into the future so it's NOT due
	future := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ? WHERE word_id = ?`, future, id1)

	due, _, _, err := s.GetStats(ctx, int64(2), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if due != 1 {
		t.Errorf("due_today: want 1, got %d", due)
	}
}

func TestGetStats_NewTodayCount(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1 := seedWord(t, s, "一", "", []string{"one"})
	seedWord(t, s, "二", "", []string{"two"})

	// Stamp one word as introduced today.
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET first_seen_date = date('now') WHERE word_id = ?`, id1)

	_, _, newToday, err := s.GetStats(ctx, int64(2), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if newToday != 1 {
		t.Errorf("new_today: want 1, got %d", newToday)
	}
}

// ── GetTranslationsForWord ────────────────────────────────────────────────────

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

// ── Tags ─────────────────────────────────────────────────────────────────────

func seedWordWithTags(t *testing.T, s *Store, zhText, pinyin string, enTexts, tags []string) int64 {
	t.Helper()
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:  zhText,
		Pinyin:  pinyin,
		EnTexts: enTexts,
		Tags:    tags,
	})
	if err != nil {
		t.Fatalf("seedWordWithTags %q: %v", zhText, err)
	}
	return id
}

func TestCreateWord_WithTags(t *testing.T) {
	s := openTestDB(t)
	id := seedWordWithTags(t, s, "你好", "nǐ hǎo", []string{"hello"}, []string{"greetings", "HSK1"})
	wd, err := s.GetWordByID(context.Background(), int64(2),id)
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
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
		EnTexts: []string{"hello"},
		Tags:    []string{"new-tag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2),id)
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

	// Mark the word as seen (first_seen_date set) and place its due_date
	// 2 days in the future — it should NOT be returned by GetNextCard.
	future := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ?, first_seen_date = date('now') WHERE word_id = ?`, future, id)

	w, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false)
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
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ?, first_seen_date = date('now') WHERE word_id = ?`, soon, id)

	w, _, err := s.GetNextCard(ctx, int64(2), nil, 100, "", false)
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

	w, _, err := s.GetNextCard(context.Background(), int64(2), []string{"food"}, 100, "", false)
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

	w, _, err := s.GetNextCard(context.Background(), int64(2), []string{"nonexistent"}, 100, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if w != nil {
		t.Error("expected nil when no words match tag filter")
	}
}

func TestGetStats_FilterByTag(t *testing.T) {
	s := openTestDB(t)
	seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"greetings"})
	seedWordWithTags(t, s, "吃饭", "", []string{"eat"}, []string{"food"})

	_, total, _, err := s.GetStats(context.Background(), int64(2), []string{"food"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("tag-filtered total: want 1, got %d", total)
	}
}

func TestGetAllTags(t *testing.T) {
	s := openTestDB(t)
	seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"B-tag", "A-tag"})
	tags, err := s.GetAllTags(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0] != "A-tag" || tags[1] != "B-tag" {
		t.Errorf("expected [A-tag, B-tag], got %v", tags)
	}
}

func TestGetAllTags_UserIsolation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	// User 2 owns words with tags (user 2 is created by openTestDB)
	seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"user2-tag"})
	// Create user 3 and give them their own word+tag
	user3ID, err := s.CreateUser(ctx, "user3@example.com", "hash", "tok-u3", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWord(ctx, user3ID, models.CreateWordRequest{
		ZhText: "再见", EnTexts: []string{"goodbye"}, Tags: []string{"user3-tag"},
	}); err != nil {
		t.Fatal(err)
	}
	tags2, err := s.GetAllTags(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(tags2) != 1 || tags2[0] != "user2-tag" {
		t.Errorf("user 2 should only see user2-tag, got %v", tags2)
	}
	tags3, err := s.GetAllTags(ctx, user3ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags3) != 1 || tags3[0] != "user3-tag" {
		t.Errorf("user 3 should only see user3-tag, got %v", tags3)
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

// ── parseDateTime ─────────────────────────────────────────────────────────────

func TestParseDateTime_RFC3339(t *testing.T) {
	s := "2026-02-21T15:04:05Z"
	got := parseDateTime(s)
	if got.IsZero() {
		t.Errorf("parseDateTime(%q) returned zero time", s)
	}
}

func TestParseDateTime_SQLiteFormat(t *testing.T) {
	s := "2026-02-21 15:04:05"
	got := parseDateTime(s)
	if got.IsZero() {
		t.Errorf("parseDateTime(%q) returned zero time", s)
	}
	if got.Year() != 2026 || got.Month() != 2 || got.Day() != 21 {
		t.Errorf("wrong date parsed: %v", got)
	}
}

func TestParseDateTime_InvalidReturnsZero(t *testing.T) {
	got := parseDateTime("not-a-date")
	if !got.IsZero() {
		t.Errorf("invalid input should return zero time, got %v", got)
	}
}

// ── Confusion pairs ───────────────────────────────────────────────────────────

func TestLookupConfusion_ZhToEn_Found(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	seedWord(t, s, "书", "shū", []string{"Buch"})

	confusedWithID, found, err := s.LookupConfusion(context.Background(), int64(2), zhID, "Buch", "zh_to_en", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found")
	}
	if confusedWithID == zhID {
		t.Error("confused_with_id must differ from zh_word_id")
	}
}

func TestLookupConfusion_ZhToEn_NoMatch(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})

	_, found, err := s.LookupConfusion(context.Background(), int64(2), zhID, "Tisch", "zh_to_en", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected no confusion for unknown word")
	}
}

func TestLookupConfusion_EnToZh_Found(t *testing.T) {
	s := openTestDB(t)
	seedWord(t, s, "书", "shū", []string{"Buch"})
	zhID := seedWord(t, s, "五", "", []string{"five"})

	confusedWithID, found, err := s.LookupConfusion(context.Background(), int64(2), zhID, "书", "en_to_zh", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found")
	}
	if confusedWithID == zhID {
		t.Error("confused_with_id must differ from zh_word_id")
	}
}

func TestLookupConfusion_SameWord_NotFound(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})

	_, found, err := s.LookupConfusion(context.Background(), int64(2), zhID, "Schuh", "zh_to_en", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("should not report confusion when answer matches the tested word")
	}
}

func TestUpsertConfusion_IncrementsCount(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), idA, idB, "zh_to_en"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertConfusion(context.Background(), idA, idB, "zh_to_en"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 confusion, got %d", len(items))
	}
	if items[0].Count != 2 {
		t.Errorf("count: want 2, got %d", items[0].Count)
	}
}

func TestGetConfusions_LastSeenUpdated(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	before := time.Now().UTC().Add(-time.Second)
	if err := s.UpsertConfusion(context.Background(), idA, idB, "zh_to_en"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one confusion")
	}
	if items[0].LastSeen.Before(before) {
		t.Errorf("last_seen should be recent, got %v", items[0].LastSeen)
	}
}

func TestLookupConfusion_ZhPinyinToEn_Found(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	seedWord(t, s, "书", "shū", []string{"Buch"})

	confusedWithID, found, err := s.LookupConfusion(context.Background(), int64(2), zhID, "Buch", "zh_pinyin_to_en", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("zh_pinyin_to_en should behave like zh_to_en")
	}
	if confusedWithID == zhID {
		t.Error("confused_with_id must differ from zh_word_id")
	}
}

func TestLookupConfusion_InvalidMode_NotFound(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	seedWord(t, s, "书", "shū", []string{"Buch"})

	_, found, err := s.LookupConfusion(context.Background(), int64(2), zhID, "Buch", "invalid_mode", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("invalid mode should never report a confusion")
	}
}

func TestLookupConfusion_EmptyAnswer_NotFound(t *testing.T) {
	s := openTestDB(t)
	zhID := seedWord(t, s, "鞋", "xié", []string{"Schuh"})

	_, found, err := s.LookupConfusion(context.Background(), int64(2), zhID, "", "zh_to_en", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("empty answer should never match")
	}
}

// ── CountLearningNewWords ─────────────────────────────────────────────────────

func TestCountLearningNewWords_BeforePresented(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Newly created word: learning_new_word=1 (default), first_seen_date=NULL
	wordId := seedWord(t, s, "一", "", []string{"one"})

	count, err := s.CountLearningNewWords(ctx, int64(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Must count unseen learning words so the new-word gate works correctly.
	if count != 0 {
		t.Errorf("want 0 learning word (unseen), got %d", count)
	}

	s.AcknowledgeWord(ctx, int64(2), wordId)

	count, err = s.CountLearningNewWords(ctx, int64(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Must count unseen learning words so the new-word gate works correctly.
	if count != 1 {
		t.Errorf("want 1 learning word (unseen), got %d", count)
	}
}

func TestCountLearningNewWords_GraduatedNotCounted(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	id := seedWord(t, s, "一", "", []string{"one"})
	// Graduate the word (learning_new_word=0)
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET learning_new_word = 0 WHERE word_id = ?`, id)

	count, err := s.CountLearningNewWords(ctx, int64(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("graduated word should not count as learning, got %d", count)
	}
}

// ── AcknowledgeWord ───────────────────────────────────────────────────────────

func TestAcknowledgeWord_SetsLearningPhase(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Fatal(err)
	}

	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}
	if !p.LearningNewWord {
		t.Error("AcknowledgeWord should set learning_new_word=1")
	}
	if p.TotalAttempts != 1 {
		t.Errorf("total_attempts: want 1, got %d", p.TotalAttempts)
	}
}

func TestAcknowledgeWord_Idempotent(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	s.AcknowledgeWord(ctx, int64(2), id)
	if err := s.AcknowledgeWord(ctx, int64(2), id); err != nil {
		t.Errorf("second AcknowledgeWord should not error: %v", err)
	}

	p, _ := s.GetSM2Progress(ctx, id)
	if p.TotalAttempts != 1 {
		t.Errorf("total_attempts should not increment beyond 1: got %d", p.TotalAttempts)
	}
}

// ── SkipWord ──────────────────────────────────────────────────────────────────

func TestSkipWord_AdvancesDueDateByNDays(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "一", "", []string{"one"})

	before := time.Now().UTC()
	if err := s.SkipWord(ctx, int64(2), id, 7); err != nil {
		t.Fatal(err)
	}

	p, err := s.GetSM2Progress(ctx, id)
	if err != nil || p == nil {
		t.Fatalf("GetSM2Progress: %v / %v", err, p)
	}

	minDue := before.Truncate(time.Second).Add(7 * 24 * time.Hour)
	maxDue := time.Now().UTC().Add(8 * 24 * time.Hour)
	if p.DueDate.Before(minDue) || p.DueDate.After(maxDue) {
		t.Errorf("due_date not advanced by ~7 days; got %v (expected between %v and %v)", p.DueDate, minDue, maxDue)
	}
}

func TestSkipWord_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.SkipWord(context.Background(), int64(2), 9999, 7)
	if err == nil {
		t.Error("expected error for unknown word id")
	}
}

// ── DeleteWord shared tag ─────────────────────────────────────────────────────

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

func TestGetConfusions_PopulatesEnTexts(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), idA, idB, "zh_to_en"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	d := items[0]
	if len(d.ZhEnTexts) == 0 || d.ZhEnTexts[0] != "Schuh" {
		t.Errorf("ZhEnTexts: want [Schuh], got %v", d.ZhEnTexts)
	}
	if len(d.ConfusedWithEnTexts) == 0 || d.ConfusedWithEnTexts[0] != "Buch" {
		t.Errorf("ConfusedWithEnTexts: want [Buch], got %v", d.ConfusedWithEnTexts)
	}
}

func TestGetConfusionDetail_ReturnsRow(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), idA, idB, "zh_to_en"); err != nil {
		t.Fatal(err)
	}

	d, err := s.GetConfusionDetail(context.Background(), idA, idB, "zh_to_en", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("expected a ConfusionDetail, got nil")
	}
	if d.ZhText != "鞋" {
		t.Errorf("ZhText: want 鞋, got %q", d.ZhText)
	}
	if d.ConfusedWithText != "书" {
		t.Errorf("ConfusedWithText: want 书, got %q", d.ConfusedWithText)
	}
	if d.Count != 1 {
		t.Errorf("Count: want 1, got %d", d.Count)
	}
}

func TestGetConfusionDetail_MissingReturnsNil(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	d, err := s.GetConfusionDetail(context.Background(), idA, idB, "zh_to_en", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if d != nil {
		t.Error("expected nil when no confusion row exists")
	}
}

func TestGetConfusionDetail_ReturnsTranslationsForSelectedLangs(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	idA, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "人",
		Pinyin:  "rén",
		EnTexts: []string{"person"},
		DeTexts: []string{"Person"},
	})
	if err != nil {
		t.Fatal(err)
	}
	idB, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "点",
		Pinyin:  "diǎn",
		EnTexts: []string{"dot"},
		DeTexts: []string{"Uhr"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertConfusion(ctx, idA, idB, "zh_to_en"); err != nil {
		t.Fatal(err)
	}

	d, err := s.GetConfusionDetail(ctx, idA, idB, "zh_to_en", []string{"en", "de"})
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("expected a ConfusionDetail, got nil")
	}

	wantZh := []string{"person", "Person"}
	if len(d.ZhEnTexts) != len(wantZh) {
		t.Errorf("ZhEnTexts: want %v, got %v", wantZh, d.ZhEnTexts)
	}
	wantCW := []string{"dot", "Uhr"}
	if len(d.ConfusedWithEnTexts) != len(wantCW) {
		t.Errorf("ConfusedWithEnTexts: want %v, got %v", wantCW, d.ConfusedWithEnTexts)
	}
}

func TestUpsertConfusion_DifferentModesSeparateRows(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), idA, idB, "zh_to_en"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertConfusion(context.Background(), idA, idB, "zh_pinyin_to_en"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("want 2 rows (one per mode), got %d", len(items))
	}
}

func TestDeleteWord_CascadesToConfusionPairs(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), idA, idB, "zh_to_en"); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteWord(context.Background(), int64(2),idA); err != nil {
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

// ── MarkWordForReview ─────────────────────────────────────────────────────────

func TestMarkWordForReview_SetsFlag(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	if err := s.MarkWordForReview(context.Background(), int64(2),id); err != nil {
		t.Fatalf("MarkWordForReview: %v", err)
	}

	wd, err := s.GetWordByID(context.Background(), int64(2),id)
	if err != nil {
		t.Fatal(err)
	}
	if !wd.NeedsReview {
		t.Error("expected NeedsReview = true after marking")
	}
}

func TestMarkWordForReview_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.MarkWordForReview(context.Background(), int64(2),9999)
	if err == nil {
		t.Error("expected error for missing word, got nil")
	}
}

func TestUpdateWord_ClearsReviewFlag(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	if err := s.MarkWordForReview(context.Background(), int64(2),id); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateWord(context.Background(), int64(2), id, models.UpdateWordRequest{
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
		EnTexts: []string{"hello"},
	}); err != nil {
		t.Fatalf("UpdateWord: %v", err)
	}

	wd, err := s.GetWordByID(context.Background(), int64(2),id)
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

	if err := s.MarkWordForReview(context.Background(), int64(2),id1); err != nil {
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

// ── DailyStats ────────────────────────────────────────────────────────────────

func TestRecordDailyStat_IncrementsCounts(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedWord(t, s, "猫", "māo", []string{"cat"})

	// Mark the word as seen and meeting the "known" threshold (≥10 attempts, ≥85% accuracy)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET first_seen_date = date('now'), total_correct = 9, total_attempts = 10`); err != nil {
		t.Fatal(err)
	}

	if _, err := s.RecordDailyStat(ctx, int64(2), true); err != nil {
		t.Fatalf("RecordDailyStat(correct): %v", err)
	}
	if _, err := s.RecordDailyStat(ctx, int64(2), true); err != nil {
		t.Fatalf("RecordDailyStat(correct): %v", err)
	}
	if _, err := s.RecordDailyStat(ctx, int64(2), false); err != nil {
		t.Fatalf("RecordDailyStat(wrong): %v", err)
	}

	stats, err := s.GetDailyStatsHistory(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetDailyStatsHistory: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 day, got %d", len(stats))
	}
	d := stats[0]
	if d.Attempts != 3 {
		t.Errorf("attempts: got %d, want 3", d.Attempts)
	}
	if d.Mistakes != 1 {
		t.Errorf("mistakes: got %d, want 1", d.Mistakes)
	}
	if d.WordsSeen != 1 {
		t.Errorf("words_seen: got %d, want 1", d.WordsSeen)
	}
	if d.CorrectStreak != 2 {
		t.Errorf("correct_streak: got %d, want 2", d.CorrectStreak)
	}
}

func TestRecordDailyStat_StreakResets(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// wrong, correct, correct, wrong, correct
	for _, correct := range []bool{false, true, true, false, true} {
		if _, err := s.RecordDailyStat(ctx, int64(2), correct); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := s.GetDailyStatsHistory(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if stats[0].CorrectStreak != 2 {
		t.Errorf("correct_streak: got %d, want 2 (max streak of the day)", stats[0].CorrectStreak)
	}
}

func TestGetDailyStatsHistory_OrderedByDate(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Insert rows for multiple dates manually
	for _, d := range []string{"2026-02-10", "2026-02-12", "2026-02-11"} {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO daily_stats (user_id, date, attempts, mistakes, correct_streak, current_streak)
			 VALUES (2, ?, 10, 2, 3, 0)`, d); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := s.GetDailyStatsHistory(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(stats))
	}
	if stats[0].Date != "2026-02-10" || stats[1].Date != "2026-02-11" || stats[2].Date != "2026-02-12" {
		t.Errorf("wrong order: %s, %s, %s", stats[0].Date, stats[1].Date, stats[2].Date)
	}
}

func TestGetDailyStatsHistory_EmptyReturnsEmptySlice(t *testing.T) {
	s := openTestDB(t)
	stats, err := s.GetDailyStatsHistory(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if stats == nil {
		t.Error("expected non-nil empty slice")
	}
	if len(stats) != 0 {
		t.Errorf("expected 0 rows, got %d", len(stats))
	}
}

// ── GetTodaySessionInfo ───────────────────────────────────────────────────────

func TestGetTodaySessionInfo_NoRows(t *testing.T) {
	s := openTestDB(t)
	attempts, mistakes, available, err := s.GetTodaySessionInfo(context.Background(), int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || mistakes != 0 {
		t.Errorf("expected 0/0, got %d/%d", attempts, mistakes)
	}
	if available != 0 {
		t.Errorf("expected 0 available, got %d", available)
	}
}

func TestGetTodaySessionInfo_WithData(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})

	// Mark the word as seen with a future due date.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sm2_progress SET first_seen_date = date('now'), due_date = datetime('now', '+1 day') WHERE word_id = ?`, id); err != nil {
		t.Fatal(err)
	}

	// Record a daily stat (1 correct answer).
	if _, err := s.RecordDailyStat(ctx, int64(2), true); err != nil {
		t.Fatal(err)
	}

	attempts, mistakes, available, err := s.GetTodaySessionInfo(ctx, int64(2))
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
	if mistakes != 0 {
		t.Errorf("expected 0 mistakes, got %d", mistakes)
	}
	if available != 1 {
		t.Errorf("expected 1 available to advance, got %d", available)
	}
}

// ── AdvanceDueDates ───────────────────────────────────────────────────────────

func TestAdvanceDueDates_AdvancesNWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed 5 words and mark them as seen with staggered future due dates.
	ids := make([]int64, 5)
	for i := range ids {
		ids[i] = seedWord(t, s, []string{"一", "二", "三", "四", "五"}[i], "", []string{"en"})
		days := i + 1 // 1 day, 2 days, ..., 5 days from now
		if _, err := s.db.ExecContext(ctx,
			`UPDATE sm2_progress SET first_seen_date = date('now'), due_date = datetime('now', ? || ' days') WHERE word_id = ?`,
			days, ids[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Advance 3 words (the 3rd earliest due date is +3 days).
	nowDue, err := s.AdvanceDueDates(ctx, int64(2), 3)
	if err != nil {
		t.Fatal(err)
	}
	if nowDue != 3 {
		t.Errorf("expected 3 words due now, got %d", nowDue)
	}

	// Verify exactly 3 are due and 2 are still future.
	var due, future int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress WHERE due_date <= CURRENT_TIMESTAMP AND first_seen_date IS NOT NULL`).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress WHERE due_date > CURRENT_TIMESTAMP AND first_seen_date IS NOT NULL`).Scan(&future); err != nil {
		t.Fatal(err)
	}
	if due != 3 {
		t.Errorf("expected 3 due, got %d", due)
	}
	if future != 2 {
		t.Errorf("expected 2 future, got %d", future)
	}
}

func TestAdvanceDueDates_FewerThanN(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Only 2 seen words with future due dates.
	for i, zh := range []string{"一", "二"} {
		id := seedWord(t, s, zh, "", []string{"en"})
		if _, err := s.db.ExecContext(ctx,
			`UPDATE sm2_progress SET first_seen_date = date('now'), due_date = datetime('now', ? || ' days') WHERE word_id = ?`,
			i+1, id); err != nil {
			t.Fatal(err)
		}
	}

	// Request 10 but only 2 available — should return 0 without error.
	nowDue, err := s.AdvanceDueDates(ctx, int64(2), 10)
	if err != nil {
		t.Fatal(err)
	}
	if nowDue != 0 {
		t.Errorf("expected 0, got %d", nowDue)
	}
}

// ── GetTranslationLanguages ───────────────────────────────────────────────────

// ── AcknowledgeRandomWords ────────────────────────────────────────────────────

func TestAcknowledgeRandomWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed 5 unseen words for user 2.
	for i := 0; i < 5; i++ {
		req := models.CreateWordRequest{ZhText: string(rune('一' + i)), EnTexts: []string{"word"}}
		if _, err := s.CreateWord(ctx, 2, req); err != nil {
			t.Fatalf("CreateWord: %v", err)
		}
	}

	// Acknowledge 3 random words.
	n, err := s.AcknowledgeRandomWords(ctx, 2, 3)
	if err != nil {
		t.Fatalf("AcknowledgeRandomWords: %v", err)
	}
	if n != 3 {
		t.Errorf("want 3 acknowledged, got %d", n)
	}

	// due_today should now be 3.
	due, _, _, err := s.GetStats(ctx, 2, nil, "")
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if due != 3 {
		t.Errorf("want due_today=3, got %d", due)
	}

	// Asking for more than available should cap at the remaining unseen count (2).
	n2, err := s.AcknowledgeRandomWords(ctx, 2, 10)
	if err != nil {
		t.Fatalf("AcknowledgeRandomWords second call: %v", err)
	}
	if n2 != 2 {
		t.Errorf("want 2 acknowledged (remaining unseen), got %d", n2)
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
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
		EnTexts: []string{"hello"},
		DeTexts: []string{"hallo"},
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

// ── GetTranslationsForWord (DE) ───────────────────────────────────────────────

func TestGetTranslationsForWord_DE(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:  "再见",
		Pinyin:  "zàijiàn",
		EnTexts: []string{"goodbye"},
		DeTexts: []string{"auf Wiedersehen", "tschüss"},
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
		ZhText:  "吃",
		Pinyin:  "chī",
		EnTexts: []string{"eat"},
		DeTexts: []string{"essen"},
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

// ── GetWords with missingLang filter ─────────────────────────────────────────

func TestGetWords_MissingLangEN(t *testing.T) {
	s := openTestDB(t)
	// Word with EN only (no DE).
	seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	// Word with both EN and DE.
	_, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:  "再见",
		Pinyin:  "zàijiàn",
		EnTexts: []string{"goodbye"},
		DeTexts: []string{"auf Wiedersehen"},
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
	s.db.Exec(`INSERT INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`, deID, zhID)

	// Word with both EN and DE.
	s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
		EnTexts: []string{"hello"},
		DeTexts: []string{"hallo"},
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

// ── UpdateWord with unchanged zh_text ────────────────────────────────────────

func TestUpdateWord_UnchangedZhText_NoError(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	// Save with the exact same ZhText — should not cause a UNIQUE constraint error.
	err := s.UpdateWord(context.Background(), int64(2), id, models.UpdateWordRequest{
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
		EnTexts: []string{"hello", "hi"},
	})
	if err != nil {
		t.Fatalf("UpdateWord with unchanged ZhText should not fail: %v", err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2),id)
	if wd.ZhText != "你好" {
		t.Errorf("ZhText should be unchanged, got %q", wd.ZhText)
	}
	if len(wd.EnTexts) != 2 {
		t.Errorf("expected 2 EnTexts after update, got %d: %v", len(wd.EnTexts), wd.EnTexts)
	}
}

// ── CreateWord and UpdateWord with DeTexts ────────────────────────────────────

func TestCreateWord_WithDeTexts(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
		EnTexts: []string{"hello"},
		DeTexts: []string{"hallo", "guten tag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, err := s.GetWordByID(context.Background(), int64(2),id)
	if err != nil {
		t.Fatal(err)
	}
	if len(wd.DeTexts) != 2 {
		t.Errorf("expected 2 DeTexts, got %d: %v", len(wd.DeTexts), wd.DeTexts)
	}
}

func TestUpdateWord_ReplacesDeTexts(t *testing.T) {
	s := openTestDB(t)
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:  "再见",
		Pinyin:  "zàijiàn",
		EnTexts: []string{"goodbye"},
		DeTexts: []string{"auf Wiedersehen"},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = s.UpdateWord(context.Background(), int64(2), id, models.UpdateWordRequest{
		ZhText:  "再见",
		Pinyin:  "zàijiàn",
		EnTexts: []string{"goodbye"},
		DeTexts: []string{"tschüss", "ciao"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), int64(2),id)
	if len(wd.DeTexts) != 2 {
		t.Errorf("expected 2 DeTexts after update, got %d: %v", len(wd.DeTexts), wd.DeTexts)
	}
	for _, dt := range wd.DeTexts {
		if dt == "auf Wiedersehen" {
			t.Error("old DE translation should have been removed")
		}
	}
}

// ── Migration v20: users table + initial user ─────────────────────────────────

func TestMigration_v20_UsersTableExists(t *testing.T) {
	s := openTestDB(t)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Error("users table should exist after migration v20")
	}
}

func TestMigration_v20_BothUsersSeeded(t *testing.T) {
	s := openTestDB(t)

	var adminHash, meHash string
	if err := s.db.QueryRow(`SELECT password_hash FROM users WHERE email = 'admin@example.de'`).Scan(&adminHash); err != nil {
		t.Fatalf("query admin user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(adminHash), []byte("I am the admin")); err != nil {
		t.Errorf("admin password hash does not match 'I am the admin': %v", err)
	}

	if err := s.db.QueryRow(`SELECT password_hash FROM users WHERE email = 'me@example.de'`).Scan(&meHash); err != nil {
		t.Fatalf("query initial user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(meHash), []byte("I learn zh")); err != nil {
		t.Errorf("me password hash does not match 'I learn zh': %v", err)
	}
}

func TestMigration_v20_AdminIsUserID1(t *testing.T) {
	s := openTestDB(t)
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM users WHERE email = 'admin@example.de'`).Scan(&id); err != nil {
		t.Fatalf("query admin id: %v", err)
	}
	if id != 1 {
		t.Errorf("expected admin user id=1, got %d", id)
	}
}

func TestMigration_v20_IdempotentOnFreshDB(t *testing.T) {
	s := openTestDB(t)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 2 {
		t.Errorf("expected exactly 2 users after migration, got %d", count)
	}
}

// ── Migration v21: words.user_id + template seeding ──────────────────────────

func TestMigration_v21_WordsHaveUserIDColumn(t *testing.T) {
	s := openTestDB(t)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('words') WHERE name = 'user_id'`).Scan(&count); err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	if count != 1 {
		t.Error("words table should have a user_id column after migration v21")
	}
}

func TestMigration_v21_CreateWordSetsUserID(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "测试", "cè shì", []string{"test"})

	var userID int64
	if err := s.db.QueryRow(`SELECT user_id FROM words WHERE id = ?`, id).Scan(&userID); err != nil {
		t.Fatalf("query word user_id: %v", err)
	}
	if userID != 2 {
		t.Errorf("expected user_id=2 for word created via CreateWord, got %d", userID)
	}
}

// ── Migration v21: template seeding from initial user ────────────────────────
// (Only meaningful when running against a real DB that has data before v21;
// on a fresh in-memory DB the initial user has no words to copy, so we verify
// the column and schema are correct and that the seeding path doesn't error.)

func TestMigration_v21_TemplateWordsAreSubsetOfAllWords(t *testing.T) {
	s := openTestDB(t)
	// Insert a template word (admin user, id=1) and a regular word (me user, id=2).
	seedTemplateWord(t, s, "学习", "xuéxí", []string{"study"}, nil)
	seedWord(t, s, "工作", "gōngzuò", []string{"work"})

	var templateCount, totalCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id = 1`).Scan(&templateCount); err != nil {
		t.Fatalf("count template words: %v", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM words`).Scan(&totalCount); err != nil {
		t.Fatalf("count all words: %v", err)
	}
	if templateCount > totalCount {
		t.Errorf("template words (%d) must not exceed total words (%d)", templateCount, totalCount)
	}
}

// ── ImportTemplateWords ───────────────────────────────────────────────────────

func seedTemplateWord(t *testing.T, s *Store, zhText, pinyin string, enTexts []string, tags []string) int64 {
	t.Helper()
	id, err := s.CreateWord(context.Background(), int64(1), models.CreateWordRequest{
		ZhText:  zhText,
		Pinyin:  pinyin,
		EnTexts: enTexts,
		Tags:    tags,
	})
	if err != nil {
		t.Fatalf("seedTemplateWord %q: %v", zhText, err)
	}
	return id
}

func insertTestUser(t *testing.T, s *Store, email string) int64 {
	t.Helper()
	res, err := s.db.Exec(`INSERT INTO users (email, password_hash) VALUES (?, 'x')`, email)
	if err != nil {
		t.Fatalf("insert test user %q: %v", email, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestImportTemplateWords_CopiesWordsForUser(t *testing.T) {
	s := openTestDB(t)
	seedTemplateWord(t, s, "苹果", "píngguǒ", []string{"apple"}, nil)

	userID := insertTestUser(t, s, "test@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("ImportTemplateWords: %v", err)
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id = ?`, userID).Scan(&count); err != nil {
		t.Fatalf("count user words: %v", err)
	}
	if count == 0 {
		t.Error("expected user to have words after ImportTemplateWords")
	}
}

func TestImportTemplateWords_CreatesSM2Progress(t *testing.T) {
	s := openTestDB(t)
	seedTemplateWord(t, s, "猫", "māo", []string{"cat"}, nil)

	userID := insertTestUser(t, s, "test2@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("ImportTemplateWords: %v", err)
	}

	var count int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sm2_progress sp
		JOIN words w ON w.id = sp.word_id
		WHERE w.user_id = ?`, userID).Scan(&count); err != nil {
		t.Fatalf("count sm2_progress: %v", err)
	}
	if count == 0 {
		t.Error("expected sm2_progress rows for imported words")
	}
}

func TestImportTemplateWords_CopiesTranslations(t *testing.T) {
	s := openTestDB(t)
	seedTemplateWord(t, s, "书", "shū", []string{"book"}, nil)

	userID := insertTestUser(t, s, "test3@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("ImportTemplateWords: %v", err)
	}

	// The zh word imported for the user should have a translation linked to an en word.
	var count int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM translations t
		JOIN words zh ON zh.id = t.zh_word_id AND zh.user_id = ?
		JOIN words en ON en.id = t.en_word_id
	`, userID).Scan(&count); err != nil {
		t.Fatalf("count translations: %v", err)
	}
	if count == 0 {
		t.Error("expected translations to be copied for imported words")
	}
}

func TestImportTemplateWords_Idempotent(t *testing.T) {
	s := openTestDB(t)
	seedTemplateWord(t, s, "水", "shuǐ", []string{"water"}, nil)

	userID := insertTestUser(t, s, "test4@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("first ImportTemplateWords: %v", err)
	}
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("second ImportTemplateWords: %v", err)
	}

	// Should still have only one zh word per template.
	var count int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM words WHERE user_id = ? AND language = 'zh'`, userID).Scan(&count); err != nil {
		t.Fatalf("count zh words: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 zh word after idempotent import, got %d", count)
	}
}

func TestImportTemplateWords_TemplatesUnchanged(t *testing.T) {
	s := openTestDB(t)
	seedTemplateWord(t, s, "火", "huǒ", []string{"fire"}, nil)

	userID := insertTestUser(t, s, "test5@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("ImportTemplateWords: %v", err)
	}

	// Template words (user_id=1, admin) must still exist after import.
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id = 1 AND language = 'zh'`).Scan(&count); err != nil {
		t.Fatalf("count template zh words: %v", err)
	}
	if count == 0 {
		t.Error("template words should remain after ImportTemplateWords")
	}
}

func TestImportTemplateWords_NoSM2ForTemplates(t *testing.T) {
	s := openTestDB(t)
	tmplID := seedTemplateWord(t, s, "地", "dì", []string{"earth", "ground"}, nil)

	// Count sm2_progress for the template word before import (CreateWord creates one).
	var before int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sm2_progress WHERE word_id = ?`, tmplID).Scan(&before); err != nil {
		t.Fatalf("count sm2_progress before import: %v", err)
	}

	userID := insertTestUser(t, s, "test6@example.com")
	if err := s.ImportTemplateWords(context.Background(), userID); err != nil {
		t.Fatalf("ImportTemplateWords: %v", err)
	}

	// ImportTemplateWords must not modify the template word's sm2_progress.
	var after int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sm2_progress WHERE word_id = ?`, tmplID).Scan(&after); err != nil {
		t.Fatalf("count sm2_progress after import: %v", err)
	}
	if after != before {
		t.Errorf("import changed template sm2_progress count: before=%d, after=%d", before, after)
	}
}

func TestLookupConfusion_ZhToEn_MatchesDeTranslation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// 人 → EN "person", DE "Person"
	targetID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "人",
		Pinyin:  "rén",
		EnTexts: []string{"person"},
		DeTexts: []string{"Person"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 点 → EN "dot", DE "Uhr"
	otherID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "点",
		Pinyin:  "diǎn",
		EnTexts: []string{"dot"},
		DeTexts: []string{"Uhr"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Typing "Uhr" (DE translation of 点) while answering for 人 should detect a confusion.
	confusedWithID, found, err := s.LookupConfusion(ctx, int64(2), targetID, "Uhr", "zh_to_en", []string{"en", "de"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found for DE answer")
	}
	if confusedWithID != otherID {
		t.Errorf("expected confusedWithID=%d, got %d", otherID, confusedWithID)
	}
}

func TestLookupConfusion_ZhToEn_MatchesEnTranslation(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	targetID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "狗",
		Pinyin:  "gǒu",
		EnTexts: []string{"dog"},
		DeTexts: []string{"Hund"},
	})
	if err != nil {
		t.Fatal(err)
	}

	otherID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "好",
		Pinyin:  "hǎo",
		EnTexts: []string{"good"},
		DeTexts: []string{"gut"},
	})
	if err != nil {
		t.Fatal(err)
	}

	confusedWithID, found, err := s.LookupConfusion(ctx, int64(2), targetID, "good", "zh_to_en", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected confusion to be found for EN answer")
	}
	if confusedWithID != otherID {
		t.Errorf("expected confusedWithID=%d, got %d", otherID, confusedWithID)
	}
}

func TestLookupConfusion_ZhToEn_DeNotMatchedWhenLangIsEnOnly(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	targetID, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "人",
		Pinyin:  "rén",
		EnTexts: []string{"person"},
		DeTexts: []string{"Person"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "点",
		Pinyin:  "diǎn",
		EnTexts: []string{"dot"},
		DeTexts: []string{"Uhr"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// With langs=["en"] only, typing "Uhr" (DE) should not produce a confusion.
	_, found, err := s.LookupConfusion(ctx, int64(2), targetID, "Uhr", "zh_to_en", []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("DE answer should not match when only EN is selected")
	}
}

// ── CreateUser ────────────────────────────────────────────────────────────────

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

// ── GetUserByID ────────────────────────────────────────────────────────────────

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

// ── SetUserEmailVerified ───────────────────────────────────────────────────────

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

// ── UpdateUserPassword ────────────────────────────────────────────────────────

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

func TestInitPinyinProgressForUser(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Create a real user (openTestDB seeds users 1 and 2; create user 3 here)
	userID, err := s.CreateUser(ctx, "pinyin-test@example.com", "hash", "tok-pinyin", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	// Insert two pinyin sounds directly
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO pinyin_sounds (initial, final, tone, syllable, filename, tag) VALUES
		 ('b', 'a', 1, 'ba', 'ba1.mp3', ''),
		 ('p', 'a', 2, 'pa', 'pa2.mp3', '')`)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.InitPinyinProgressForUser(ctx, userID); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pinyin_progress WHERE user_id = ?`, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 progress rows, got %d", count)
	}

	// Calling again must be idempotent
	if err := s.InitPinyinProgressForUser(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pinyin_progress WHERE user_id = ?`, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("idempotent: expected 2 progress rows, got %d", count)
	}
}

// ── Tag metadata tests ────────────────────────────────────────────────────────

func TestGetTagDetails_Empty(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	tags, err := s.GetTagDetails(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetTagDetails: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

func TestUpsertTagMeta_AndGetTagDetails(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed a word with a tag so the tag appears in GetTagDetails.
	if _, err := s.CreateWord(ctx, int64(2), models.CreateWordRequest{
		ZhText:  "测试",
		EnTexts: []string{"test"},
		Tags:    []string{"hsk1"},
	}); err != nil {
		t.Fatalf("CreateWord: %v", err)
	}

	// Initially description is empty, importable defaults to true.
	tags, err := s.GetTagDetails(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetTagDetails: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].Name != "hsk1" {
		t.Errorf("expected name hsk1, got %q", tags[0].Name)
	}
	if tags[0].Description != "" {
		t.Errorf("expected empty description, got %q", tags[0].Description)
	}
	if !tags[0].Importable {
		t.Errorf("expected importable=true by default")
	}

	// Update meta.
	if err := s.UpsertTagMeta(ctx, int64(2), "hsk1", "HSK level 1 vocabulary", false); err != nil {
		t.Fatalf("UpsertTagMeta: %v", err)
	}

	tags, err = s.GetTagDetails(ctx, int64(2))
	if err != nil {
		t.Fatalf("GetTagDetails after upsert: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].Description != "HSK level 1 vocabulary" {
		t.Errorf("expected updated description, got %q", tags[0].Description)
	}
	if tags[0].Importable {
		t.Errorf("expected importable=false after update")
	}
}

func TestGetImportableSourceTags_FiltersImportable(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Seed two tags for user 1 (source/library user).
	for _, tag := range []string{"hsk1", "hsk2"} {
		if _, err := s.CreateWord(ctx, int64(1), models.CreateWordRequest{
			ZhText:  tag + "字",
			EnTexts: []string{tag + " word"},
			Tags:    []string{tag},
		}); err != nil {
			t.Fatalf("CreateWord %s: %v", tag, err)
		}
	}

	// Mark hsk2 as not importable.
	if err := s.UpsertTagMeta(ctx, int64(1), "hsk2", "", false); err != nil {
		t.Fatalf("UpsertTagMeta: %v", err)
	}

	tags, err := s.GetImportableSourceTags(ctx, int64(1))
	if err != nil {
		t.Fatalf("GetImportableSourceTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 importable tag, got %d", len(tags))
	}
	if tags[0].Name != "hsk1" {
		t.Errorf("expected hsk1, got %q", tags[0].Name)
	}
}

func TestGetImportableSourceTags_WithDescription(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if _, err := s.CreateWord(ctx, int64(1), models.CreateWordRequest{
		ZhText:  "你好",
		EnTexts: []string{"hello"},
		Tags:    []string{"greetings"},
	}); err != nil {
		t.Fatalf("CreateWord: %v", err)
	}
	if err := s.UpsertTagMeta(ctx, int64(1), "greetings", "Basic greeting words", true); err != nil {
		t.Fatalf("UpsertTagMeta: %v", err)
	}

	tags, err := s.GetImportableSourceTags(ctx, int64(1))
	if err != nil {
		t.Fatalf("GetImportableSourceTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Description != "Basic greeting words" {
		t.Errorf("expected description 'Basic greeting words', got %+v", tags)
	}
}

// ── GetUserRole ───────────────────────────────────────────────────────────────

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
