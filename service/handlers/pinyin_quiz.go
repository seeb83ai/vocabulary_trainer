package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"

	"github.com/go-chi/chi/v5"
)

type PinyinQuizHandler struct {
	Store           *db.Store
	PinyinAudioDirs []string
}

// pinyinTier returns the accuracy bucket label for a pinyin progress record.
func pinyinTier(p models.SM2Progress) string {
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

func (h *PinyinQuizHandler) Next(w http.ResponseWriter, r *http.Request) {
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	skipNew := r.URL.Query().Get("skip_new") == "true"

	sound, progress, err := h.Store.GetNextPinyinCard(r.Context(), tags, skipNew)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sound == nil {
		writeError(w, http.StatusNotFound, "no pinyin sounds available")
		return
	}

	// First time seeing this sound: mark as seen
	if progress.TotalAttempts == 0 {
		_ = h.Store.AcknowledgePinyinSound(r.Context(), sound.ID)
	}

	// Determine mode: learning phase = multiple choice, review = type answer
	mode := models.PinyinModeTypeAnswer
	if progress.LearningNewWord {
		mode = models.PinyinModeMultipleChoice
	}

	card := models.PinyinCard{
		SoundID:      sound.ID,
		Mode:         mode,
		AudioFile:    sound.Filename,
		DueDate:      progress.DueDate,
		IntervalDays: progress.IntervalDays,
		Learning:     progress.LearningNewWord,
	}

	if mode == models.PinyinModeMultipleChoice {
		distractors, err := h.Store.GetPinyinDistractors(r.Context(), *sound, 3)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		options := []models.PinyinOption{
			{
				SoundID:  sound.ID,
				Label:    sm2.FormatPinyinDisplay(sound.Syllable, sound.Tone),
				Syllable: sound.Syllable,
				Tone:     sound.Tone,
			},
		}
		for _, d := range distractors {
			options = append(options, models.PinyinOption{
				SoundID:  d.ID,
				Label:    sm2.FormatPinyinDisplay(d.Syllable, d.Tone),
				Syllable: d.Syllable,
				Tone:     d.Tone,
			})
		}
		sort.Slice(options, func(i, j int) bool {
			if options[i].Syllable != options[j].Syllable {
				return options[i].Syllable < options[j].Syllable
			}
			return options[i].Tone < options[j].Tone
		})
		card.Options = options
	}

	writeJSON(w, http.StatusOK, card)
}

func (h *PinyinQuizHandler) Answer(w http.ResponseWriter, r *http.Request) {
	var req models.PinyinAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Answer = strings.TrimSpace(req.Answer)
	if req.SoundID <= 0 {
		writeError(w, http.StatusBadRequest, "sound_id is required")
		return
	}
	if req.Mode != models.PinyinModeMultipleChoice && req.Mode != models.PinyinModeTypeAnswer {
		writeError(w, http.StatusBadRequest, "invalid mode")
		return
	}

	sound, err := h.Store.GetPinyinSoundByID(r.Context(), req.SoundID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sound == nil {
		writeError(w, http.StatusNotFound, "sound not found")
		return
	}

	progress, err := h.Store.GetPinyinProgress(r.Context(), req.SoundID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if progress == nil {
		writeError(w, http.StatusNotFound, "progress not found")
		return
	}
	prevTier := pinyinTier(*progress)

	// Determine correctness
	var correct bool
	var confusedWithID int64
	switch req.Mode {
	case models.PinyinModeMultipleChoice:
		// Answer is the chosen sound_id as string
		var chosenID int64
		if _, err := json.Number(req.Answer).Int64(); err == nil {
			chosenID, _ = json.Number(req.Answer).Int64()
		}
		correct = chosenID == sound.ID
		if !correct && chosenID > 0 {
			confusedWithID = chosenID
		}
	case models.PinyinModeTypeAnswer:
		correct = sm2.CheckPinyinAnswer(req.Answer, sound.Syllable, sound.Tone)
		if !correct {
			// Try to find what they typed as a different sound
			syllable, tone, parseErr := sm2.ParsePinyinAnswer(req.Answer)
			if parseErr == nil {
				confused, _ := h.Store.GetPinyinSoundBySyllableTone(r.Context(), syllable, tone)
				if confused != nil && confused.ID != sound.ID {
					confusedWithID = confused.ID
				}
			}
		}
	}

	quality := sm2.QualityWrong
	if correct {
		quality = sm2.QualityCorrect
	}

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

	if err := h.Store.UpdatePinyinProgress(r.Context(), updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "sound not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.Store.RecordPinyinDailyStat(r.Context(), correct, sound.Tone)

	resp := models.PinyinAnswerResponse{
		Correct:       correct,
		CorrectAnswer: sm2.FormatPinyinDisplay(sound.Syllable, sound.Tone),
		NextDue:       updated.DueDate,
		IntervalDays:  updated.IntervalDays,
		TotalCorrect:  updated.TotalCorrect,
		TotalAttempts: updated.TotalAttempts,
		StreakBonus:   updated.StreakBonus,
		Repetitions:   updated.Repetitions,
		GraduateReps:  sm2.LearningGraduateReps,
		Learning:      updated.LearningNewWord,
		Graduated:     graduated,
	}

	if !correct && req.Mode == models.PinyinModeTypeAnswer {
		resp.YourAnswer = req.Answer
	}

	if correct {
		resp.Tier = pinyinTier(updated)
		if prevTier != "" && prevTier != resp.Tier {
			resp.PrevTier = prevTier
		}
	}

	// Include all tone variants for the syllable so the user can compare
	variants, _ := h.Store.GetPinyinToneVariants(r.Context(), sound.Syllable)
	for _, v := range variants {
		resp.ToneVariants = append(resp.ToneVariants, models.PinyinToneVariant{
			Label:    sm2.FormatPinyinDisplay(v.Syllable, v.Tone),
			Filename: v.Filename,
			Tone:     v.Tone,
			Current:  v.ID == sound.ID,
		})
	}

	if !correct && confusedWithID > 0 {
		_ = h.Store.UpsertPinyinConfusion(r.Context(), sound.ID, confusedWithID)
		detail, _ := h.Store.GetPinyinConfusionDetail(r.Context(), sound.ID, confusedWithID)
		if detail != nil {
			resp.ConfusedWith = detail
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *PinyinQuizHandler) Stats(w http.ResponseWriter, r *http.Request) {
	var tags []string
	if t := r.URL.Query().Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}
	due, total, err := h.Store.GetPinyinStats(r.Context(), tags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"due_today": due,
		"total":     total,
	})
}

func (h *PinyinQuizHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.Store.ListPinyinTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tags == nil {
		tags = []string{}
	}
	writeJSON(w, http.StatusOK, tags)
}

func (h *PinyinQuizHandler) DailyStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Store.GetPinyinDailyStatsHistory(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := models.PinyinDailyStatsResponse{Days: make([]models.PinyinDailyStatEntry, len(stats))}
	for i, s := range stats {
		resp.Days[i] = models.PinyinDailyStatEntry{
			Date:         s.Date,
			Attempts:     s.Attempts,
			Mistakes:     s.Mistakes,
			SoundsSeen:   s.SoundsSeen,
			Tone1Correct: s.Tone1Correct,
			Tone1Wrong:   s.Tone1Wrong,
			Tone2Correct: s.Tone2Correct,
			Tone2Wrong:   s.Tone2Wrong,
			Tone3Correct: s.Tone3Correct,
			Tone3Wrong:   s.Tone3Wrong,
			Tone4Correct: s.Tone4Correct,
			Tone4Wrong:   s.Tone4Wrong,
			Tone5Correct: s.Tone5Correct,
			Tone5Wrong:   s.Tone5Wrong,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *PinyinQuizHandler) ServeAudio(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	// Shuffle dirs so each request picks a random voice when multiple are configured.
	dirs := make([]string, len(h.PinyinAudioDirs))
	copy(dirs, h.PinyinAudioDirs)
	rand.Shuffle(len(dirs), func(i, j int) { dirs[i], dirs[j] = dirs[j], dirs[i] })

	for _, dir := range dirs {
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); err == nil {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, path)

			log.Printf("play: %s", path)
			return
		}
	}
	writeError(w, http.StatusNotFound, "audio not found")
}
