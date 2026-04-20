package models

import "time"

// Mode constants for quiz card types
const (
	ModeTranslToZh       = "transl_to_zh"
	ModeZhToTransl       = "zh_to_transl"
	ModeZhPinyinToTransl = "zh_pinyin_to_transl"
	ModeProgressive      = "progressive"
	ModeNewWord          = "new_word"
)

// DB-layer structs

type User struct {
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	PasswordHash  string `json:"-"`
	EmailVerified bool   `json:"email_verified"`
	Role          string `json:"role"`
}

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
	WordID          int64               `json:"word_id"`
	Mode            string              `json:"mode"`
	Prompt          string              `json:"prompt"`
	Pinyin          *string             `json:"pinyin"`
	Translations    map[string][]string `json:"translations,omitempty"`
	DueDate         time.Time           `json:"due_date"`
	IntervalDays    int                 `json:"interval_days"`
	LearningNewWord bool                `json:"learning_new_word"`
	// HMM mnemonic card fields (card_type="hmm"); zero-value for word cards.
	CardType   string `json:"card_type,omitempty"`
	EntityType string `json:"entity_type,omitempty"`
	EntityKey  string `json:"entity_key,omitempty"`
	Category   string `json:"category,omitempty"`
	Hint       string `json:"hint,omitempty"`
}

type AnswerRequest struct {
	WordID int64    `json:"word_id"`
	Mode   string   `json:"mode"`
	Answer string   `json:"answer"`
	Langs  []string `json:"langs,omitempty"`
}

type AnswerResponse struct {
	Correct         bool                `json:"correct"`
	CorrectAnswers  []string            `json:"correct_answers"`
	ZhText          string              `json:"zh_text"`
	Pinyin          *string             `json:"pinyin"`
	Translations    map[string][]string `json:"translations"`
	NextDue         time.Time           `json:"next_due"`
	IntervalDays    int                 `json:"interval_days"`
	TotalCorrect    int                 `json:"total_correct"`
	TotalAttempts   int                 `json:"total_attempts"`
	StreakBonus      int                `json:"streak_bonus"`
	Repetitions     int                 `json:"repetitions"`
	GraduateReps    int                 `json:"graduate_reps,omitempty"`
	LearningNewWord bool                `json:"learning_new_word"`
	Graduated       bool                `json:"graduated,omitempty"`
	ConfusedWith    *ConfusionDetail    `json:"confused_with,omitempty"`
	SessionStreak   int                 `json:"session_streak,omitempty"`
	Tier            string              `json:"tier,omitempty"`
	PrevTier        string              `json:"prev_tier,omitempty"`
	SceneText       string              `json:"scene_text,omitempty"`
}

type CreateWordRequest struct {
	ZhText        string              `json:"zh_text"`
	Pinyin        string              `json:"pinyin"`
	Translations  map[string][]string `json:"translations"`
	Tags          []string            `json:"tags"`
	StartTraining bool                `json:"start_training"`
}

type UpdateWordRequest struct {
	ZhText       string              `json:"zh_text"`
	Pinyin       string              `json:"pinyin"`
	Translations map[string][]string `json:"translations"`
	Tags         []string            `json:"tags"`
	StartTraining bool               `json:"start_training"`
}

type WordDetail struct {
	ID              int64               `json:"id"`
	ZhText          string              `json:"zh_text"`
	Pinyin          *string             `json:"pinyin"`
	Translations    map[string][]string `json:"translations"`
	CreatedAt       time.Time           `json:"created_at"`
	Repetitions     int                 `json:"repetitions"`
	Easiness        float64             `json:"easiness"`
	IntervalDays    int                 `json:"interval_days"`
	TotalCorrect    int                 `json:"total_correct"`
	TotalAttempts   int                 `json:"total_attempts"`
	StreakBonus     int                 `json:"streak_bonus"`
	DueDate         time.Time           `json:"due_date"`
	Tags            []string            `json:"tags"`
	NeedsReview     bool                `json:"needs_review"`
	LearningNewWord bool                `json:"learning_new_word"`
	SceneText       string              `json:"scene_text,omitempty"`
}

type ConfusionDetail struct {
	ZhWordID               int64               `json:"zh_word_id"`
	ZhText                 string              `json:"zh_text"`
	ZhPinyin               *string             `json:"zh_pinyin"`
	ZhTranslations         map[string][]string `json:"zh_translations"`
	ConfusedWithID         int64               `json:"confused_with_id"`
	ConfusedWithText       string              `json:"confused_with_text"`
	ConfusedWithPinyin     *string             `json:"confused_with_pinyin"`
	ConfusedWithTranslations map[string][]string `json:"confused_with_translations"`
	Mode                   string              `json:"mode"`
	Count                  int                 `json:"count"`
	LastSeen               time.Time           `json:"last_seen"`
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
	WordID       int64               `json:"word_id"`
	ZhText       string              `json:"zh_text"`
	Pinyin       *string             `json:"pinyin"`
	Translations map[string][]string `json:"translations"`
	Correct      int                 `json:"total_correct"`
	Attempts     int                 `json:"total_attempts"`
	StreakBonus  int                 `json:"streak_bonus"`
	Accuracy     float64             `json:"accuracy"`
	Easiness     float64             `json:"easiness"`
}

type PinyinDailyStat struct {
	Date         string
	Attempts     int
	Mistakes     int
	SoundsSeen   int
	Tone1Correct int
	Tone1Wrong   int
	Tone2Correct int
	Tone2Wrong   int
	Tone3Correct int
	Tone3Wrong   int
	Tone4Correct int
	Tone4Wrong   int
	Tone5Correct int
	Tone5Wrong   int
}

type PinyinDailyStatEntry struct {
	Date         string `json:"date"`
	Attempts     int    `json:"attempts"`
	Mistakes     int    `json:"mistakes"`
	SoundsSeen   int    `json:"sounds_seen"`
	Tone1Correct int    `json:"tone1_correct"`
	Tone1Wrong   int    `json:"tone1_wrong"`
	Tone2Correct int    `json:"tone2_correct"`
	Tone2Wrong   int    `json:"tone2_wrong"`
	Tone3Correct int    `json:"tone3_correct"`
	Tone3Wrong   int    `json:"tone3_wrong"`
	Tone4Correct int    `json:"tone4_correct"`
	Tone4Wrong   int    `json:"tone4_wrong"`
	Tone5Correct int    `json:"tone5_correct"`
	Tone5Wrong   int    `json:"tone5_wrong"`
}

type PinyinDailyStatsResponse struct {
	Days []PinyinDailyStatEntry `json:"days"`
}

type TagDetail struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Importable     bool     `json:"importable"`
	AvailableLangs []string `json:"available_langs,omitempty"`
}

type UpsertTagMetaRequest struct {
	Description string `json:"description"`
	Importable  bool   `json:"importable"`
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
	Components    []HanziDecomposition `json:"components,omitempty"`
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
	Initial        string            `json:"initial"`
	Final          string            `json:"final"`
	Tone           int               `json:"tone"`
	Decomposition  string            `json:"decomposition,omitempty"`
	Radicals       []string          `json:"radicals"`
	RadicalDefs    map[string]string `json:"radical_defs"`
	RadicalDeDefs  map[string]string `json:"radical_de_defs,omitempty"`
	Actor          *HMMActor         `json:"actor"`
	Location       *HMMLocation      `json:"location"`
	ToneRoom       *HMMToneRoom      `json:"tone_room"`
	Props          []HMMProp         `json:"props"`
	Scene          *HMMScene         `json:"scene,omitempty"`
	MultiChar      bool              `json:"multi_char,omitempty"`
}

type HMMSaveSceneRequest struct {
	SceneText     string    `json:"scene_text"`
	ActorName     string    `json:"actor_name"`
	LocationName  string    `json:"location_name"`
	RoomName      string    `json:"room_name"`
	Props         []HMMProp `json:"props"`
	Decomposition string    `json:"decomposition,omitempty"`
}

// HMM quiz models

const (
	HMMEntityActor    = "actor"
	HMMEntityLocation = "location"
	HMMEntityToneRoom = "tone_room"
	HMMEntityProp     = "prop"
)

type HMMProgress struct {
	UserID        int64
	EntityType    string
	EntityKey     string
	Repetitions   int
	Easiness      float64
	IntervalDays  int
	DueDate       time.Time
	TotalCorrect  int
	TotalAttempts int
	Learning      bool
	StreakBonus   int
	FirstSeenDate string // raw date string, "" if NULL
}

type HMMQuizCard struct {
	EntityType   string    `json:"entity_type"`
	EntityKey    string    `json:"entity_key"`
	Prompt       string    `json:"prompt"`
	Category     string    `json:"category,omitempty"`
	Hint         string    `json:"hint,omitempty"`
	DueDate      time.Time `json:"due_date"`
	IntervalDays int       `json:"interval_days"`
	Learning     bool      `json:"learning"`
}

type HMMAnswerRequest struct {
	EntityType string `json:"entity_type"`
	EntityKey  string `json:"entity_key"`
	Answer     string `json:"answer"`
}

type HMMAnswerResponse struct {
	Correct       bool      `json:"correct"`
	CorrectAnswer string    `json:"correct_answer"`
	YourAnswer    string    `json:"your_answer,omitempty"`
	NextDue       time.Time `json:"next_due"`
	IntervalDays  int       `json:"interval_days"`
	TotalCorrect  int       `json:"total_correct"`
	TotalAttempts int       `json:"total_attempts"`
	StreakBonus   int       `json:"streak_bonus"`
	Repetitions   int       `json:"repetitions"`
	Learning      bool      `json:"learning"`
	Graduated     bool      `json:"graduated,omitempty"`
	Tier          string    `json:"tier,omitempty"`
	PrevTier      string    `json:"prev_tier,omitempty"`
}

type HMMQuizStats struct {
	DueToday int `json:"due_today"`
	Total    int `json:"total"`
}

// Pinyin listening training models

const (
	PinyinModeMultipleChoice = "multiple_choice"
	PinyinModeTypeAnswer     = "type_answer"
)

type PinyinSound struct {
	ID       int64
	Initial  string // "b", "zh", "" (pure vowels)
	Final    string // "a", "an", "iao"
	Tone     int    // 1-4
	Syllable string // "ba" (without tone number)
	Filename string // "ba1.mp3"
	Tag      string // group tag: "b_p_m_f", "zh_ch_sh_r", "vowels"
}

type PinyinCard struct {
	SoundID      int64          `json:"sound_id"`
	Mode         string         `json:"mode"`
	AudioFile    string         `json:"audio_file"`
	Options      []PinyinOption `json:"options,omitempty"`
	DueDate      time.Time      `json:"due_date"`
	IntervalDays int            `json:"interval_days"`
	Learning     bool           `json:"learning"`
}

type PinyinOption struct {
	SoundID  int64  `json:"sound_id"`
	Label    string `json:"label"`
	Syllable string `json:"syllable"`
	Tone     int    `json:"tone"`
}

type PinyinAnswerRequest struct {
	SoundID int64  `json:"sound_id"`
	Answer  string `json:"answer"`
	Mode    string `json:"mode"`
}

type PinyinAnswerResponse struct {
	Correct       bool                   `json:"correct"`
	CorrectAnswer string                 `json:"correct_answer"`
	YourAnswer    string                 `json:"your_answer,omitempty"`
	NextDue       time.Time              `json:"next_due"`
	IntervalDays  int                    `json:"interval_days"`
	TotalCorrect  int                    `json:"total_correct"`
	TotalAttempts int                    `json:"total_attempts"`
	StreakBonus   int                    `json:"streak_bonus"`
	Repetitions   int                    `json:"repetitions"`
	GraduateReps  int                    `json:"graduate_reps,omitempty"`
	Learning      bool                   `json:"learning"`
	Graduated     bool                   `json:"graduated,omitempty"`
	ConfusedWith  *PinyinConfusionDetail `json:"confused_with,omitempty"`
	ToneVariants  []PinyinToneVariant    `json:"tone_variants,omitempty"`
	Tier          string                 `json:"tier,omitempty"`
	PrevTier      string                 `json:"prev_tier,omitempty"`
}

type PinyinToneVariant struct {
	Label    string `json:"label"`
	Filename string `json:"filename"`
	Tone     int    `json:"tone"`
	Current  bool   `json:"current"`
}

type PinyinConfusionDetail struct {
	SoundID           int64  `json:"sound_id"`
	SoundLabel        string `json:"sound_label"`
	ConfusedWithID    int64  `json:"confused_with_id"`
	ConfusedWithLabel string `json:"confused_with_label"`
	Count             int    `json:"count"`
}
