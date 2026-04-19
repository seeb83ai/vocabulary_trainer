package db

import (
	"context"
	"testing"
	"vocabulary_trainer/models"
)

const pinyinTestUserID = int64(2)

func seedPinyinSounds(t *testing.T, store *Store) []models.PinyinSound {
	t.Helper()
	sounds := []models.PinyinSound{
		{Initial: "b", Final: "a", Tone: 1, Syllable: "ba", Filename: "ba1.mp3", Tag: "b_p_m_f"},
		{Initial: "b", Final: "a", Tone: 2, Syllable: "ba", Filename: "ba2.mp3", Tag: "b_p_m_f"},
		{Initial: "b", Final: "a", Tone: 3, Syllable: "ba", Filename: "ba3.mp3", Tag: "b_p_m_f"},
		{Initial: "b", Final: "a", Tone: 4, Syllable: "ba", Filename: "ba4.mp3", Tag: "b_p_m_f"},
		{Initial: "p", Final: "a", Tone: 1, Syllable: "pa", Filename: "pa1.mp3", Tag: "b_p_m_f"},
		{Initial: "m", Final: "a", Tone: 1, Syllable: "ma", Filename: "ma1.mp3", Tag: "b_p_m_f"},
		{Initial: "zh", Final: "i", Tone: 1, Syllable: "zhi", Filename: "zhi1.mp3", Tag: "zh_ch_sh_r"},
		{Initial: "", Final: "a", Tone: 1, Syllable: "a", Filename: "a1.mp3", Tag: "vowels"},
	}
	for i, s := range sounds {
		id, err := store.InsertPinyinSound(context.Background(), pinyinTestUserID, s)
		if err != nil {
			t.Fatalf("InsertPinyinSound %s: %v", s.Filename, err)
		}
		sounds[i].ID = id
	}
	return sounds
}

func TestInsertPinyinSound(t *testing.T) {
	store := openTestDB(t)
	sounds := seedPinyinSounds(t, store)

	if len(sounds) != 8 {
		t.Fatalf("expected 8 sounds, got %d", len(sounds))
	}

	// Duplicate insert should return existing ID
	id, err := store.InsertPinyinSound(context.Background(), pinyinTestUserID, models.PinyinSound{
		Initial: "b", Final: "a", Tone: 1, Syllable: "ba", Filename: "ba1.mp3", Tag: "b_p_m_f",
	})
	if err != nil {
		t.Fatalf("duplicate insert: %v", err)
	}
	if id != sounds[0].ID {
		t.Errorf("duplicate insert id = %d, want %d", id, sounds[0].ID)
	}
}

func TestGetPinyinSoundByID(t *testing.T) {
	store := openTestDB(t)
	sounds := seedPinyinSounds(t, store)

	s, err := store.GetPinyinSoundByID(context.Background(), sounds[0].ID)
	if err != nil {
		t.Fatalf("GetPinyinSoundByID: %v", err)
	}
	if s == nil {
		t.Fatal("expected sound, got nil")
	}
	if s.Syllable != "ba" || s.Tone != 1 {
		t.Errorf("got syllable=%q tone=%d, want ba/1", s.Syllable, s.Tone)
	}

	// Non-existent ID
	s, err = store.GetPinyinSoundByID(context.Background(), 9999)
	if err != nil {
		t.Fatalf("GetPinyinSoundByID non-existent: %v", err)
	}
	if s != nil {
		t.Error("expected nil for non-existent ID")
	}
}

func TestGetPinyinSoundBySyllableTone(t *testing.T) {
	store := openTestDB(t)
	seedPinyinSounds(t, store)

	s, err := store.GetPinyinSoundBySyllableTone(context.Background(), "ba", 3)
	if err != nil {
		t.Fatalf("GetPinyinSoundBySyllableTone: %v", err)
	}
	if s == nil {
		t.Fatal("expected sound, got nil")
	}
	if s.Filename != "ba3.mp3" {
		t.Errorf("got filename=%q, want ba3.mp3", s.Filename)
	}
}

func TestGetNextPinyinCard(t *testing.T) {
	store := openTestDB(t)
	seedPinyinSounds(t, store)

	// Should return a card (all are due immediately)
	sound, prog, err := store.GetNextPinyinCard(context.Background(), pinyinTestUserID, nil, false)
	if err != nil {
		t.Fatalf("GetNextPinyinCard: %v", err)
	}
	if sound == nil {
		t.Fatal("expected sound, got nil")
	}
	if prog == nil {
		t.Fatal("expected progress, got nil")
	}
	if prog.LearningNewWord != true {
		t.Error("expected learning=true for new sound")
	}
}

func TestGetNextPinyinCardWithTags(t *testing.T) {
	store := openTestDB(t)
	seedPinyinSounds(t, store)

	// Filter by tag
	sound, _, err := store.GetNextPinyinCard(context.Background(), pinyinTestUserID, []string{"vowels"}, false)
	if err != nil {
		t.Fatalf("GetNextPinyinCard with tags: %v", err)
	}
	if sound == nil {
		t.Fatal("expected sound, got nil")
	}
	if sound.Tag != "vowels" {
		t.Errorf("expected tag=vowels, got %q", sound.Tag)
	}
}

func TestGetPinyinDistractors(t *testing.T) {
	store := openTestDB(t)
	sounds := seedPinyinSounds(t, store)

	// ba1 should get ba2, ba3, ba4 as same-syllable distractors
	distractors, err := store.GetPinyinDistractors(context.Background(), sounds[0], 3)
	if err != nil {
		t.Fatalf("GetPinyinDistractors: %v", err)
	}
	if len(distractors) != 3 {
		t.Fatalf("expected 3 distractors, got %d", len(distractors))
	}
	for _, d := range distractors {
		if d.ID == sounds[0].ID {
			t.Error("distractor should not be the target sound")
		}
	}
}

func TestGetPinyinProgress(t *testing.T) {
	store := openTestDB(t)
	sounds := seedPinyinSounds(t, store)

	prog, err := store.GetPinyinProgress(context.Background(), pinyinTestUserID, sounds[0].ID)
	if err != nil {
		t.Fatalf("GetPinyinProgress: %v", err)
	}
	if prog == nil {
		t.Fatal("expected progress, got nil")
	}
	if prog.Easiness != 2.5 {
		t.Errorf("expected default easiness=2.5, got %f", prog.Easiness)
	}
}

func TestUpdatePinyinProgress(t *testing.T) {
	store := openTestDB(t)
	sounds := seedPinyinSounds(t, store)

	prog, _ := store.GetPinyinProgress(context.Background(), pinyinTestUserID, sounds[0].ID)
	prog.TotalAttempts = 5
	prog.TotalCorrect = 3

	err := store.UpdatePinyinProgress(context.Background(), pinyinTestUserID, *prog)
	if err != nil {
		t.Fatalf("UpdatePinyinProgress: %v", err)
	}

	updated, _ := store.GetPinyinProgress(context.Background(), pinyinTestUserID, sounds[0].ID)
	if updated.TotalAttempts != 5 || updated.TotalCorrect != 3 {
		t.Errorf("progress not updated: attempts=%d correct=%d", updated.TotalAttempts, updated.TotalCorrect)
	}
}

func TestPinyinStats(t *testing.T) {
	store := openTestDB(t)
	seedPinyinSounds(t, store)

	due, total, err := store.GetPinyinStats(context.Background(), pinyinTestUserID, nil)
	if err != nil {
		t.Fatalf("GetPinyinStats: %v", err)
	}
	if total != 8 {
		t.Errorf("expected total=8, got %d", total)
	}
	// All sounds start with first_seen_date=NULL, so due=0
	if due != 0 {
		t.Errorf("expected due=0 (first_seen_date is NULL), got %d", due)
	}
}

func TestListPinyinTags(t *testing.T) {
	store := openTestDB(t)
	seedPinyinSounds(t, store)

	tags, err := store.ListPinyinTags(context.Background())
	if err != nil {
		t.Fatalf("ListPinyinTags: %v", err)
	}
	if len(tags) != 3 {
		t.Errorf("expected 3 tags, got %d: %v", len(tags), tags)
	}
}

func TestPinyinConfusions(t *testing.T) {
	store := openTestDB(t)
	sounds := seedPinyinSounds(t, store)

	// Upsert confusion
	err := store.UpsertPinyinConfusion(context.Background(), pinyinTestUserID, sounds[0].ID, sounds[1].ID)
	if err != nil {
		t.Fatalf("UpsertPinyinConfusion: %v", err)
	}

	// Upsert again to increment
	err = store.UpsertPinyinConfusion(context.Background(), pinyinTestUserID, sounds[0].ID, sounds[1].ID)
	if err != nil {
		t.Fatalf("UpsertPinyinConfusion second: %v", err)
	}

	detail, err := store.GetPinyinConfusionDetail(context.Background(), sounds[0].ID, sounds[1].ID)
	if err != nil {
		t.Fatalf("GetPinyinConfusionDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected confusion detail, got nil")
	}
	if detail.Count != 2 {
		t.Errorf("expected count=2, got %d", detail.Count)
	}
}

func TestAcknowledgePinyinSound(t *testing.T) {
	store := openTestDB(t)
	sounds := seedPinyinSounds(t, store)

	err := store.AcknowledgePinyinSound(context.Background(), pinyinTestUserID, sounds[0].ID)
	if err != nil {
		t.Fatalf("AcknowledgePinyinSound: %v", err)
	}

	// After acknowledging, the sound should be counted in due stats
	due, _, err := store.GetPinyinStats(context.Background(), pinyinTestUserID, nil)
	if err != nil {
		t.Fatalf("GetPinyinStats after acknowledge: %v", err)
	}
	if due != 1 {
		t.Errorf("expected due=1 after acknowledging, got %d", due)
	}
}
