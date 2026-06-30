package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

type QuizHandler struct {
	Store        quizStore
	MaxNewPerDay int
	mu           sync.Mutex
	capResetDate string // date string (YYYY-MM-DD) on which the new-word cap was reset
	newCapBase   int    // newToday count at cap-reset time; cap = newCapBase + MaxNewPerDay
}

// Langs returns the distinct translation languages available in the database.
func (h *QuizHandler) Langs(w http.ResponseWriter, r *http.Request) {
	langs, err := h.Store.GetTranslationLanguages(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if langs == nil {
		langs = []string{}
	}
	writeJSON(w, http.StatusOK, langs)
}

// parseLangs extracts the "langs" query param (comma-separated) or returns [defaultLang].
func parseLangs(r *http.Request, defaultLang string) []string {
	if l := r.URL.Query().Get("langs"); l != "" {
		return strings.Split(l, ",")
	}
	if defaultLang == "" {
		defaultLang = "en"
	}
	return []string{defaultLang}
}

// Next returns the next card to study.
func (h *QuizHandler) Next(w http.ResponseWriter, r *http.Request) {
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	bucket := r.URL.Query().Get("bucket")

	// Load user settings first — per-user daily cap and baselines override the server default.
	userID := UserIDFromContext(r.Context())
	userSettings, _ := h.Store.GetUserSettings(r.Context(), userID)
	progCfg := sm2.DefaultProgressiveModeConfig()
	nwCfg := sm2.DefaultNewWordModeConfig()
	primaryLang := "en"
	// h.MaxNewPerDay=0 is a test sentinel meaning "block all new words".
	// Otherwise the per-user setting takes precedence over the server default.
	maxNew := h.MaxNewPerDay
	var baselines *db.NewWordBaselines
	if userSettings != nil {
		progCfg = userSettings.QuizConfig()
		nwCfg = userSettings.NewWordConfig()
		primaryLang = userSettings.PrimaryLang
		if h.MaxNewPerDay > 0 && userSettings.MaxNewWordsPerDay >= 1 {
			maxNew = userSettings.MaxNewWordsPerDay
		}
		baselines = &db.NewWordBaselines{
			DueTodayEnabled:   userSettings.BaselineDueTodayEnabled,
			DueTodayValue:     userSettings.BaselineDueTodayValue,
			StrugglingEnabled: userSettings.BaselineStrugglingEnabled,
			StrugglingValue:   userSettings.BaselineStrugglingValue,
			LearningEnabled:   userSettings.BaselineLearningEnabled,
			LearningValue:     userSettings.BaselineLearningValue,
			CooldownMinutes:   userSettings.NewWordCooldownMinutes,
		}
	}

	h.mu.Lock()
	cap := maxNew
	if h.capResetDate == time.Now().Format("2006-01-02") {
		extra := maxNew
		if extra < 1 {
			extra = 1
		}
		cap = h.newCapBase + extra
	}
	h.mu.Unlock()
	skipNew := r.URL.Query().Get("skip_new") == "true"

	var excludeIDs []int64
	if excl := r.URL.Query().Get("exclude"); excl != "" {
		for _, part := range strings.Split(excl, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
				excludeIDs = append(excludeIDs, id)
			}
		}
	}

	word, progress, err := h.Store.GetNextCard(r.Context(), userID, tags, cap, bucket, skipNew, baselines, excludeIDs)
	if err != nil {
		internalError(w, err)
		return
	}

	langs := parseLangs(r, primaryLang)
	mnemonics := r.URL.Query().Get("mnemonics") != "false"
	trainComponents := r.URL.Query().Get("trainComponents") == "1"

	// Ensure progress rows exist for any newly-named library entries.
	if err := h.Store.EnsureHMMProgress(r.Context(), UserIDFromContext(r.Context())); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch HMM mnemonic candidate.
	var hmmCard *models.HMMQuizCard
	if mnemonics {
		var hmmErr error
		hmmCard, _, hmmErr = h.Store.GetNextDueHMMCard(r.Context(), UserIDFromContext(r.Context()), nil)
		if hmmErr != nil {
			writeError(w, http.StatusInternalServerError, hmmErr.Error())
			return
		}
	}

	// Fetch component candidate (filtered to langs the user is currently training).
	var compCard *componentCard
	if trainComponents {
		cc, ccErr := h.Store.GetNextComponentCard(r.Context(), UserIDFromContext(r.Context()), langs)
		if ccErr != nil {
			writeError(w, http.StatusInternalServerError, ccErr.Error())
			return
		}
		if cc != nil {
			compCard = &componentCard{
				Character:   cc.Character,
				Pinyin:      cc.Pinyin,
				DueDate:     db.ParseDateTime(cc.Progress.DueDate),
				IsNew:       cc.Progress.FirstSeenDate == nil,
				Definitions: cc.Definitions,
			}
		}
	}

	// Pick the card with the lowest due_date across word, HMM, and component.
	// Candidates are ordered [word, hmm, component] so SelectNextCard reproduces
	// the historical tie-break (word > hmm > component on an exact due-date tie).
	wordCand := CardCandidate{Kind: cardWord, Present: word != nil, Word: word, WordProgress: progress}
	if word != nil {
		wordCand.DueDate = progress.DueDate
	}
	candidates := []CardCandidate{wordCand}
	if hmmCard != nil {
		candidates = append(candidates, CardCandidate{Kind: cardHMM, Present: true, DueDate: hmmCard.DueDate, HMM: hmmCard})
	}
	if compCard != nil {
		candidates = append(candidates, CardCandidate{Kind: cardComponent, Present: true, DueDate: compCard.DueDate, Component: compCard})
	}

	best, ok := SelectNextCard(candidates)
	switch {
	case ok && best.Kind == cardHMM:
		hc := best.HMM
		writeJSON(w, http.StatusOK, models.QuizCard{
			CardType:     "hmm",
			EntityType:   hc.EntityType,
			EntityKey:    hc.EntityKey,
			Prompt:       hc.Prompt,
			Category:     hc.Category,
			Hint:         hc.Hint,
			DueDate:      hc.DueDate,
			IntervalDays: hc.IntervalDays,
		})
		return
	case ok && best.Kind == cardComponent:
		cc := best.Component
		isAlsoWord, err := h.Store.IsZhWordForUser(r.Context(), UserIDFromContext(r.Context()), cc.Character)
		if err != nil {
			internalError(w, err)
			return
		}
		compQuizCard := models.QuizCard{
			CardType:    "component",
			Prompt:      cc.Character,
			DueDate:     cc.DueDate,
			IsNew:       cc.IsNew,
			Definitions: cc.Definitions,
			IsAlsoWord:  isAlsoWord,
		}
		if cc.Pinyin != "" {
			compQuizCard.Pinyin = &cc.Pinyin
		}
		writeJSON(w, http.StatusOK, compQuizCard)
		return
	}

	// The word candidate won (or nothing is present) — fall through to the word
	// serialisation below.
	if word == nil {
		writeError(w, http.StatusNotFound, "no words available")
		return
	}

	requestedMode := r.URL.Query().Get("mode")

	// Progressive mode: new words (total_attempts==0) are shown as introductions
	if progress.TotalAttempts == 0 {
		card := models.QuizCard{
			WordID:       word.ID,
			Mode:         models.ModeNewWord,
			Prompt:       word.Text,
			Pinyin:       word.Pinyin,
			DueDate:      progress.DueDate,
			IntervalDays: progress.IntervalDays,
		}
		card.Translations = map[string][]string{}
		for _, lang := range langs {
			transWords, err := h.Store.GetTranslationsForWord(r.Context(), word.ID, lang)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if len(transWords) > 0 {
				texts := make([]string, len(transWords))
				for i, tw := range transWords {
					texts[i] = tw.Text
				}
				card.Translations[lang] = texts
			}
		}
		writeJSON(w, http.StatusOK, card)
		return
	}

	var mode string
	switch requestedMode {
	case models.ModeTranslToZh, models.ModeZhToTransl, models.ModeZhPinyinToTransl, models.ModeMaskPinyin:
		mode = requestedMode
	case models.ModeProgressive:
		if progress.LearningNewWord {
			mode = sm2.SelectNewWordMode(progress.TotalCorrect, nwCfg)
		} else {
			mode = sm2.SelectProgressiveMode(progress.TotalCorrect, progress.TotalAttempts, progress.StreakBonus, progCfg)
		}
	case models.ModeCycle:
		seqStr := sm2.DefaultCycleSequence
		if userSettings != nil && userSettings.CycleSequence != "" {
			seqStr = userSettings.CycleSequence
		}
		cycleCounter := progress.TotalAttempts
		if userSettings != nil && userSettings.CycleAdvanceOnSuccessOnly {
			cycleCounter = progress.TotalCorrect
		}
		mode = sm2.SelectCycleMode(cycleCounter, sm2.ParseCycleSequence(seqStr))
	default:
		mode = sm2.SelectMode()
	}

	// mask_pinyin resolves to transl_to_zh with the pinyin hint forced on.
	forceMaskPinyin := mode == models.ModeMaskPinyin
	if forceMaskPinyin {
		mode = models.ModeTranslToZh
	}

	// zh_pinyin_to_transl requires pinyin; fall back if missing
	if mode == models.ModeZhPinyinToTransl && (word.Pinyin == nil || *word.Pinyin == "") {
		mode = models.ModeZhToTransl
	}

	card := models.QuizCard{
		WordID:          word.ID,
		Mode:            mode,
		DueDate:         progress.DueDate,
		IntervalDays:    progress.IntervalDays,
		LearningNewWord: progress.LearningNewWord,
	}

	switch mode {
	case models.ModeTranslToZh:
		// Load translations for ALL selected langs so the user sees every meaning as context.
		translations := map[string][]string{}
		for _, lang := range langs {
			words, err := h.Store.GetTranslationsForWord(r.Context(), word.ID, lang)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			for _, w := range words {
				translations[lang] = append(translations[lang], w.Text)
			}
		}
		if len(translations) == 0 {
			card.Mode = models.ModeZhToTransl
			card.Prompt = word.Text
		} else {
			// Use the first translation of the first selected lang with results as the prompt word.
			for _, lang := range langs {
				if texts := translations[lang]; len(texts) > 0 {
					card.Prompt = texts[0]
					break
				}
			}
		}
		card.Translations = translations
		// Apply pinyin hint when the word is in the learning phase or mask_pinyin was requested.
		// Cycle mode skips the learning-phase hint: the user chose the step explicitly.
		if word.Pinyin != nil {
			if forceMaskPinyin && !progress.LearningNewWord {
				// Tier-based mask_pinyin: always show a level-0 masked hint (first char of each syllable).
				if masked := sm2.MaskPinyin(*word.Pinyin, 0); masked != "" {
					card.Pinyin = &masked
				}
			} else if progress.LearningNewWord && requestedMode != models.ModeCycle {
				// Intro-phase: fade out hint as correct answers accumulate.
				if masked := sm2.MaskPinyin(*word.Pinyin, progress.TotalCorrect); masked != "" {
					card.Pinyin = &masked
				}
			}
		}
	case models.ModeZhToTransl:
		card.Prompt = word.Text
	case models.ModeZhPinyinToTransl:
		card.Prompt = word.Text
		card.Pinyin = word.Pinyin
	}

	writeJSON(w, http.StatusOK, card)
}

// Answer processes a submitted answer and updates SM-2 progress.
func (h *QuizHandler) Answer(w http.ResponseWriter, r *http.Request) {
	var req models.AnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Answer = strings.TrimSpace(req.Answer)
	if req.WordID <= 0 {
		writeError(w, http.StatusBadRequest, "word_id is required")
		return
	}
	validModes := map[string]bool{
		models.ModeTranslToZh:       true,
		models.ModeZhToTransl:       true,
		models.ModeZhPinyinToTransl: true,
	}
	if !validModes[req.Mode] {
		writeError(w, http.StatusBadRequest, "invalid mode")
		return
	}

	// Look up the zh word (word_id is always the zh word)
	zhWord, err := h.Store.GetWordByID(r.Context(), UserIDFromContext(r.Context()), req.WordID)
	if err != nil {
		internalError(w, err)
		return
	}
	if zhWord == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}

	langs := req.Langs
	if len(langs) == 0 {
		if st, _ := h.Store.GetUserSettings(r.Context(), UserIDFromContext(r.Context())); st != nil {
			langs = []string{st.PrimaryLang}
		} else {
			langs = []string{"en"}
		}
	}
	userID := UserIDFromContext(r.Context())
	var correctTexts []string
	switch req.Mode {
	case models.ModeTranslToZh:
		correctTexts = []string{zhWord.ZhText}
		// Also accept any other zh word whose translation text matches one of
		// this word's translations (e.g. 错 and 不对 both → "falsch").
		for _, lang := range langs {
			transWords, err := h.Store.GetTranslationsForWord(r.Context(), req.WordID, lang)
			if err != nil {
				internalError(w, err)
				return
			}
			for _, tw := range transWords {
				equiv, err := h.Store.GetZhWordsWithTranslation(r.Context(), userID, lang, tw.Text)
				if err != nil {
					internalError(w, err)
					return
				}
				correctTexts = append(correctTexts, equiv...)
			}
		}
	case models.ModeZhToTransl, models.ModeZhPinyinToTransl:
		for _, lang := range langs {
			transWords, err := h.Store.GetTranslationsForWord(r.Context(), req.WordID, lang)
			if err != nil {
				internalError(w, err)
				return
			}
			for _, tw := range transWords {
				correctTexts = append(correctTexts, tw.Text)
			}
		}
	}

	correct := sm2.CheckAnswer(req.Answer, correctTexts)

	progress, err := h.Store.GetSM2Progress(r.Context(), req.WordID)
	if err != nil {
		internalError(w, err)
		return
	}
	if progress == nil {
		writeError(w, http.StatusNotFound, "progress not found")
		return
	}
	prevTier := sm2.ClassifyTier(*progress).String()

	updated := sm2.ProcessAnswer(*progress, correct)
	graduated := progress.LearningNewWord && !updated.LearningNewWord

	if err := h.Store.UpdateSM2Progress(r.Context(), updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		internalError(w, err)
		return
	}

	ctx := r.Context()
	if correct {
		if err := h.Store.ClearSM2PrevState(ctx, req.WordID); err != nil {
			log.Printf("answer: ClearSM2PrevState word %d: %v", req.WordID, err)
		}
	} else {
		if err := h.Store.SaveSM2PrevState(ctx, req.WordID, *progress); err != nil {
			log.Printf("answer: SaveSM2PrevState word %d: %v", req.WordID, err)
		}
	}

	sessionStreak, err := h.Store.RecordDailyStat(r.Context(), UserIDFromContext(r.Context()), correct)
	if err != nil {
		log.Printf("answer: RecordDailyStat user %d: %v", UserIDFromContext(r.Context()), err)
	}

	resp := models.AnswerResponse{
		Correct:         correct,
		CorrectAnswers:  correctTexts,
		ZhText:          zhWord.ZhText,
		Pinyin:          zhWord.Pinyin,
		Translations:    zhWord.Translations,
		NextDue:         updated.DueDate,
		IntervalDays:    updated.IntervalDays,
		TotalCorrect:    updated.TotalCorrect,
		TotalAttempts:   updated.TotalAttempts,
		StreakBonus:     updated.StreakBonus,
		Repetitions:     updated.Repetitions,
		GraduateReps:    sm2.LearningGraduateReps,
		LearningNewWord: updated.LearningNewWord,
		Graduated:       graduated,
	}

	if sceneText, err := h.Store.GetHMMSceneText(r.Context(), req.WordID); err != nil {
		log.Printf("answer: GetHMMSceneText word %d: %v", req.WordID, err)
	} else {
		resp.SceneText = sceneText
	}

	if correct {
		if sessionStreak > 1 {
			resp.SessionStreak = sessionStreak
		}
		resp.Tier = sm2.ClassifyTier(updated).String()
		if prevTier != "" && prevTier != resp.Tier {
			resp.PrevTier = prevTier
		}
	}

	if !correct {
		confusedWithID, found, err := h.Store.DetectConfusion(r.Context(), UserIDFromContext(r.Context()), req.WordID, req.Answer, req.Mode, langs)
		if err == nil && found {
			_ = h.Store.UpsertConfusion(r.Context(), req.WordID, confusedWithID, req.Mode)
			confusions, err := h.Store.GetConfusionDetail(r.Context(), req.WordID, confusedWithID, req.Mode, langs)
			if err == nil {
				resp.ConfusedWith = confusions
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// AcceptCorrect handles POST /api/quiz/accept-correct. It restores the pre-answer
// SM-2 state stored by Answer() on a wrong submission and applies a correct-quality
// update, giving the same result as if the user had answered correctly the first time.
// No SM-2 state is accepted from the client — the DB column is the sole source of truth.
func (h *QuizHandler) AcceptCorrect(w http.ResponseWriter, r *http.Request) {
	var req models.AcceptCorrectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.WordID <= 0 {
		writeError(w, http.StatusBadRequest, "word_id is required")
		return
	}
	validModes := map[string]bool{
		models.ModeTranslToZh:       true,
		models.ModeZhToTransl:       true,
		models.ModeZhPinyinToTransl: true,
	}
	if !validModes[req.Mode] && req.Mode != "" {
		writeError(w, http.StatusBadRequest, "invalid mode")
		return
	}

	ctx := r.Context()
	userID := UserIDFromContext(ctx)

	zhWord, err := h.Store.GetWordByID(ctx, userID, req.WordID)
	if err != nil {
		internalError(w, err)
		return
	}
	if zhWord == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}

	prev, err := h.Store.GetSM2PrevState(ctx, req.WordID)
	if err != nil {
		internalError(w, err)
		return
	}
	if prev == nil {
		writeError(w, http.StatusNotFound, "no pending accept-correct for this word")
		return
	}

	updated := sm2.ProcessAnswer(*prev, true)
	graduated := prev.LearningNewWord && !updated.LearningNewWord

	if err := h.Store.UpdateSM2Progress(ctx, updated); err != nil {
		internalError(w, err)
		return
	}
	_ = h.Store.ClearSM2PrevState(ctx, req.WordID)

	sessionStreak, _ := h.Store.RecordDailyStat(ctx, userID, true)

	langs := req.Langs
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	var correctTexts []string
	switch req.Mode {
	case models.ModeTranslToZh:
		correctTexts = []string{zhWord.ZhText}
	default:
		for _, lang := range langs {
			transWords, err := h.Store.GetTranslationsForWord(ctx, req.WordID, lang)
			if err != nil {
				internalError(w, err)
				return
			}
			for _, tw := range transWords {
				correctTexts = append(correctTexts, tw.Text)
			}
		}
	}

	resp := models.AnswerResponse{
		Correct:         true,
		CorrectAnswers:  correctTexts,
		ZhText:          zhWord.ZhText,
		Pinyin:          zhWord.Pinyin,
		Translations:    zhWord.Translations,
		NextDue:         updated.DueDate,
		IntervalDays:    updated.IntervalDays,
		TotalCorrect:    updated.TotalCorrect,
		TotalAttempts:   updated.TotalAttempts,
		StreakBonus:     updated.StreakBonus,
		Repetitions:     updated.Repetitions,
		GraduateReps:    sm2.LearningGraduateReps,
		LearningNewWord: updated.LearningNewWord,
		Graduated:       graduated,
	}
	if sessionStreak > 1 {
		resp.SessionStreak = sessionStreak
	}
	resp.SceneText, _ = h.Store.GetHMMSceneText(ctx, req.WordID)
	writeJSON(w, http.StatusOK, resp)
}

// DailyStats returns the full daily stats history.
func (h *QuizHandler) DailyStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Store.GetDailyStatsHistory(r.Context(), UserIDFromContext(r.Context()))
	if err != nil {
		internalError(w, err)
		return
	}
	resp := models.DailyStatsResponse{Days: make([]models.DailyStatEntry, len(stats))}
	for i, s := range stats {
		resp.Days[i] = models.DailyStatEntry{
			Date:             s.Date,
			Attempts:         s.Attempts,
			Mistakes:         s.Mistakes,
			WordsSeen:        s.WordsSeen,
			CorrectStreak:    s.CorrectStreak,
			BucketNew:        s.BucketNew,
			BucketStruggling: s.BucketStruggling,
			BucketLearning:   s.BucketLearning,
			BucketPracticing: s.BucketPracticing,
			BucketMastered:   s.BucketMastered,
			TrainingSeconds:  s.TrainingSeconds,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// RecordTime accumulates focused training seconds for today's daily stats.
func (h *QuizHandler) RecordTime(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seconds int `json:"seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Seconds <= 0 || req.Seconds > 3600 {
		writeError(w, http.StatusBadRequest, "seconds must be between 1 and 3600")
		return
	}
	if err := h.Store.RecordTrainingTime(r.Context(), UserIDFromContext(r.Context()), req.Seconds); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Skip moves a word's due date forward by the requested number of days
// (default 7) without marking it as seen.
func (h *QuizHandler) Skip(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WordID int64 `json:"word_id"`
		Days   int   `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.WordID <= 0 {
		writeError(w, http.StatusBadRequest, "word_id is required")
		return
	}
	if req.Days <= 0 {
		req.Days = 7
	}

	userID := UserIDFromContext(r.Context())

	// When the user has hidden the new-word skip button, reject skip attempts
	// for words still in the new-word introduction phase.
	st, _ := h.Store.GetUserSettings(r.Context(), userID)
	if st != nil && !st.SkipNewWordsVisible {
		isNew, err := h.Store.IsLearningNewWord(r.Context(), userID, req.WordID)
		if err == nil && isNew {
			writeError(w, http.StatusBadRequest, "skipping new words is disabled")
			return
		}
	}

	if err := h.Store.SkipWord(r.Context(), userID, req.WordID, req.Days); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Acknowledge marks a new word as "introduced" so it becomes available for quizzing.
func (h *QuizHandler) Acknowledge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WordID int64 `json:"word_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.WordID <= 0 {
		writeError(w, http.StatusBadRequest, "word_id is required")
		return
	}
	userID := UserIDFromContext(r.Context())
	if err := h.Store.AcknowledgeWord(r.Context(), userID, req.WordID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		internalError(w, err)
		return
	}
	zhText, err := h.Store.GetZhTextByID(r.Context(), userID, req.WordID)
	if err == nil && zhText != "" {
		initComponents(r.Context(), h.Store, userID, req.WordID, zhText)
	}
	w.WriteHeader(http.StatusNoContent)
}

// WordStats returns aggregate per-word statistics for all seen words.
func (h *QuizHandler) WordStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Store.GetWordStats(r.Context(), UserIDFromContext(r.Context()))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// Stats returns due-today and total card counts, plus today's session info.
func (h *QuizHandler) Stats(w http.ResponseWriter, r *http.Request) {
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	bucket := r.URL.Query().Get("bucket")
	userID := UserIDFromContext(r.Context())
	due, total, newToday, err := h.Store.GetStats(r.Context(), userID, tags, bucket)
	if err != nil {
		internalError(w, err)
		return
	}
	todayAttempts, todayMistakes, availableToAdvance, err := h.Store.GetTodaySessionInfo(r.Context(), userID)
	if err != nil {
		internalError(w, err)
		return
	}
	maxNew := h.MaxNewPerDay
	userSettings, _ := h.Store.GetUserSettings(r.Context(), userID)
	if userSettings != nil && h.MaxNewPerDay > 0 && userSettings.MaxNewWordsPerDay >= 1 {
		maxNew = userSettings.MaxNewWordsPerDay
	}
	h.mu.Lock()
	cap := maxNew
	if h.capResetDate == time.Now().Format("2006-01-02") {
		extra := maxNew
		if extra < 1 {
			extra = 1
		}
		cap = h.newCapBase + extra
	}
	h.mu.Unlock()
	newAvailable := 0
	// When drilling a specific tier, don't introduce new words (they have no tier yet).
	if bucket == "" && newToday < cap {
		n, err := h.Store.CountUnseenZhWords(r.Context(), UserIDFromContext(r.Context()), tags)
		if err != nil {
			internalError(w, err)
			return
		}
		if remaining := cap - newToday; n > remaining {
			n = remaining
		}
		newAvailable = n
	}
	mnemonics := r.URL.Query().Get("mnemonics") != "false"
	hmmDueToday := 0
	hmmTotal := 0
	if mnemonics {
		hmmStats, err := h.Store.GetHMMStats(r.Context(), UserIDFromContext(r.Context()), nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		hmmDueToday = hmmStats.DueToday
		hmmTotal = hmmStats.Total
	}
	compDueToday := 0
	compTotal := 0
	if r.URL.Query().Get("trainComponents") == "1" {
		var cErr error
		compDueToday, compTotal, cErr = h.Store.GetComponentCounts(r.Context(), UserIDFromContext(r.Context()))
		if cErr != nil {
			writeError(w, http.StatusInternalServerError, cErr.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"due_today":            due,
		"total":                total,
		"new_today":            newToday,
		"max_new_per_day":      maxNew,
		"today_attempts":       todayAttempts,
		"today_mistakes":       todayMistakes,
		"available_to_advance": availableToAdvance,
		"new_available":        newAvailable,
		"hmm_due_today":        hmmDueToday,
		"hmm_total":            hmmTotal,
		"components_due_today": compDueToday,
		"components_total":     compTotal,
	})
}

// DueDateDistribution returns word counts grouped by due date, optionally filtered by tags.
func (h *QuizHandler) DueDateDistribution(w http.ResponseWriter, r *http.Request) {
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	dates, err := h.Store.GetWordCountByDueDate(r.Context(), UserIDFromContext(r.Context()), tags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dates == nil {
		dates = []models.DueDateCount{}
	}
	writeJSON(w, http.StatusOK, models.DueDateDistributionResponse{Dates: dates})
}

// AcknowledgeRandom marks up to n random unseen zh words as due now, bypassing
// the new-word introduction flow. Used after onboarding import.
func (h *QuizHandler) AcknowledgeRandom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Count <= 0 {
		writeError(w, http.StatusBadRequest, "count must be positive")
		return
	}
	n, err := h.Store.AcknowledgeRandomWords(r.Context(), UserIDFromContext(r.Context()), req.Count)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"acknowledged": n})
}

// Advance pulls forward the due dates of n seen zh words so they become due now,
// and optionally resets the daily new-word cap for the rest of the day.
func (h *QuizHandler) Advance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Count       int  `json:"count"`
		ResetNewCap bool `json:"reset_new_cap"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	advanced := 0
	if req.Count > 0 {
		n, err := h.Store.AdvanceDueDates(r.Context(), UserIDFromContext(r.Context()), req.Count)
		if err != nil {
			internalError(w, err)
			return
		}
		advanced = n
	}
	if req.ResetNewCap {
		_, _, newToday, err := h.Store.GetStats(r.Context(), UserIDFromContext(r.Context()), nil, "")
		if err == nil {
			h.mu.Lock()
			h.capResetDate = time.Now().Format("2006-01-02")
			h.newCapBase = newToday
			h.mu.Unlock()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"advanced":  advanced,
		"cap_reset": req.ResetNewCap,
	})
}

// MatchGame handles GET /api/quiz/match-game.
// Loads 2 recent confusion pairs (each with zh_word + confused_with), extracts
// up to 4 unique words, marks the pairs as shown, and returns the word list.
// Returns {words:[]} when fewer than 2 eligible pairs exist.
func (h *QuizHandler) MatchGame(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	since := time.Now().UTC().AddDate(0, 0, -7)
	pairs, err := h.Store.GetRecentMismatches(r.Context(), userID, since, 2)
	if err != nil {
		internalError(w, err)
		return
	}
	if len(pairs) < 2 {
		writeJSON(w, http.StatusOK, models.MatchGameResponse{Words: []models.MatchGameWord{}})
		return
	}

	ptrStr := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	// Collect unique words from both zh_word and confused_with sides of each pair.
	seen := map[int64]bool{}
	var words []models.MatchGameWord
	for _, p := range pairs {
		for _, candidate := range []struct {
			id           int64
			text, pinyin string
			translations map[string][]string
		}{
			{p.ZhWordID, p.ZhText, ptrStr(p.ZhPinyin), p.ZhTranslations},
			{p.ConfusedWithID, p.ConfusedWithText, ptrStr(p.ConfusedWithPinyin), p.ConfusedWithTranslations},
		} {
			if !seen[candidate.id] {
				seen[candidate.id] = true
				words = append(words, models.MatchGameWord{
					ZhWordID:     candidate.id,
					ZhText:       candidate.text,
					Pinyin:       candidate.pinyin,
					Translations: candidate.translations,
				})
			}
		}
	}

	// Mark the source pairs as shown so they are suppressed until re-confused.
	pairKeys := make([][2]int64, len(pairs))
	for i, p := range pairs {
		pairKeys[i] = [2]int64{p.ZhWordID, p.ConfusedWithID}
	}
	_ = h.Store.MarkConfusionsShownInGame(r.Context(), pairKeys)

	writeJSON(w, http.StatusOK, models.MatchGameResponse{Words: words})
}

// MatchAnswer handles POST /api/quiz/match-answer.
// Updates SM-2 progress for a word after a match-game interaction.
func (h *QuizHandler) MatchAnswer(w http.ResponseWriter, r *http.Request) {
	var req models.MatchAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.ZhWordID <= 0 {
		writeError(w, http.StatusBadRequest, "zh_word_id is required")
		return
	}

	userID := UserIDFromContext(r.Context())
	zhWord, err := h.Store.GetWordByID(r.Context(), userID, req.ZhWordID)
	if err != nil {
		internalError(w, err)
		return
	}
	if zhWord == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}

	progress, err := h.Store.GetSM2Progress(r.Context(), req.ZhWordID)
	if err != nil {
		internalError(w, err)
		return
	}
	if progress == nil {
		writeError(w, http.StatusNotFound, "progress not found")
		return
	}

	quality := sm2.QualityWrong
	if req.Correct {
		quality = sm2.QualityCorrect
	}

	updated := sm2.Update(*progress, quality)
	updated.TotalAttempts++
	if req.Correct {
		updated.TotalCorrect++
	}
	updated.StreakBonus = sm2.CalcStreakBonus(updated.StreakBonus, updated.Repetitions, updated.TotalCorrect, updated.TotalAttempts)

	if err := h.Store.UpdateSM2Progress(r.Context(), updated); err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.AnswerResponse{
		Correct:       req.Correct,
		ZhText:        zhWord.ZhText,
		Pinyin:        zhWord.Pinyin,
		NextDue:       updated.DueDate,
		IntervalDays:  updated.IntervalDays,
		TotalCorrect:  updated.TotalCorrect,
		TotalAttempts: updated.TotalAttempts,
		StreakBonus:   updated.StreakBonus,
		Repetitions:   updated.Repetitions,
		Tier:          sm2.ClassifyTier(updated).String(),
	})
}
