package models

import "time"

// Mode constants for quiz card types
const (
	ModeEnToZh       = "en_to_zh"
	ModeZhToEn       = "zh_to_en"
	ModeZhPinyinToEn = "zh_pinyin_to_en"
	ModeProgressive  = "progressive"
	ModeNewWord      = "new_word"
)

// DB-layer structs

type Word struct {
	ID        int64
	Text      string
	Language  string // "en" or "zh"
	Pinyin    *string
	CreatedAt time.Time
}

type SM2Progress struct {
	WordID          int64
	Repetitions     int
	Easiness        float64
	IntervalDays    int
	DueDate         time.Time
	TotalCorrect    int
	TotalAttempts   int
	LearningNewWord bool
}

// API request/response structs

type QuizCard struct {
	WordID          int64     `json:"word_id"`
	Mode            string    `json:"mode"`
	Prompt          string    `json:"prompt"`
	Pinyin          *string   `json:"pinyin"`
	EnTexts         []string  `json:"en_texts,omitempty"`
	DueDate         time.Time `json:"due_date"`
	IntervalDays    int       `json:"interval_days"`
	LearningNewWord bool      `json:"learning_new_word"`
}

type AnswerRequest struct {
	WordID int64  `json:"word_id"`
	Mode   string `json:"mode"`
	Answer string `json:"answer"`
}

type AnswerResponse struct {
	Correct         bool             `json:"correct"`
	CorrectAnswers  []string         `json:"correct_answers"`
	ZhText          string           `json:"zh_text"`
	Pinyin          *string          `json:"pinyin"`
	EnTexts         []string         `json:"en_texts"`
	NextDue         time.Time        `json:"next_due"`
	IntervalDays    int              `json:"interval_days"`
	TotalCorrect    int              `json:"total_correct"`
	TotalAttempts   int              `json:"total_attempts"`
	Repetitions     int              `json:"repetitions"`
	GraduateReps    int              `json:"graduate_reps,omitempty"`
	LearningNewWord bool             `json:"learning_new_word"`
	Graduated       bool             `json:"graduated,omitempty"`
	ConfusedWith    *ConfusionDetail `json:"confused_with,omitempty"`
}

type CreateWordRequest struct {
	ZhText        string   `json:"zh_text"`
	Pinyin        string   `json:"pinyin"`
	EnTexts       []string `json:"en_texts"`
	Tags          []string `json:"tags"`
	StartTraining bool     `json:"start_training"`
}

type UpdateWordRequest struct {
	ZhText        string   `json:"zh_text"`
	Pinyin        string   `json:"pinyin"`
	EnTexts       []string `json:"en_texts"`
	Tags          []string `json:"tags"`
	StartTraining bool     `json:"start_training"`
}

type WordDetail struct {
	ID              int64     `json:"id"`
	ZhText          string    `json:"zh_text"`
	Pinyin          *string   `json:"pinyin"`
	EnTexts         []string  `json:"en_texts"`
	CreatedAt       time.Time `json:"created_at"`
	Repetitions     int       `json:"repetitions"`
	Easiness        float64   `json:"easiness"`
	IntervalDays    int       `json:"interval_days"`
	TotalCorrect    int       `json:"total_correct"`
	TotalAttempts   int       `json:"total_attempts"`
	DueDate         time.Time `json:"due_date"`
	Tags            []string  `json:"tags"`
	NeedsReview     bool      `json:"needs_review"`
	LearningNewWord bool      `json:"learning_new_word"`
}

type ConfusionDetail struct {
	ZhWordID            int64     `json:"zh_word_id"`
	ZhText              string    `json:"zh_text"`
	ZhPinyin            *string   `json:"zh_pinyin"`
	ZhEnTexts           []string  `json:"zh_en_texts"`
	ConfusedWithID      int64     `json:"confused_with_id"`
	ConfusedWithText    string    `json:"confused_with_text"`
	ConfusedWithPinyin  *string   `json:"confused_with_pinyin"`
	ConfusedWithEnTexts []string  `json:"confused_with_en_texts"`
	Mode                string    `json:"mode"`
	Count               int       `json:"count"`
	LastSeen            time.Time `json:"last_seen"`
}

type WordListResponse struct {
	Words   []WordDetail `json:"words"`
	Total   int          `json:"total"`
	Page    int          `json:"page"`
	PerPage int          `json:"per_page"`
}

type DailyStat struct {
	Date          string
	Attempts      int
	Mistakes      int
	WordsKnown    int
	NewWords      int
	WordsSeen     int
	CorrectStreak int
}

type DailyStatsResponse struct {
	Days []DailyStatEntry `json:"days"`
}

type WordStatsResponse struct {
	TotalSeen  int              `json:"total_seen"`
	AccBuckets map[string]int   `json:"accuracy_buckets"`
	Hardest    []WordStatDetail `json:"hardest"`
	MostPract  []WordStatDetail `json:"most_practiced"`
}

type WordStatDetail struct {
	WordID   int64   `json:"word_id"`
	ZhText   string  `json:"zh_text"`
	Pinyin   *string `json:"pinyin"`
	EnTexts  []string `json:"en_texts"`
	Correct  int     `json:"total_correct"`
	Attempts int     `json:"total_attempts"`
	Accuracy float64 `json:"accuracy"`
	Easiness float64 `json:"easiness"`
}

type DailyStatEntry struct {
	Date          string `json:"date"`
	Attempts      int    `json:"attempts"`
	Mistakes      int    `json:"mistakes"`
	WordsKnown    int    `json:"words_known"`
	NewWords      int    `json:"new_words"`
	WordsSeen     int    `json:"words_seen"`
	CorrectStreak int    `json:"correct_streak"`
}
