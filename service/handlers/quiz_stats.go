package handlers

import (
	"net/http"
	"strings"
	"time"
	"vocabulary_trainer/models"
)

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

// WordStats returns aggregate per-word statistics for all seen words. The
// optional "tags" query param (comma-separated) restricts the accuracy-bucket
// breakdown to words carrying at least one of the given tags; other sections
// (hardest, most-practiced) still cover all of the user's words.
func (h *QuizHandler) WordStats(w http.ResponseWriter, r *http.Request) {
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	stats, err := h.Store.GetWordStats(r.Context(), UserIDFromContext(r.Context()), tags)
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
	primaryLang := "en"
	if userSettings != nil {
		if h.MaxNewPerDay > 0 && userSettings.MaxNewWordsPerDay >= 1 {
			maxNew = userSettings.MaxNewWordsPerDay
		}
		primaryLang = userSettings.PrimaryLang
	}
	langs := parseLangs(r, primaryLang)
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
		compDueToday, compTotal, cErr = h.Store.GetComponentCounts(r.Context(), UserIDFromContext(r.Context()), langs)
		if cErr != nil {
			writeError(w, http.StatusInternalServerError, cErr.Error())
			return
		}
	}
	difficultRemaining, err := h.Store.CountDrillFlags(r.Context(), userID)
	if err != nil {
		internalError(w, err)
		return
	}
	wordsImproved, err := h.Store.GetWordsImprovedToday(r.Context(), userID)
	if err != nil {
		internalError(w, err)
		return
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
		"difficult_remaining":  difficultRemaining,
		"words_improved_today": wordsImproved,
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
