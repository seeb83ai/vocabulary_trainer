package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

type QuizHandler struct {
	Store        *db.Store
	MaxNewPerDay int
	capResetDate string // date string (YYYY-MM-DD) on which the new-word cap was reset
	newCapBase   int    // newToday count at cap-reset time; cap = newCapBase + MaxNewPerDay
}

// Next returns the next card to study.
func (h *QuizHandler) Next(w http.ResponseWriter, r *http.Request) {
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	bucket := r.URL.Query().Get("bucket")
	cap := h.MaxNewPerDay
	if h.capResetDate == time.Now().Format("2006-01-02") {
		extra := h.MaxNewPerDay
		if extra < 1 {
			extra = 1
		}
		cap = h.newCapBase + extra
	}
	word, progress, err := h.Store.GetNextCard(r.Context(), tags, cap, bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if word == nil {
		writeError(w, http.StatusNotFound, "no words available")
		return
	}

	requestedMode := r.URL.Query().Get("mode")

	// Progressive mode: new words (total_attempts==0) are shown as introductions
	if progress.TotalAttempts == 0 {
		enWords, err := h.Store.GetTranslationsForWord(r.Context(), word.ID, "en")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		enTexts := make([]string, len(enWords))
		for i, ew := range enWords {
			enTexts[i] = ew.Text
		}
		card := models.QuizCard{
			WordID:       word.ID,
			Mode:         models.ModeNewWord,
			Prompt:       word.Text,
			Pinyin:       word.Pinyin,
			EnTexts:      enTexts,
			DueDate:      progress.DueDate,
			IntervalDays: progress.IntervalDays,
		}
		writeJSON(w, http.StatusOK, card)
		return
	}

	var mode string
	switch requestedMode {
	case models.ModeEnToZh, models.ModeZhToEn, models.ModeZhPinyinToEn:
		mode = requestedMode
	case models.ModeProgressive:
		mode = sm2.SelectProgressiveMode(progress.TotalCorrect, progress.TotalAttempts)
	default:
		mode = sm2.SelectMode()
	}

	// zh_pinyin_to_en requires pinyin; fall back if missing
	if mode == models.ModeZhPinyinToEn && (word.Pinyin == nil || *word.Pinyin == "") {
		mode = models.ModeZhToEn
	}

	card := models.QuizCard{
		WordID:       word.ID,
		Mode:         mode,
		DueDate:      progress.DueDate,
		IntervalDays: progress.IntervalDays,
	}

	switch mode {
	case models.ModeEnToZh:
		enWords, err := h.Store.GetTranslationsForWord(r.Context(), word.ID, "en")
		if err != nil || len(enWords) == 0 {
			card.Mode = models.ModeZhToEn
			card.Prompt = word.Text
		} else {
			card.Prompt = enWords[0].Text
			for _, ew := range enWords {
				card.EnTexts = append(card.EnTexts, ew.Text)
			}
		}
	case models.ModeZhToEn:
		card.Prompt = word.Text
	case models.ModeZhPinyinToEn:
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
		models.ModeEnToZh:       true,
		models.ModeZhToEn:       true,
		models.ModeZhPinyinToEn: true,
	}
	if !validModes[req.Mode] {
		writeError(w, http.StatusBadRequest, "invalid mode")
		return
	}

	// Look up the zh word (word_id is always the zh word)
	zhWord, err := h.Store.GetWordByID(r.Context(), req.WordID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if zhWord == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}

	var correctTexts []string
	switch req.Mode {
	case models.ModeEnToZh:
		correctTexts = []string{zhWord.ZhText}
	case models.ModeZhToEn, models.ModeZhPinyinToEn:
		enWords, err := h.Store.GetTranslationsForWord(r.Context(), req.WordID, "en")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, ew := range enWords {
			correctTexts = append(correctTexts, ew.Text)
		}
	}

	correct := sm2.CheckAnswer(req.Answer, correctTexts)
	quality := sm2.QualityWrong
	if correct {
		quality = sm2.QualityCorrect
	}

	progress, err := h.Store.GetSM2Progress(r.Context(), req.WordID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if progress == nil {
		writeError(w, http.StatusNotFound, "progress not found")
		return
	}

	updated := sm2.Update(*progress, quality)
	updated.TotalAttempts++
	if correct {
		updated.TotalCorrect++
	}

	if err := h.Store.UpdateSM2Progress(r.Context(), updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_ = h.Store.RecordDailyStat(r.Context(), correct)

	resp := models.AnswerResponse{
		Correct:        correct,
		CorrectAnswers: correctTexts,
		ZhText:         zhWord.ZhText,
		Pinyin:         zhWord.Pinyin,
		EnTexts:        zhWord.EnTexts,
		NextDue:        updated.DueDate,
		IntervalDays:   updated.IntervalDays,
		TotalCorrect:   updated.TotalCorrect,
		TotalAttempts:  updated.TotalAttempts,
	}

	if !correct {
		confusedWithID, found, err := h.Store.LookupConfusion(r.Context(), req.WordID, req.Answer, req.Mode)
		if err == nil && found {
			_ = h.Store.UpsertConfusion(r.Context(), req.WordID, confusedWithID, req.Mode)
			confusions, err := h.Store.GetConfusionDetail(r.Context(), req.WordID, confusedWithID, req.Mode)
			if err == nil {
				resp.ConfusedWith = confusions
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// DailyStats returns the full daily stats history.
func (h *QuizHandler) DailyStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Store.GetDailyStatsHistory(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := models.DailyStatsResponse{Days: make([]models.DailyStatEntry, len(stats))}
	for i, s := range stats {
		resp.Days[i] = models.DailyStatEntry{
			Date:          s.Date,
			Attempts:      s.Attempts,
			Mistakes:      s.Mistakes,
			WordsKnown:    s.WordsKnown,
			NewWords:      s.NewWords,
			WordsSeen:     s.WordsSeen,
			CorrectStreak: s.CorrectStreak,
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
	if err := h.Store.SkipWord(r.Context(), req.WordID, 7); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
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
	if err := h.Store.AcknowledgeWord(r.Context(), req.WordID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "word not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// WordStats returns aggregate per-word statistics for all seen words.
func (h *QuizHandler) WordStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Store.GetWordStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	due, total, newToday, err := h.Store.GetStats(r.Context(), tags, bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	todayAttempts, todayMistakes, availableToAdvance, err := h.Store.GetTodaySessionInfo(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cap := h.MaxNewPerDay
	if h.capResetDate == time.Now().Format("2006-01-02") {
		extra := h.MaxNewPerDay
		if extra < 1 {
			extra = 1
		}
		cap = h.newCapBase + extra
	}
	newAvailable := 0
	// When drilling a specific tier, don't introduce new words (they have no tier yet).
	if bucket == "" && newToday < cap {
		n, err := h.Store.CountUnseenZhWords(r.Context(), tags)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if remaining := cap - newToday; n > remaining {
			n = remaining
		}
		newAvailable = n
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"due_today":            due,
		"total":                total,
		"new_today":            newToday,
		"max_new_per_day":      h.MaxNewPerDay,
		"today_attempts":       todayAttempts,
		"today_mistakes":       todayMistakes,
		"available_to_advance": availableToAdvance,
		"new_available":        newAvailable,
	})
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
		n, err := h.Store.AdvanceDueDates(r.Context(), req.Count)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		advanced = n
	}
	if req.ResetNewCap {
		_, _, newToday, err := h.Store.GetStats(r.Context(), nil, "")
		if err == nil {
			h.capResetDate = time.Now().Format("2006-01-02")
			h.newCapBase = newToday
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"advanced":  advanced,
		"cap_reset": req.ResetNewCap,
	})
}
