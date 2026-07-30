package db

import (
	"context"
	"time"

	"vocabulary_trainer/models"
)

// Sub-store interfaces split the ~117-method Store surface into focused,
// domain-grouped facets so handlers can declare the narrowest dependency they
// actually use (interface segregation) instead of taking the whole *Store.
//
// *Store implements every interface here (see the compile-time assertions at the
// bottom), so db.Open() and handler construction are unchanged: a *Store value
// can be assigned to any of these. No SQL moves between files — this is a
// type-level reorganisation only.

// WordStore: word CRUD, translation links, tag queries, and GetNextCard.
type WordStore interface {
	GetWords(ctx context.Context, userID int64, q string, page, perPage int, sortBy, sortDir string, tags []string, reviewOnly bool, hideUnseen bool, bucket string, dueFilter string, missingLang string) ([]models.WordDetail, int, error)
	IsZhWordForUser(ctx context.Context, userID int64, text string) (bool, error)
	GetWordIDByZhText(ctx context.Context, userID int64, text string) (int64, error)
	GetPinyinByZhText(ctx context.Context, userID int64, text string) (*string, error)
	GetZhTextByID(ctx context.Context, userID, wordID int64) (string, error)
	GetWordByID(ctx context.Context, userID, id int64) (*models.WordDetail, error)
	CreateWord(ctx context.Context, userID int64, req models.CreateWordRequest) (int64, error)
	UpdateWord(ctx context.Context, userID int64, id int64, req models.UpdateWordRequest) error
	AddTranslation(ctx context.Context, userID int64, zhID int64, lang, text string) error
	DeleteWord(ctx context.Context, userID, id int64) error
	MarkWordForReview(ctx context.Context, userID, id int64) error
	GetTranslationLanguages(ctx context.Context) ([]string, error)
	GetTranslationsForWord(ctx context.Context, wordID int64, targetLang string) ([]models.Word, error)
	GetNextCard(ctx context.Context, userID int64, tags []string, maxNew int, bucket string, skipNew bool, baselines *NewWordBaselines, excludeIDs []int64, allowSessionExtension bool) (*models.Word, *models.SM2Progress, bool, error)
	GetAllTags(ctx context.Context, userID int64) ([]string, error)
	GetTagDetails(ctx context.Context, userID int64) ([]models.TagDetail, error)
	UpsertTagMeta(ctx context.Context, userID int64, name, description string, importable bool) error
	GetImportableSourceTags(ctx context.Context, userID int64) ([]models.TagDetail, error)
}

// QuizStore: SM-2 progress, confusion pairs, and daily stats.
type QuizStore interface {
	GetSM2Progress(ctx context.Context, wordID int64) (*models.SM2Progress, error)
	UpdateSM2Progress(ctx context.Context, p models.SM2Progress) error
	IsLearningNewWord(ctx context.Context, userID, wordID int64) (bool, error)
	SkipWord(ctx context.Context, userID, wordID int64, days int) error
	AcknowledgeWord(ctx context.Context, userID, wordID int64) error
	AcknowledgeRandomWords(ctx context.Context, userID int64, n int) (int, error)
	GetStats(ctx context.Context, userID int64, tags []string, bucket string) (dueToday, total, newToday int, err error)
	CountUnseenZhWords(ctx context.Context, userID int64, tags []string) (int, error)
	CountLearningNewWords(ctx context.Context, userID int64, tags []string) (int, error)
	GetWordCountByDueDate(ctx context.Context, userID int64, tags []string) ([]models.DueDateCount, error)
	DetectConfusion(ctx context.Context, userID, zhWordID int64, answer, mode string, langs []string) (int64, bool, error)
	UpsertConfusion(ctx context.Context, zhWordID, confusedWithID int64, mode string) error
	GetConfusionDetail(ctx context.Context, zhWordID, confusedWithID int64, mode string, langs []string) (*models.ConfusionDetail, error)
	GetConfusions(ctx context.Context, userID int64) ([]models.ConfusionDetail, error)
	GetRecentMismatches(ctx context.Context, userID int64, since time.Time, limit int) ([]models.ConfusionDetail, error)
	MarkConfusionsShownInGame(ctx context.Context, pairs [][2]int64) error
	SaveSM2PrevState(ctx context.Context, wordID int64, p models.SM2Progress) error
	GetSM2PrevState(ctx context.Context, wordID int64) (*models.SM2Progress, error)
	ClearSM2PrevState(ctx context.Context, wordID int64) error
	RecordDailyStat(ctx context.Context, userID int64, correct bool) (int, error)
	EnsureDueTodaySnapshot(ctx context.Context, userID int64) (int, error)
	RecordTrainingTime(ctx context.Context, userID int64, seconds int) error
	GetDailyStatsHistory(ctx context.Context, userID int64) ([]models.DailyStat, error)
	GetWordStats(ctx context.Context, userID int64) (*models.WordStatsResponse, error)
	GetTodaySessionInfo(ctx context.Context, userID int64) (attempts, mistakes, availableToAdvance int, err error)
	AdvanceDueDates(ctx context.Context, userID int64, n int) (int, error)
	SharesTranslation(ctx context.Context, wordID1, wordID2 int64, langs []string) (bool, error)
	FlagDifficultWords(ctx context.Context, userID int64, count int) (int, error)
	ClearDrillFlag(ctx context.Context, wordID int64) error
	ClearAllDrillFlags(ctx context.Context, userID int64) error
	CountDrillFlags(ctx context.Context, userID int64) (int, error)
	GetNextDrillCard(ctx context.Context, userID int64) (*models.Word, *models.SM2Progress, error)
}

// MnemonicStore: HMM actor/location/room/prop library, scene storage, and the HMM quiz.
type MnemonicStore interface {
	GetHMMActors(ctx context.Context, userID int64) ([]models.HMMActor, error)
	UpdateHMMActor(ctx context.Context, userID int64, initial, actorName string) error
	GetHMMLocations(ctx context.Context, userID int64) ([]models.HMMLocation, error)
	UpdateHMMLocation(ctx context.Context, userID int64, finalKey, locationName string) error
	GetHMMToneRooms(ctx context.Context, userID int64) ([]models.HMMToneRoom, error)
	UpdateHMMToneRoom(ctx context.Context, userID int64, tone int, roomName string) error
	GetHMMProps(ctx context.Context, userID int64) ([]models.HMMProp, error)
	UpsertHMMProp(ctx context.Context, userID int64, radical, propName string) error
	DeleteHMMProp(ctx context.Context, userID int64, radical string) error
	GetHMMScene(ctx context.Context, wordID int64) (*models.HMMScene, error)
	UpsertHMMScene(ctx context.Context, wordID int64, sceneText string) error
	DeleteHMMScene(ctx context.Context, userID, wordID int64) error
	GetHMMSceneText(ctx context.Context, wordID int64) (string, error)
	GetHMMActorByInitial(ctx context.Context, userID int64, initial string) (*models.HMMActor, error)
	GetHMMLocationByFinal(ctx context.Context, userID int64, finalKey string) (*models.HMMLocation, error)
	GetHMMToneRoom(ctx context.Context, userID int64, tone int) (*models.HMMToneRoom, error)
	GetHMMPropsByRadicals(ctx context.Context, userID int64, radicals []string) ([]models.HMMProp, error)
	SaveHMMSceneWithLibrary(ctx context.Context, userID, wordID int64, initial, finalKey string, tone int, req models.HMMSaveSceneRequest) error
	ImportTemplateWords(ctx context.Context, userID int64) error
	EnsureHMMProgress(ctx context.Context, userID int64) error
	GetNextDueHMMCard(ctx context.Context, userID int64, types []string) (*models.HMMQuizCard, *models.HMMProgress, error)
	GetHMMProgress(ctx context.Context, userID int64, entityType, entityKey string) (*models.HMMProgress, error)
	SkipHMM(ctx context.Context, userID int64, entityType, entityKey string, days int) error
	UpdateHMMProgress(ctx context.Context, p models.HMMProgress) error
	GetHMMStats(ctx context.Context, userID int64, types []string) (models.HMMQuizStats, error)
	GetHMMBreakdown(ctx context.Context, userID int64) ([]HMMEntityBreakdown, error)
}

// PinyinStore: pinyin sound library and listening-quiz progress.
type PinyinStore interface {
	InsertPinyinSound(ctx context.Context, userID int64, sound models.PinyinSound) (int64, error)
	InitPinyinProgressForUser(ctx context.Context, userID int64) error
	GetPinyinSoundByID(ctx context.Context, id int64) (*models.PinyinSound, error)
	GetPinyinSoundBySyllableTone(ctx context.Context, syllable string, tone int) (*models.PinyinSound, error)
	GetPinyinToneVariants(ctx context.Context, syllable string) ([]models.PinyinSound, error)
	GetNextPinyinCard(ctx context.Context, userID int64, tags []string, skipNew bool) (*models.PinyinSound, *models.SM2Progress, error)
	GetPinyinDistractors(ctx context.Context, target models.PinyinSound, count int) ([]models.PinyinSound, error)
	GetPinyinProgress(ctx context.Context, userID, soundID int64) (*models.SM2Progress, error)
	UpdatePinyinProgress(ctx context.Context, userID int64, p models.SM2Progress) error
	AcknowledgePinyinSound(ctx context.Context, userID, soundID int64) error
	GetPinyinStats(ctx context.Context, userID int64, tags []string) (due, total int, err error)
	ListPinyinTags(ctx context.Context) ([]string, error)
	UpsertPinyinConfusion(ctx context.Context, userID, soundID, confusedWithID int64) error
	GetPinyinConfusionDetail(ctx context.Context, soundID, confusedWithID int64) (*models.PinyinConfusionDetail, error)
	RecordPinyinDailyStat(ctx context.Context, userID int64, correct bool, tone int) error
	GetPinyinDailyStatsHistory(ctx context.Context, userID int64) ([]models.PinyinDailyStat, error)
}

// ComponentStore: hanzi component progress, initialisation, decomposition, and component scenes.
type ComponentStore interface {
	InitComponentsForWord(ctx context.Context, userID int64, zhText string, dueDate time.Time) error
	GetNextComponentCard(ctx context.Context, userID int64, langs []string) (*componentCard, error)
	GetComponentDefinitions(ctx context.Context, userID int64, character string, langs []string) (map[string]string, error)
	StoreComponentTranslation(ctx context.Context, userID int64, character, lang, definition string) error
	GetComponentTranslations(ctx context.Context, userID int64, character string) (map[string]string, error)
	MarkComponentForReview(userID int64, character string) error
	MarkComponentSeen(ctx context.Context, userID int64, character string) error
	SkipComponent(ctx context.Context, userID int64, character string, days int) error
	GetComponentProgress(ctx context.Context, userID int64, character string) (*models.ComponentProgress, error)
	RecordComponentAnswer(ctx context.Context, userID int64, character string, correct bool) (models.ComponentProgress, time.Time, error)
	RecordComponentStat(ctx context.Context, userID int64, correct bool) error
	GetComponentStatsHistory(ctx context.Context, userID int64) ([]models.ComponentDailyStat, error)
	GetComponentHMMSceneText(ctx context.Context, userID int64, character string) (string, error)
	UpsertComponentHMMScene(ctx context.Context, userID int64, character, sceneText string) error
	DeleteComponentHMMScene(ctx context.Context, userID int64, character string) error
	GetComponentHMMSceneRecord(ctx context.Context, userID int64, character string) (*models.HMMScene, error)
	SaveComponentHMMSceneWithLibrary(ctx context.Context, userID int64, character, initial, finalKey string, tone int, req models.HMMSaveSceneRequest) error
	SaveComponentPrevState(ctx context.Context, userID int64, character string, p models.ComponentProgress) error
	GetComponentPrevState(ctx context.Context, userID int64, character string) (*models.ComponentProgress, error)
	ClearComponentPrevState(ctx context.Context, userID int64, character string) error
	UpdateComponentProgress(ctx context.Context, userID int64, character string, p models.SM2Progress) error
	GetComponentList(ctx context.Context, userID int64, search string, page, perPage int, reviewOnly bool) ([]ComponentListItem, int, error)
	GetComponentPinyin(ctx context.Context, character string) string
	GetComponentCounts(ctx context.Context, userID int64, langs []string) (dueToday, total int, err error)
	GetComponentCountByDueDate(ctx context.Context, userID int64) ([]models.DueDateCount, error)
	GetHanziDecomposition(ctx context.Context, chars []rune) ([]models.HanziDecomposition, error)
	AnnotateComponentDefinitions(ctx context.Context, userID int64, results []models.HanziDecomposition, langs []string) error
	AnnotateNewComponents(ctx context.Context, userID int64, results []models.HanziDecomposition) error
	GetHanziDecompositionString(ctx context.Context, char string) (string, error)
	UpsertHanziDecomposition(ctx context.Context, char, decomp string) error
	GetTranslationsByZhTexts(ctx context.Context, zhTexts []string, lang string) (map[string]string, error)
	StoreTranslationForZhChar(ctx context.Context, zhText, pinyin, transText, lang string) error
}

// UserStore: user CRUD, auth, password, and settings.
type UserStore interface {
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetUserByID(ctx context.Context, id int64) (*models.User, error)
	GetUserRole(ctx context.Context, userID int64) (string, error)
	CreateUser(ctx context.Context, email, passwordHash, verificationToken string, expiresAt time.Time) (int64, error)
	SetUserEmailVerified(ctx context.Context, token string) (*models.User, error)
	UpdateUserPassword(ctx context.Context, userID int64, passwordHash string) error
	GetSessionsInvalidatedAt(ctx context.Context, userID int64) (time.Time, error)
	GetUserSettings(ctx context.Context, userID int64) (*models.UserSettings, error)
	GetUserSettingsRaw(ctx context.Context, userID int64) (settings *models.UserSettings, salt, deeplEnc, llmEnc string, err error)
	UpdateUserSettings(ctx context.Context, userID int64, st models.UserSettings) error
	UpdateTrainingFilters(ctx context.Context, userID int64, mode, bucket string, langs []string, mnemonics, components bool, tags []string) error
	UpdateUserAPIKeys(ctx context.Context, userID int64, deeplEnc, llmProvider, llmEnc, llmLocalURL string) error
	CreateUserWithSettings(ctx context.Context, email, passwordHash, verificationToken string, expiresAt time.Time) (int64, error)
	// Login lockout + audit (auth domain).
	RecordAuditLog(ctx context.Context, userID int64, action, ipAddress, detail string) error
	IncrementFailedLogins(ctx context.Context, userID int64) (int, error)
	LockAccountUntil(ctx context.Context, userID int64, until time.Time) error
	IsAccountLocked(ctx context.Context, userID int64) (bool, time.Time, error)
	ResetFailedLogins(ctx context.Context, userID int64) error
}

// Compile-time guarantees that the concrete *Store provides every sub-store
// surface, so db.Open() callers and handler construction need no changes.
var (
	_ WordStore      = (*Store)(nil)
	_ QuizStore      = (*Store)(nil)
	_ MnemonicStore  = (*Store)(nil)
	_ PinyinStore    = (*Store)(nil)
	_ ComponentStore = (*Store)(nil)
	_ UserStore      = (*Store)(nil)
)
