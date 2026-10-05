package handlers

import (
	"context"
	"time"

	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

// Per-handler store dependencies. Each interface embeds only the db sub-stores
// the handler actually uses, so the handler declares the narrowest surface it
// needs instead of taking the whole *db.Store. *db.Store satisfies all of these,
// so handler construction in main.go and tests is unchanged.
//
// A brand-new handler that only needs word data can declare `Store db.WordStore`
// directly without pulling in quiz/mnemonic/etc. methods.

type wordsStore interface {
	db.WordStore
	db.QuizStore
	db.ComponentStore
	db.UserStore
	LibraryDiff(ctx context.Context, userID, wordID int64) (models.LibraryDiff, error)
	ResetToLibrary(ctx context.Context, userID, wordID int64) error
}

type uploadCSVStore interface {
	db.WordStore
	db.QuizStore
	db.ComponentStore
	db.UserStore
}

type tagsStore interface {
	db.WordStore
}

type quizStore interface {
	db.WordStore
	db.QuizStore
	db.MnemonicStore
	db.ComponentStore
	db.UserStore
}

type mismatchesStore interface {
	db.QuizStore
}

type hanziStore interface {
	db.ComponentStore
}

type hmmStore interface {
	db.MnemonicStore
	db.WordStore
	db.ComponentStore
}

type hmmQuizStore interface {
	db.MnemonicStore
}

type pinyinQuizStore interface {
	db.PinyinStore
}

type componentStore interface {
	db.ComponentStore
	db.MnemonicStore
	db.UserStore
}

type importStore interface {
	db.WordStore
	AcknowledgeWord(ctx context.Context, userID, wordID int64) error
	db.ImportJobStore
	LookupDictionary(ctx context.Context, simplified, lang string) ([]string, error)
	LibraryWordsWithGlosses(ctx context.Context, userID int64, libraryIDs []int64) (map[int64]bool, error)
	CreateReferences(ctx context.Context, userID int64, libraryIDs []int64, tags []string) ([]int64, error)
	TombstonedLibraryWords(ctx context.Context, userID int64) (map[int64]bool, error)
	ImportedLists(ctx context.Context, userID int64) ([]models.ImportedList, error)
}

type llmStore interface {
	db.ComponentStore
	db.UserStore
	db.WordStore
}

type translateStore interface {
	db.UserStore
	LookupDictionary(ctx context.Context, simplified, lang string) ([]string, error)
}

type authStore interface {
	db.UserStore
	db.PinyinStore
}

type settingsStore interface {
	db.UserStore
	db.ComponentStore
	GetTrainingZhWords(ctx context.Context, userID int64) ([]models.Word, error)
}

type audioStore interface {
	db.WordStore
}

type adminStore interface {
	db.AdminStore
}

// componentIniter is the minimal surface the initComponents helper needs.
type componentIniter interface {
	GetSM2Progress(ctx context.Context, wordID int64) (*models.SM2Progress, error)
	InitComponentsForWord(ctx context.Context, userID int64, zhText string, dueDate time.Time) error
	CreateSubwordsForWord(ctx context.Context, userID, zhWordID int64, zhText string) error
	GetUserSettings(ctx context.Context, userID int64) (*models.UserSettings, error)
}
