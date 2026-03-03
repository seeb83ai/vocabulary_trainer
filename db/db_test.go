package db

import (
	"context"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

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
	id, err := s.CreateWord(context.Background(), models.CreateWordRequest{
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
	wd, err := s.GetWordByID(context.Background(), id)
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
	wd, err := s.GetWordByID(context.Background(), 9999)
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
	wd, err := s.GetWordByID(context.Background(), id)
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
	wd, err := s.GetWordByID(context.Background(), id)
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
	words, total, err := s.GetWords(context.Background(), "", 1, 20, "", "", nil)
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
	words, total, err := s.GetWords(context.Background(), "你好", 1, 20, "", "", nil)
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
	words, total, err := s.GetWords(context.Background(), "thank", 1, 20, "", "", nil)
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
	words, total, err := s.GetWords(context.Background(), "", 1, 3, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("total: want 5, got %d", total)
	}
	if len(words) != 3 {
		t.Errorf("page 1 per_page 3: want 3 results, got %d", len(words))
	}

	words2, _, err := s.GetWords(context.Background(), "", 2, 3, "", "", nil)
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
	err := s.UpdateWord(context.Background(), id, models.UpdateWordRequest{
		ZhText:  "妳好",
		Pinyin:  "nǐ hǎo",
		EnTexts: []string{"hello (female)"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), id)
	if wd.ZhText != "妳好" {
		t.Errorf("ZhText: want 妳好, got %q", wd.ZhText)
	}
	if len(wd.EnTexts) != 1 || wd.EnTexts[0] != "hello (female)" {
		t.Errorf("EnTexts: want [hello (female)], got %v", wd.EnTexts)
	}
}

func TestUpdateWord_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.UpdateWord(context.Background(), 9999, models.UpdateWordRequest{
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
	if err := s.DeleteWord(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), id)
	if wd != nil {
		t.Error("word should be gone after delete")
	}
}

func TestDeleteWord_NotFound(t *testing.T) {
	s := openTestDB(t)
	err := s.DeleteWord(context.Background(), 9999)
	if err == nil {
		t.Error("expected error when deleting non-existent word")
	}
}

// ── AddTranslation ────────────────────────────────────────────────────────────

func TestAddTranslation_AddsNewEN(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	if err := s.AddTranslation(context.Background(), id, "hi"); err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), id)
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
	s.AddTranslation(context.Background(), id, "hi")
	s.AddTranslation(context.Background(), id, "hi") // second call is no-op
	wd, _ := s.GetWordByID(context.Background(), id)
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
	err := s.AddTranslation(context.Background(), 9999, "hello")
	if err == nil {
		t.Error("expected error for unknown zh word id")
	}
}

// ── GetNextCard ───────────────────────────────────────────────────────────────

func TestGetNextCard_NilWhenEmpty(t *testing.T) {
	s := openTestDB(t)
	w, p, err := s.GetNextCard(context.Background(), nil, 100)
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
	w, p, err := s.GetNextCard(context.Background(), nil, 100)
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

func TestGetNextCard_MostOverduFirst(t *testing.T) {
	s := openTestDB(t)
	id1 := seedWord(t, s, "一", "", []string{"one"})
	id2 := seedWord(t, s, "二", "", []string{"two"})

	// Set id2's due_date far in the past so it's more overdue
	ctx := context.Background()
	past := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ? WHERE word_id = ?`, past, id2)
	_ = id1

	w, _, err := s.GetNextCard(ctx, nil, 100)
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
	w, _, err := s.GetNextCard(ctx, nil, 1)
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
	w2, _, err := s.GetNextCard(ctx, nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	if w2 == nil {
		t.Fatal("expected a card when cap is not yet reached")
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
	due, total, _, err := s.GetStats(context.Background(), nil)
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
	_, total, _, err := s.GetStats(context.Background(), nil)
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

	// Move one word into the future so it's NOT due
	ctx := context.Background()
	future := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02 15:04:05")
	s.db.ExecContext(ctx, `UPDATE sm2_progress SET due_date = ? WHERE word_id = ?`, future, id1)

	due, _, _, err := s.GetStats(ctx, nil)
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

	_, _, newToday, err := s.GetStats(ctx, nil)
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
	id, err := s.CreateWord(context.Background(), models.CreateWordRequest{
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
	wd, err := s.GetWordByID(context.Background(), id)
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
	err := s.UpdateWord(context.Background(), id, models.UpdateWordRequest{
		ZhText:  "你好",
		Pinyin:  "nǐ hǎo",
		EnTexts: []string{"hello"},
		Tags:    []string{"new-tag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := s.GetWordByID(context.Background(), id)
	if len(wd.Tags) != 1 || wd.Tags[0] != "new-tag" {
		t.Errorf("expected [new-tag], got %v", wd.Tags)
	}
	tags, _ := s.GetAllTags(context.Background())
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

	words, total, err := s.GetWords(context.Background(), "", 1, 20, "", "", []string{"greetings"})
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

	words, total, err := s.GetWords(context.Background(), "", 1, 20, "", "", []string{"greetings", "food"})
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

func TestGetNextCard_FilterByTag(t *testing.T) {
	s := openTestDB(t)
	seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"greetings"})
	id2 := seedWordWithTags(t, s, "吃饭", "", []string{"eat"}, []string{"food"})

	w, _, err := s.GetNextCard(context.Background(), []string{"food"}, 100)
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

	w, _, err := s.GetNextCard(context.Background(), []string{"nonexistent"}, 100)
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

	_, total, _, err := s.GetStats(context.Background(), []string{"food"})
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
	tags, err := s.GetAllTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0] != "A-tag" || tags[1] != "B-tag" {
		t.Errorf("expected [A-tag, B-tag], got %v", tags)
	}
}

func TestDeleteWord_CleansOrphanTags(t *testing.T) {
	s := openTestDB(t)
	id := seedWordWithTags(t, s, "你好", "", []string{"hello"}, []string{"unique-tag"})
	if err := s.DeleteWord(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	tags, _ := s.GetAllTags(context.Background())
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

	confusedWithID, found, err := s.LookupConfusion(context.Background(), zhID, "Buch", "zh_to_en")
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

	_, found, err := s.LookupConfusion(context.Background(), zhID, "Tisch", "zh_to_en")
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

	confusedWithID, found, err := s.LookupConfusion(context.Background(), zhID, "书", "en_to_zh")
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

	_, found, err := s.LookupConfusion(context.Background(), zhID, "Schuh", "zh_to_en")
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

	items, err := s.GetConfusions(context.Background())
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

	items, err := s.GetConfusions(context.Background())
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

	confusedWithID, found, err := s.LookupConfusion(context.Background(), zhID, "Buch", "zh_pinyin_to_en")
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

	_, found, err := s.LookupConfusion(context.Background(), zhID, "Buch", "invalid_mode")
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

	_, found, err := s.LookupConfusion(context.Background(), zhID, "", "zh_to_en")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("empty answer should never match")
	}
}

func TestGetConfusions_PopulatesEnTexts(t *testing.T) {
	s := openTestDB(t)
	idA := seedWord(t, s, "鞋", "xié", []string{"Schuh"})
	idB := seedWord(t, s, "书", "shū", []string{"Buch"})

	if err := s.UpsertConfusion(context.Background(), idA, idB, "zh_to_en"); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background())
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

	d, err := s.GetConfusionDetail(context.Background(), idA, idB, "zh_to_en")
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

	d, err := s.GetConfusionDetail(context.Background(), idA, idB, "zh_to_en")
	if err != nil {
		t.Fatal(err)
	}
	if d != nil {
		t.Error("expected nil when no confusion row exists")
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

	items, err := s.GetConfusions(context.Background())
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

	if err := s.DeleteWord(context.Background(), idA); err != nil {
		t.Fatal(err)
	}

	items, err := s.GetConfusions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("confusion_pairs should be cascade-deleted, got %d rows", len(items))
	}
}
