package handlers_test

import (
	"context"
	"reflect"
	"testing"

	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

func todayBuckets(t *testing.T, s *db.Store, wordID int64) map[string]db.AnswerBucketCount {
	t.Helper()
	got, err := s.TodayAnswerBucketsForTest(context.Background(), wordID)
	if err != nil {
		t.Fatalf("TodayAnswerBucketsForTest: %v", err)
	}
	return got
}

func boolPtr(b bool) *bool { return &b }

func TestQuizAnswer_RecordsAnswerBuckets(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhPinyinToTransl, Answer: "wrong",
		PinyinSeen: boolPtr(true), AudioPlayed: boolPtr(false),
	})

	want := map[string]db.AnswerBucketCount{"pinyin": {Attempts: 1, Errors: 1}}
	if got := todayBuckets(t, s, id); !reflect.DeepEqual(got, want) {
		t.Errorf("buckets: got %v, want %v", got, want)
	}
}

func TestQuizAnswer_DrillAnswerNotRecordedInBuckets(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhToTransl, Answer: "wrong", Drill: true,
		PinyinSeen: boolPtr(false), AudioPlayed: boolPtr(false),
	})

	if got := todayBuckets(t, s, id); len(got) != 0 {
		t.Errorf("drill answers must not be recorded, got %v", got)
	}
}

func TestQuizAnswer_VoiceUnavailableIsNotVoiceOnly(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)
	st, err := s.GetUserSettings(ctx, 2)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	st.VoiceUnavailable = true
	if err := s.UpdateUserSettings(ctx, 2, *st); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeVoiceToTransl, Answer: "hello",
		PinyinSeen: boolPtr(false), AudioPlayed: boolPtr(false),
	})

	want := map[string]db.AnswerBucketCount{"zh_only": {Attempts: 1, Errors: 0}}
	if got := todayBuckets(t, s, id); !reflect.DeepEqual(got, want) {
		t.Errorf("buckets: got %v, want %v", got, want)
	}
}

func TestAcceptCorrect_RemovesBucketError(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhPinyinToTransl, Answer: "helo",
		PinyinSeen: boolPtr(true), AudioPlayed: boolPtr(true),
	})
	rec := do(t, r, "POST", "/api/quiz/accept-correct", models.AcceptCorrectRequest{
		WordID: id, Mode: models.ModeZhPinyinToTransl,
		PinyinSeen: boolPtr(true), AudioPlayed: boolPtr(true),
	})
	if rec.Code != 200 {
		t.Fatalf("accept-correct: want 200, got %d: %s", rec.Code, rec.Body)
	}

	want := map[string]db.AnswerBucketCount{
		"pinyin": {Attempts: 1, Errors: 0},
		"voice":  {Attempts: 1, Errors: 0},
	}
	if got := todayBuckets(t, s, id); !reflect.DeepEqual(got, want) {
		t.Errorf("buckets: got %v, want %v", got, want)
	}
}

func TestQuizAnswer_RecordsHelpBuckets(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)
	st, err := s.GetUserSettings(ctx, 2)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	st.BlurPinyin = true
	if err := s.UpdateUserSettings(ctx, 2, *st); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhPinyinToTransl, Answer: "wrong",
		PinyinSeen: boolPtr(true), AudioPlayed: boolPtr(true), PlayPressed: boolPtr(false),
	})

	wrong := db.AnswerBucketCount{Attempts: 1, Errors: 1}
	want := map[string]db.AnswerBucketCount{
		"pinyin": wrong, "voice": wrong,
		"pinyin_reveal_offered": wrong, "pinyin_revealed": wrong,
		"play_offered": wrong,
	}
	if got := todayBuckets(t, s, id); !reflect.DeepEqual(got, want) {
		t.Errorf("buckets: got %v, want %v", got, want)
	}
}

// A "That's a different word" answer (a translation of another word) is a
// wrong answer and counts as an error in the buckets.
func TestQuizAnswer_ConfusionCountsAsBucketError(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	other := seedWord(t, s, "再见", "zài jiàn", []string{"goodbye"})
	r := newRouter(s)
	// A confusion only matches words the user has already seen.
	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{WordID: other, Mode: models.ModeZhToTransl, Answer: "goodbye"})

	rec := do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeZhToTransl, Answer: "goodbye", Langs: []string{"en"},
		PinyinSeen: boolPtr(false), AudioPlayed: boolPtr(false), PlayPressed: boolPtr(false),
	})
	var resp models.AnswerResponse
	decodeJSON(t, rec, &resp)
	if resp.Correct || resp.ConfusedWith == nil {
		t.Fatalf("want a wrong answer confused with 再见, got correct=%v confused_with=%v", resp.Correct, resp.ConfusedWith)
	}

	wrong := db.AnswerBucketCount{Attempts: 1, Errors: 1}
	want := map[string]db.AnswerBucketCount{"zh_only": wrong, "play_offered": wrong}
	if got := todayBuckets(t, s, id); !reflect.DeepEqual(got, want) {
		t.Errorf("buckets: got %v, want %v", got, want)
	}
}

// Translation → ZH cards offer no help: only to_zh, whatever the flags say.
func TestQuizAnswer_TranslToZhRecordsOnlyToZh(t *testing.T) {
	s := openTestDB(t)
	id := seedWord(t, s, "你好", "nǐ hǎo", []string{"hello"})
	r := newRouter(s)

	do(t, r, "POST", "/api/quiz/answer", models.AnswerRequest{
		WordID: id, Mode: models.ModeTranslToZh, Answer: "再见",
		PinyinSeen: boolPtr(true), AudioPlayed: boolPtr(true), PlayPressed: boolPtr(true),
	})

	want := map[string]db.AnswerBucketCount{"to_zh": {Attempts: 1, Errors: 1}}
	if got := todayBuckets(t, s, id); !reflect.DeepEqual(got, want) {
		t.Errorf("buckets: got %v, want %v", got, want)
	}
}
