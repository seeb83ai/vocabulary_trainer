package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
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
	var randCfg sm2.RandomModeConfig
	primaryLang := "en"
	// h.MaxNewPerDay=0 is a test sentinel meaning "block all new words".
	// Otherwise the per-user setting takes precedence over the server default.
	maxNew := h.MaxNewPerDay
	var baselines *db.NewWordBaselines
	if userSettings != nil {
		progCfg = userSettings.QuizConfig()
		nwCfg = userSettings.NewWordConfig()
		randCfg = userSettings.RandomModeConfig()
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
			NewBucketEnabled:  userSettings.BaselineNewBucketEnabled,
			NewBucketValue:    userSettings.BaselineNewBucketValue,
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
	var excludeComponents []string
	if excl := r.URL.Query().Get("exclude_components"); excl != "" {
		for _, part := range strings.Split(excl, ",") {
			if part = strings.TrimSpace(part); part != "" {
				excludeComponents = append(excludeComponents, part)
			}
		}
	}

	// Difficult-words drill: serve only flagged words (ordered by due date,
	// ignoring the normal due-date horizon). Mnemonic/component candidates are
	// not part of the drill.
	difficult := r.URL.Query().Get("difficult") == "true"

	langs := parseLangs(r, primaryLang)

	// Sentence fill-in-the-blank: when enabled, roll against the user's
	// configured ratio to decide whether to attempt serving a sentence-blank
	// card this turn instead of a normal word/HMM/component card. It shares
	// sm2_progress with normal word quizzing (see CLAUDE.md), so this bypasses
	// the HMM/component candidate selection below entirely on a hit.
	if !difficult && userSettings != nil && userSettings.SentenceBlankEnabled &&
		rand.Intn(100) < userSettings.SentenceBlankRatio {
		card, err := h.Store.NextSentenceBlankCard(r.Context(), userID, progCfg, nwCfg, langs)
		if err != nil {
			internalError(w, err)
			return
		}
		if card != nil {
			writeJSON(w, http.StatusOK, *card)
			return
		}
	}
	// Opt-in (default): allow GetNextCard to widen beyond today's due-date bound
	// and serve a not-yet-due word so a just-answered word isn't immediately
	// repeated. Users can disable this in settings to only ever be served
	// genuinely due-today cards.
	allowSessionExtension := userSettings == nil || userSettings.ExtendSessionWithExtraWords
	var word *models.Word
	var progress *models.SM2Progress
	var sessionExtension bool
	var err error
	if difficult {
		word, progress, err = h.Store.GetNextDrillCard(r.Context(), userID)
	} else {
		word, progress, sessionExtension, err = h.Store.GetNextCard(r.Context(), userID, tags, cap, bucket, skipNew, baselines, excludeIDs, allowSessionExtension)
	}
	if err != nil {
		internalError(w, err)
		return
	}

	mnemonics := r.URL.Query().Get("mnemonics") != "false" && !difficult
	trainComponents := r.URL.Query().Get("trainComponents") == "1" && !difficult

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
		cc, ccErr := h.Store.GetNextComponentCard(r.Context(), UserIDFromContext(r.Context()), langs, excludeComponents)
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

	// Reciprocal of the component card's IsAlsoWord: flag word cards whose zh
	// text is also tracked as a hanzi component.
	isAlsoComponent, err := h.Store.IsComponentForUser(r.Context(), UserIDFromContext(r.Context()), word.Text)
	if err != nil {
		internalError(w, err)
		return
	}

	requestedMode := r.URL.Query().Get("mode")

	// Progressive mode: new words (total_attempts==0) are shown as introductions
	if progress.TotalAttempts == 0 {
		card := models.QuizCard{
			WordID:           word.ID,
			Mode:             models.ModeNewWord,
			Prompt:           word.Text,
			Pinyin:           fullPinyinForDisplay(word.Text, word.Pinyin),
			DueDate:          progress.DueDate,
			IntervalDays:     progress.IntervalDays,
			SessionExtension: sessionExtension,
			IsAlsoComponent:  isAlsoComponent,
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
	case models.ModeTranslToZh, models.ModeZhToTransl, models.ModeZhPinyinToTransl, models.ModeMaskPinyin, models.ModeZhToTranslNoSound, models.ModeVoiceToTransl:
		mode = requestedMode
	case models.ModeProgressive:
		if progress.LearningNewWord {
			mode = sm2.SelectNewWordMode(progress.TotalCorrect, nwCfg)
		} else {
			mode = sm2.SelectProgressiveMode(progress.TotalCorrect, progress.TotalAttempts, progress.StreakBonus, progCfg)
		}
	case models.ModeCycle:
		// Under advance-only-if-known/success, a wrong answer leaves the cycle
		// position counter unchanged but can still shift the word's accuracy-tier
		// bucket, which would otherwise re-filter the configured sequence into a
		// different mode at the same position. Pin to the exact mode last shown
		// (set by Answer on a wrong submission, cleared once the position actually
		// advances) so the unresolved encounter keeps repeating the same question.
		var pinnedMode string
		if userSettings != nil && (userSettings.CycleAdvanceOnKnownOnly || userSettings.CycleAdvanceOnSuccessOnly) {
			pinnedMode, err = h.Store.GetCyclePinMode(r.Context(), word.ID)
			if err != nil {
				log.Printf("quiz next: GetCyclePinMode word %d: %v", word.ID, err)
				pinnedMode = ""
			}
		}
		if pinnedMode != "" {
			mode = pinnedMode
		} else {
			seqStr := sm2.DefaultCycleSequence
			if userSettings != nil && userSettings.CycleSequence != "" {
				seqStr = userSettings.CycleSequence
			}
			cycleCounter := progress.TotalAttempts
			if userSettings != nil && userSettings.CycleAdvanceOnKnownOnly {
				cycleCounter = progress.KnownCorrectCount
			} else if userSettings != nil && userSettings.CycleAdvanceOnSuccessOnly {
				cycleCounter = progress.TotalCorrect
			}
			bucket := sm2.ClassifyTier(*progress).BucketKey()
			mode = sm2.SelectCycleMode(cycleCounter, sm2.ParseCycleSequence(seqStr), bucket, randCfg)
		}
	default:
		bucket := sm2.ClassifyTier(*progress).BucketKey()
		mode = sm2.SelectMode(bucket, randCfg)
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
		WordID:           word.ID,
		Mode:             mode,
		DueDate:          progress.DueDate,
		IntervalDays:     progress.IntervalDays,
		LearningNewWord:  progress.LearningNewWord,
		SessionExtension: sessionExtension,
		IsAlsoComponent:  isAlsoComponent,
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
		card.ZhText = word.Text
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
	case models.ModeZhToTransl, models.ModeZhToTranslNoSound, models.ModeVoiceToTransl:
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
		models.ModeTranslToZh:        true,
		models.ModeZhToTransl:        true,
		models.ModeZhPinyinToTransl:  true,
		models.ModeZhToTranslNoSound: true,
		models.ModeVoiceToTransl:     true,
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

	userSettings, _ := h.Store.GetUserSettings(r.Context(), UserIDFromContext(r.Context()))

	langs := req.Langs
	if len(langs) == 0 {
		if userSettings != nil {
			langs = []string{userSettings.PrimaryLang}
		} else {
			langs = []string{"en"}
		}
	}
	var correctTexts []string
	switch req.Mode {
	case models.ModeTranslToZh:
		correctTexts = []string{zhWord.ZhText}
	case models.ModeZhToTransl, models.ModeZhPinyinToTransl, models.ModeZhToTranslNoSound, models.ModeVoiceToTransl:
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

	// A word with no pending prev_state has not been answered wrong yet in
	// this encounter, so a correct answer now is "known" — correct on the
	// first try. Checked before UpdateSM2Progress so it reflects the state
	// coming into this answer, not any state this request is about to write.
	prevBeforeAnswer, prevErr := h.Store.GetSM2PrevState(r.Context(), req.WordID)
	if prevErr != nil {
		log.Printf("answer: GetSM2PrevState word %d: %v", req.WordID, prevErr)
	}
	firstTry := prevErr == nil && prevBeforeAnswer == nil

	updated := sm2.ProcessAnswer(*progress, correct)
	if correct && firstTry {
		updated.KnownCorrectCount++
	}
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
	if err := h.Store.RecordAnswerTimestamps(ctx, req.WordID, correct); err != nil {
		log.Printf("answer: RecordAnswerTimestamps word %d: %v", req.WordID, err)
	}
	if correct {
		if err := h.Store.ClearSM2PrevState(ctx, req.WordID); err != nil {
			log.Printf("answer: ClearSM2PrevState word %d: %v", req.WordID, err)
		}
		// The encounter is resolved — any pinned cycle mode (see below) no
		// longer applies; the next question re-derives its position/bucket normally.
		if err := h.Store.ClearCyclePinMode(ctx, req.WordID); err != nil {
			log.Printf("answer: ClearCyclePinMode word %d: %v", req.WordID, err)
		}
		// A correctly-answered word leaves the difficult-words drill.
		if err := h.Store.ClearDrillFlag(ctx, req.WordID); err != nil {
			log.Printf("answer: ClearDrillFlag word %d: %v", req.WordID, err)
		}
	} else {
		if err := h.Store.SaveSM2PrevState(ctx, req.WordID, *progress); err != nil {
			log.Printf("answer: SaveSM2PrevState word %d: %v", req.WordID, err)
		}
		// Under advance-only-if-known/success, a wrong answer doesn't move the
		// cycle position counter, so pin the exact mode just shown — otherwise a
		// tier shift from this wrong answer could re-filter the cycle sequence
		// into a different mode next time (issue #400).
		if userSettings != nil && (userSettings.CycleAdvanceOnKnownOnly || userSettings.CycleAdvanceOnSuccessOnly) {
			if err := h.Store.SaveCyclePinMode(ctx, req.WordID, req.Mode); err != nil {
				log.Printf("answer: SaveCyclePinMode word %d: %v", req.WordID, err)
			}
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

	resp.Tier = sm2.ClassifyTier(updated).String()
	if prevTier != "" && prevTier != resp.Tier {
		resp.PrevTier = prevTier
	}
	if correct && sessionStreak > 1 {
		resp.SessionStreak = sessionStreak
	}

	if !correct {
		userID := UserIDFromContext(r.Context())
		confusedWithID, found, err := h.Store.DetectConfusion(r.Context(), userID, req.WordID, req.Answer, req.Mode, langs)
		if err == nil && found {
			_ = h.Store.UpsertConfusion(r.Context(), userID, req.WordID, confusedWithID, req.Mode)
			confusions, err := h.Store.GetConfusionDetail(r.Context(), userID, req.WordID, confusedWithID, req.Mode, langs)
			if err == nil {
				resp.ConfusedWith = confusions
			}
			if req.Mode == models.ModeTranslToZh {
				if shared, shareErr := h.Store.SharesTranslation(r.Context(), req.WordID, confusedWithID, langs); shareErr == nil && shared {
					resp.Ambiguous = true
				}
			}
		}

		if req.Mode == models.ModeTranslToZh {
			pinyin, _ := h.Store.GetPinyinByZhText(r.Context(), UserIDFromContext(r.Context()), req.Answer)
			resp.UserAnswerPinyin = pinyin
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
		models.ModeTranslToZh:        true,
		models.ModeZhToTransl:        true,
		models.ModeZhPinyinToTransl:  true,
		models.ModeZhToTranslNoSound: true,
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
	_ = h.Store.RecordAnswerTimestamps(ctx, req.WordID, true)
	_ = h.Store.ClearSM2PrevState(ctx, req.WordID)
	// Accepting a typo as correct resolves the encounter, same as a correct answer.
	_ = h.Store.ClearCyclePinMode(ctx, req.WordID)
	// Accepting a typo as correct also retires the word from the difficult drill.
	_ = h.Store.ClearDrillFlag(ctx, req.WordID)

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

// AcknowledgeRandom marks up to n random unseen zh words as due now, skipping
// the one-at-a-time new-word introduction card. Used after onboarding import
// so a brand-new user isn't stuck waiting through the intro flow. The count is
// still capped at the same daily new-word pacing limit that governs
// manually-added words (see issue #344): a bulk import must not flood the
// first session with dozens of never-practiced words at once — the rest stay
// unseen and get introduced gradually like any other new word.
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

	userID := UserIDFromContext(r.Context())
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

	_, _, newToday, err := h.Store.GetStats(r.Context(), userID, nil, "")
	if err != nil {
		internalError(w, err)
		return
	}
	remaining := cap - newToday
	if remaining < 0 {
		remaining = 0
	}
	count := req.Count
	if count > remaining {
		count = remaining
	}
	if count <= 0 {
		writeJSON(w, http.StatusOK, map[string]int{"acknowledged": 0})
		return
	}

	n, err := h.Store.AcknowledgeRandomWords(r.Context(), userID, count)
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

// FlagDifficult handles POST /api/quiz/difficult. It flags the user's hardest
// words (lowest accuracy / lowest ease factor) for a focused drill and returns
// how many were actually flagged.
func (h *QuizHandler) FlagDifficult(w http.ResponseWriter, r *http.Request) {
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
	flagged, err := h.Store.FlagDifficultWords(r.Context(), UserIDFromContext(r.Context()), req.Count)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"flagged": flagged})
}

// ClearDifficult handles POST /api/quiz/difficult/clear. It ends the
// difficult-words drill by clearing all of the user's drill flags.
func (h *QuizHandler) ClearDifficult(w http.ResponseWriter, r *http.Request) {
	if err := h.Store.ClearAllDrillFlags(r.Context(), UserIDFromContext(r.Context())); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"flagged": 0})
}
