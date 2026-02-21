package models

import "time"

// Mode constants for quiz card types
const (
	ModeEnToZh       = "en_to_zh"
	ModeZhToEn       = "zh_to_en"
	ModeZhPinyinToEn = "zh_pinyin_to_en"
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
	WordID        int64
	Repetitions   int
	Easiness      float64
	IntervalDays  int
	DueDate       time.Time
	TotalCorrect  int
	TotalAttempts int
}

// API request/response structs

type QuizCard struct {
	WordID       int64     `json:"word_id"`
	Mode         string    `json:"mode"`
	Prompt       string    `json:"prompt"`
	Pinyin       *string   `json:"pinyin"`
	DueDate      time.Time `json:"due_date"`
	IntervalDays int       `json:"interval_days"`
}

type AnswerRequest struct {
	WordID int64  `json:"word_id"`
	Mode   string `json:"mode"`
	Answer string `json:"answer"`
}

type AnswerResponse struct {
	Correct        bool      `json:"correct"`
	CorrectAnswers []string  `json:"correct_answers"`
	NextDue        time.Time `json:"next_due"`
	IntervalDays   int       `json:"interval_days"`
	TotalCorrect   int       `json:"total_correct"`
	TotalAttempts  int       `json:"total_attempts"`
}

type CreateWordRequest struct {
	ZhText  string   `json:"zh_text"`
	Pinyin  string   `json:"pinyin"`
	EnTexts []string `json:"en_texts"`
}

type UpdateWordRequest struct {
	ZhText  string   `json:"zh_text"`
	Pinyin  string   `json:"pinyin"`
	EnTexts []string `json:"en_texts"`
}

type WordDetail struct {
	ID        int64     `json:"id"`
	ZhText    string    `json:"zh_text"`
	Pinyin    *string   `json:"pinyin"`
	EnTexts   []string  `json:"en_texts"`
	CreatedAt time.Time `json:"created_at"`
}

type WordListResponse struct {
	Words   []WordDetail `json:"words"`
	Total   int          `json:"total"`
	Page    int          `json:"page"`
	PerPage int          `json:"per_page"`
}
