package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

type QuizHandler struct {
	Store *db.Store
}

// Next returns the next card to study.
func (h *QuizHandler) Next(w http.ResponseWriter, r *http.Request) {
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	word, progress, err := h.Store.GetNextCard(r.Context(), tags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if word == nil {
		writeError(w, http.StatusNotFound, "no words available")
		return
	}

	var mode string
	switch r.URL.Query().Get("mode") {
	case models.ModeEnToZh, models.ModeZhToEn, models.ModeZhPinyinToEn:
		mode = r.URL.Query().Get("mode")
	default:
		mode = sm2.SelectMode()
	}

	// zh_pinyin_to_en requires pinyin; fall back if missing
	if mode == models.ModeZhPinyinToEn && (word.Pinyin == nil || *word.Pinyin == "") {
		mode = models.ModeZhToEn
	}

	// For en_to_zh we need an EN word — but GetNextCard may return a zh word.
	// Strategy: GetNextCard returns the zh word always (it's the canonical unit).
	// If mode is en_to_zh, we pick one of the linked EN words as the prompt.
	// The word_id we return is always the zh word ID regardless of mode,
	// because SM-2 progress is tracked on the zh word.
	// For the answer, the handler checks zh translations when mode is en_to_zh.

	card := models.QuizCard{
		WordID:       word.ID,
		Mode:         mode,
		DueDate:      progress.DueDate,
		IntervalDays: progress.IntervalDays,
	}

	switch mode {
	case models.ModeEnToZh:
		// Prompt is one of the English words; pick the first linked EN word
		enWords, err := h.Store.GetTranslationsForWord(r.Context(), word.ID, "en")
		if err != nil || len(enWords) == 0 {
			// No EN translations — fall back to zh_to_en
			card.Mode = models.ModeZhToEn
			card.Prompt = word.Text
		} else {
			card.Prompt = enWords[0].Text
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

// Stats returns due-today and total card counts.
func (h *QuizHandler) Stats(w http.ResponseWriter, r *http.Request) {
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	due, total, err := h.Store.GetStats(r.Context(), tags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"due_today": due, "total": total})
}
