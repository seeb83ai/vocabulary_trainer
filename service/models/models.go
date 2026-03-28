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
	StreakBonus     int
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
	StreakBonus     int              `json:"streak_bonus"`
	Repetitions     int              `json:"repetitions"`
	GraduateReps    int              `json:"graduate_reps,omitempty"`
	LearningNewWord bool             `json:"learning_new_word"`
	Graduated       bool             `json:"graduated,omitempty"`
	ConfusedWith    *ConfusionDetail `json:"confused_with,omitempty"`
	SessionStreak   int              `json:"session_streak,omitempty"`
	Tier            string           `json:"tier,omitempty"`
	PrevTier        string           `json:"prev_tier,omitempty"`
	SceneText       string           `json:"scene_text,omitempty"`
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
	StreakBonus     int       `json:"streak_bonus"`
	DueDate         time.Time `json:"due_date"`
	Tags            []string  `json:"tags"`
	NeedsReview     bool      `json:"needs_review"`
	LearningNewWord bool      `json:"learning_new_word"`
	SceneText       string    `json:"scene_text,omitempty"`
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
	Date             string
	Attempts         int
	Mistakes         int
	WordsSeen        int
	CorrectStreak    int
	BucketNew        int
	BucketStruggling int
	BucketLearning   int
	BucketPracticing int
	BucketMastered   int
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
	Correct     int     `json:"total_correct"`
	Attempts    int     `json:"total_attempts"`
	StreakBonus int     `json:"streak_bonus"`
	Accuracy    float64 `json:"accuracy"`
	Easiness float64 `json:"easiness"`
}

type DueDateCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type DueDateDistributionResponse struct {
	Dates []DueDateCount `json:"dates"`
}

type DailyStatEntry struct {
	Date             string `json:"date"`
	Attempts         int    `json:"attempts"`
	Mistakes         int    `json:"mistakes"`
	WordsSeen        int    `json:"words_seen"`
	CorrectStreak    int    `json:"correct_streak"`
	BucketNew        int    `json:"bucket_new"`
	BucketStruggling int    `json:"bucket_struggling"`
	BucketLearning   int    `json:"bucket_learning"`
	BucketPracticing int    `json:"bucket_practicing"`
	BucketMastered   int    `json:"bucket_mastered"`
}

type HanziDecomposition struct {
	Character     string               `json:"character"`
	Definition    string               `json:"definition,omitempty"`
	Radical       string               `json:"radical,omitempty"`
	Decomposition string               `json:"decomposition,omitempty"`
	Etymology     *HanziEtymology      `json:"etymology,omitempty"`
	Components    []HanziDecomposition  `json:"components,omitempty"`
}

type HanziEtymology struct {
	Type     string `json:"type,omitempty"`
	Hint     string `json:"hint,omitempty"`
	Phonetic string `json:"phonetic,omitempty"`
	Semantic string `json:"semantic,omitempty"`
}

// Hanzi Movie Method (HMM) structs

type HMMActor struct {
	Initial   string `json:"initial"`
	Category  string `json:"category"`
	ActorName string `json:"actor_name"`
	Hint      string `json:"hint"`
}

type HMMLocation struct {
	FinalKey     string `json:"final_key"`
	LocationName string `json:"location_name"`
}

type HMMToneRoom struct {
	Tone     int    `json:"tone"`
	RoomName string `json:"room_name"`
}

type HMMProp struct {
	Radical  string `json:"radical"`
	PropName string `json:"prop_name"`
}

type HMMScene struct {
	WordID    int64  `json:"word_id"`
	SceneText string `json:"scene_text"`
}

type HMMSceneContext struct {
	Initial  string       `json:"initial"`
	Final    string       `json:"final"`
	Tone     int          `json:"tone"`
	Radicals []string     `json:"radicals"`
	Actor    *HMMActor    `json:"actor"`
	Location *HMMLocation `json:"location"`
	ToneRoom *HMMToneRoom `json:"tone_room"`
	Props    []HMMProp    `json:"props"`
	Scene    *HMMScene    `json:"scene,omitempty"`
}

type HMMSaveSceneRequest struct {
	SceneText    string    `json:"scene_text"`
	ActorName    string    `json:"actor_name"`
	LocationName string    `json:"location_name"`
	RoomName     string    `json:"room_name"`
	Props        []HMMProp `json:"props"`
}
