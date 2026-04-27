package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

type QuizHandler struct {
	Store        *db.Store
	MaxNewPerDay int
	mu           sync.Mutex
	capResetDate string // date string (YYYY-MM-DD) on which the new-word cap was reset
	newCapBase   int    // newToday count at cap-reset time; cap = newCapBase + MaxNewPerDay
}

// wordTier returns the accuracy bucket label for a progress record.
// Must stay in sync with wordTier() in app.js and tierFilter() in db.go.
func wordTier(p models.SM2Progress) string {
	if p.TotalAttempts == 0 {
		return ""
	}
	if p.LearningNewWord {
		return "New"
	}
	acc := float64(p.TotalCorrect+p.StreakBonus) / float64(p.TotalAttempts)
	switch {
	case p.TotalAttempts >= 10 && acc >= 0.85:
		return "Mastered"
	case p.TotalAttempts >= 10 && acc >= 0.70:
		return "Practicing"
	case p.TotalAttempts >= 3 && acc >= 0.50:
		return "Learning"
	default:
		return "Struggling"
	}
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
	h.mu.Lock()
	cap := h.MaxNewPerDay
	if h.capResetDate == time.Now().Format("2006-01-02") {
		extra := h.MaxNewPerDay
		if extra < 1 {
			extra = 1
		}
		cap = h.newCapBase + extra
	}
	h.mu.Unlock()
	skipNew := r.URL.Query().Get("skip_new") == "true"
	word, progress, err := h.Store.GetNextCard(r.Context(), UserIDFromContext(r.Context()), tags, cap, bucket, skipNew)
	if err != nil {
		internalError(w, err)
		return
	}

	// Load user settings once; used for quiz mode config and language defaults.
	userSettings, _ := h.Store.GetUserSettings(r.Context(), UserIDFromContext(r.Context()))
	progCfg := sm2.DefaultProgressiveModeConfig()
	nwCfg := sm2.DefaultNewWordModeConfig()
	primaryLang := "en"
	if userSettings != nil {
		progCfg = sm2.ProgressiveModeConfig{
			New:        userSettings.ProgNew,
			Struggling: userSettings.ProgTierStruggling,
			Learning:   userSettings.ProgTierLearning,
			Practicing: userSettings.ProgTierPracticing,
			Mastered:   userSettings.ProgTierMastered,
		}
		nwCfg = sm2.NewWordModeConfig{
			Step0: userSettings.NewWordMode0,
			Step1: userSettings.NewWordMode1,
			Step2: userSettings.NewWordMode2,
		}
		primaryLang = userSettings.PrimaryLang
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
	var compCard *struct {
		Character   string
		DueDate     time.Time
		IsNew       bool
		Definitions map[string]string
	}
	if trainComponents {
		cc, ccErr := h.Store.GetNextComponentCard(r.Context(), UserIDFromContext(r.Context()), langs)
		if ccErr != nil {
			writeError(w, http.StatusInternalServerError, ccErr.Error())
			return
		}
		if cc != nil {
			compCard = &struct {
				Character   string
				DueDate     time.Time
				IsNew       bool
				Definitions map[string]string
			}{
				Character:   cc.Character,
				DueDate:     db.ParseDateTime(cc.Progress.DueDate),
				IsNew:       cc.Progress.FirstSeenDate == nil,
				Definitions: cc.Definitions,
			}
		}
	}

	// Pick the card with the lowest due_date across word, HMM, and component.
	// HMM check.
	if hmmCard != nil {
		serveHMM := word == nil || hmmCard.DueDate.Before(progress.DueDate)
		if compCard != nil && serveHMM {
			serveHMM = hmmCard.DueDate.Before(compCard.DueDate) || hmmCard.DueDate.Equal(compCard.DueDate)
		}
		if serveHMM {
			writeJSON(w, http.StatusOK, models.QuizCard{
				CardType:     "hmm",
				EntityType:   hmmCard.EntityType,
				EntityKey:    hmmCard.EntityKey,
				Prompt:       hmmCard.Prompt,
				Category:     hmmCard.Category,
				Hint:         hmmCard.Hint,
				DueDate:      hmmCard.DueDate,
				IntervalDays: hmmCard.IntervalDays,
			})
			return
		}
	}

	// Component check.
	if compCard != nil {
		serveComp := word == nil || compCard.DueDate.Before(progress.DueDate)
		if serveComp {
			writeJSON(w, http.StatusOK, models.QuizCard{
				CardType:    "component",
				Prompt:      compCard.Character,
				DueDate:     compCard.DueDate,
				IsNew:       compCard.IsNew,
				Definitions: compCard.Definitions,
			})
			return
		}
	}

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
			card.Translations = translations
			// Apply pinyin hint when the word is in the learning phase or mask_pinyin was requested.
			if (progress.LearningNewWord || forceMaskPinyin) && word.Pinyin != nil {
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
	var correctTexts []string
	switch req.Mode {
	case models.ModeTranslToZh:
		correctTexts = []string{zhWord.ZhText}
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
	quality := sm2.QualityWrong
	if correct {
		quality = sm2.QualityCorrect
	}

	progress, err := h.Store.GetSM2Progress(r.Context(), req.WordID)
	if err != nil {
		internalError(w, err)
		return
	}
	if progress == nil {
		writeError(w, http.StatusNotFound, "progress not found")
		return
	}
	prevTier := wordTier(*progress)

	var updated models.SM2Progress
	var graduated bool
	if progress.LearningNewWord {
		updated, graduated = sm2.UpdateLearning(*progress, quality)
		if !graduated {
			updated.TotalAttempts++
			if correct {
				updated.TotalCorrect++
			}
		}
	} else {
		updated = sm2.Update(*progress, quality)
		updated.TotalAttempts++
		if correct {
			updated.TotalCorrect++
		}
	}
	updated.StreakBonus = sm2.CalcStreakBonus(updated.StreakBonus, updated.Repetitions, updated.TotalCorrect, updated.TotalAttempts)

	if err := h.Store.UpdateSM2Progress(r.Context(), updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		internalError(w, err)
		return
	}

	sessionStreak, _ := h.Store.RecordDailyStat(r.Context(), UserIDFromContext(r.Context()), correct)

	resp := models.AnswerResponse{
		Correct:        correct,
		CorrectAnswers: correctTexts,
		ZhText:         zhWord.ZhText,
		Pinyin:         zhWord.Pinyin,
		Translations:   zhWord.Translations,
		NextDue:        updated.DueDate,
		IntervalDays:    updated.IntervalDays,
		TotalCorrect:    updated.TotalCorrect,
		TotalAttempts:   updated.TotalAttempts,
		StreakBonus:     updated.StreakBonus,
		Repetitions:     updated.Repetitions,
		GraduateReps:    sm2.LearningGraduateReps,
		LearningNewWord: updated.LearningNewWord,
		Graduated:       graduated,
	}

	resp.SceneText, _ = h.Store.GetHMMSceneText(r.Context(), req.WordID)

	if correct {
		if sessionStreak > 1 {
			resp.SessionStreak = sessionStreak
		}
		resp.Tier = wordTier(updated)
		if prevTier != "" && prevTier != resp.Tier {
			resp.PrevTier = prevTier
		}
	}

	if !correct {
		confusedWithID, found, err := h.Store.LookupConfusion(r.Context(), UserIDFromContext(r.Context()), req.WordID, req.Answer, req.Mode, langs)
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
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// Skip moves a word's due date forward by 7 days without marking it as seen.
func (h *QuizHandler) Skip(w http.ResponseWriter, r *http.Request) {
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
	if err := h.Store.SkipWord(r.Context(), UserIDFromContext(r.Context()), req.WordID, 7); err != nil {
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
	due, total, newToday, err := h.Store.GetStats(r.Context(), UserIDFromContext(r.Context()), tags, bucket)
	if err != nil {
		internalError(w, err)
		return
	}
	todayAttempts, todayMistakes, availableToAdvance, err := h.Store.GetTodaySessionInfo(r.Context(), UserIDFromContext(r.Context()))
	if err != nil {
		internalError(w, err)
		return
	}
	h.mu.Lock()
	cap := h.MaxNewPerDay
	if h.capResetDate == time.Now().Format("2006-01-02") {
		extra := h.MaxNewPerDay
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
		"due_today":              due,
		"total":                  total,
		"new_today":              newToday,
		"max_new_per_day":        h.MaxNewPerDay,
		"today_attempts":         todayAttempts,
		"today_mistakes":         todayMistakes,
		"available_to_advance":   availableToAdvance,
		"new_available":          newAvailable,
		"hmm_due_today":          hmmDueToday,
		"hmm_total":              hmmTotal,
		"components_due_today":   compDueToday,
		"components_total":       compTotal,
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
