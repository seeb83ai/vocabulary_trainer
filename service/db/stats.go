package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"vocabulary_trainer/models"
)

// RecordDailyStat upserts today's daily_stats row after an answer submission.
// It returns the updated session streak (consecutive correct answers today).
func (s *Store) RecordDailyStat(ctx context.Context, userID int64, correct bool) (int, error) {
	var wordsSeen int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sm2_progress p
		 JOIN words w ON w.id = p.word_id
		 WHERE w.language = 'zh' AND p.first_seen_date IS NOT NULL`).Scan(&wordsSeen); err != nil {
		return 0, fmt.Errorf("count words seen: %w", err)
	}

	var bNew, bStruggling, bLearning, bPracticing, bMastered int
	if err := s.db.QueryRowContext(ctx, `
		SELECT
		  COALESCE(SUM(CASE WHEN p.learning_new_word = 1 THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN p.learning_new_word = 0
		    AND (p.total_attempts < 3 OR CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts < 0.50)
		    THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN p.learning_new_word = 0
		    AND p.total_attempts >= 3
		    AND CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts >= 0.50
		    AND NOT (p.total_attempts >= 10 AND CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts >= 0.70)
		    THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN p.learning_new_word = 0
		    AND p.total_attempts >= 10
		    AND CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts >= 0.70
		    AND CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts < 0.85
		    THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN p.learning_new_word = 0
		    AND p.total_attempts >= 10
		    AND CAST(p.total_correct + p.streak_bonus AS REAL) / p.total_attempts >= 0.85
		    THEN 1 ELSE 0 END), 0)
		FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh' AND p.first_seen_date IS NOT NULL`).Scan(
		&bNew, &bStruggling, &bLearning, &bPracticing, &bMastered,
	); err != nil {
		return 0, fmt.Errorf("count buckets: %w", err)
	}

	mistakeInc := 0
	streakInit := 0
	if correct {
		streakInit = 1
	} else {
		mistakeInc = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO daily_stats (user_id, date, attempts, mistakes, words_seen,
			correct_streak, current_streak,
			bucket_new, bucket_struggling, bucket_learning, bucket_practicing, bucket_mastered)
		VALUES (?, date('now'), 1, ?, ?, ?, ?,
			?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET
			attempts         = attempts + 1,
			mistakes         = mistakes + ?,
			words_seen       = ?,
			current_streak   = CASE WHEN ? = 0 THEN current_streak + 1 ELSE 0 END,
			correct_streak   = CASE WHEN ? = 0 THEN MAX(correct_streak, current_streak + 1) ELSE correct_streak END,
			bucket_new       = ?,
			bucket_struggling = ?,
			bucket_learning  = ?,
			bucket_practicing = ?,
			bucket_mastered  = ?`,
		// INSERT values
		userID,
		mistakeInc, wordsSeen, streakInit, streakInit,
		bNew, bStruggling, bLearning, bPracticing, bMastered,
		// UPDATE values
		mistakeInc, wordsSeen, mistakeInc, mistakeInc,
		bNew, bStruggling, bLearning, bPracticing, bMastered,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert daily stat: %w", err)
	}
	var streak int
	_ = s.db.QueryRowContext(ctx, `SELECT current_streak FROM daily_stats WHERE user_id = ? AND date = date('now')`, userID).Scan(&streak)
	return streak, nil
}

// GetDailyStatsHistory returns all daily stats for the given user ordered by date ascending.
func (s *Store) GetDailyStatsHistory(ctx context.Context, userID int64) ([]models.DailyStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT date, attempts, mistakes, words_seen, correct_streak,
		       bucket_new, bucket_struggling, bucket_learning, bucket_practicing, bucket_mastered
		FROM daily_stats WHERE user_id = ? ORDER BY date ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get daily stats: %w", err)
	}
	defer rows.Close()
	var stats []models.DailyStat
	for rows.Next() {
		var d models.DailyStat
		if err := rows.Scan(&d.Date, &d.Attempts, &d.Mistakes, &d.WordsSeen, &d.CorrectStreak,
			&d.BucketNew, &d.BucketStruggling, &d.BucketLearning, &d.BucketPracticing, &d.BucketMastered); err != nil {
			return nil, fmt.Errorf("scan daily stat: %w", err)
		}
		stats = append(stats, d)
	}
	if stats == nil {
		stats = []models.DailyStat{}
	}
	return stats, rows.Err()
}

// GetWordStats returns aggregate statistics for all words seen at least once.
func (s *Store) GetWordStats(ctx context.Context, userID int64) (*models.WordStatsResponse, error) {
	// Fetch per-word stats for all seen zh words in a single query.
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, w.text, w.pinyin,
		       p.total_correct, p.total_attempts, p.streak_bonus, p.easiness,
		       p.learning_new_word
		FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh' AND w.user_id = ? AND p.first_seen_date IS NOT NULL
		ORDER BY p.total_attempts DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get word stats: %w", err)
	}
	defer rows.Close()

	type row struct {
		id          int64
		text        string
		pinyin      *string
		correct     int
		attempts    int
		streakBonus int
		easiness    float64
		accuracy    float64
		learning    bool
	}
	var all []row
	for rows.Next() {
		var r row
		var learning int
		if err := rows.Scan(&r.id, &r.text, &r.pinyin, &r.correct, &r.attempts, &r.streakBonus, &r.easiness, &learning); err != nil {
			return nil, fmt.Errorf("scan word stat: %w", err)
		}
		r.learning = learning == 1
		if r.attempts > 0 {
			r.accuracy = float64(r.correct+r.streakBonus) / float64(r.attempts) * 100
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	resp := &models.WordStatsResponse{
		TotalSeen:  len(all),
		AccBuckets: map[string]int{"new": 0, "0-49": 0, "50-69": 0, "70-84": 0, "85-100": 0},
		Hardest:    []models.WordStatDetail{},
		MostPract:  []models.WordStatDetail{},
	}

	if len(all) == 0 {
		return resp, nil
	}

	for _, r := range all {
		if r.learning {
			resp.AccBuckets["new"]++
		} else if r.attempts > 0 {
			switch {
			case r.attempts >= 10 && r.accuracy >= 85:
				resp.AccBuckets["85-100"]++
			case r.attempts >= 10 && r.accuracy >= 70:
				resp.AccBuckets["70-84"]++
			case r.attempts >= 3 && r.accuracy >= 50:
				resp.AccBuckets["50-69"]++
			default:
				resp.AccBuckets["0-49"]++
			}
		}
	}

	// Hardest words: lowest accuracy, min 3 attempts, up to 20
	type scored struct {
		idx int
		acc float64
	}
	var candidates []scored
	for i, r := range all {
		if r.attempts >= 3 {
			candidates = append(candidates, scored{i, r.accuracy})
		}
	}
	// Sort by accuracy ascending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].acc < candidates[i].acc {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	limit := 20
	if limit > len(candidates) {
		limit = len(candidates)
	}

	// Collect IDs for batch en-text loading
	detailIDs := make([]int64, 0, limit*2)
	for k := 0; k < limit; k++ {
		r := all[candidates[k].idx]
		resp.Hardest = append(resp.Hardest, models.WordStatDetail{
			WordID: r.id, ZhText: r.text, Pinyin: r.pinyin,
			Correct: r.correct, Attempts: r.attempts, StreakBonus: r.streakBonus,
			Accuracy: r.accuracy, Easiness: r.easiness,
		})
		detailIDs = append(detailIDs, r.id)
	}

	// Most practiced: already sorted by total_attempts DESC from query
	mpLimit := 20
	if mpLimit > len(all) {
		mpLimit = len(all)
	}
	for k := 0; k < mpLimit; k++ {
		r := all[k]
		resp.MostPract = append(resp.MostPract, models.WordStatDetail{
			WordID: r.id, ZhText: r.text, Pinyin: r.pinyin,
			Correct: r.correct, Attempts: r.attempts, StreakBonus: r.streakBonus,
			Accuracy: r.accuracy, Easiness: r.easiness,
		})
		detailIDs = append(detailIDs, r.id)
	}

	// Batch-load en texts for all detail words
	if len(detailIDs) > 0 {
		placeholders := make([]string, len(detailIDs))
		args := make([]any, len(detailIDs))
		for i, id := range detailIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		enRows, err := s.db.QueryContext(ctx,
			`SELECT t.zh_word_id, ew.language, ew.text FROM words ew
			 JOIN translations t ON t.translation_word_id = ew.id
			 WHERE t.zh_word_id IN (`+strings.Join(placeholders, ",")+`)
			 ORDER BY ew.text`, args...)
		if err != nil {
			return nil, fmt.Errorf("batch en texts for word stats: %w", err)
		}
		defer enRows.Close()
		type langText struct{ lang, text string }
		transMap := map[int64][]langText{}
		for enRows.Next() {
			var zhID int64
			var lang, text string
			if err := enRows.Scan(&zhID, &lang, &text); err != nil {
				return nil, err
			}
			transMap[zhID] = append(transMap[zhID], langText{lang, text})
		}
		if err := enRows.Err(); err != nil {
			return nil, err
		}
		for i := range resp.Hardest {
			resp.Hardest[i].Translations = map[string][]string{}
			for _, lt := range transMap[resp.Hardest[i].WordID] {
				resp.Hardest[i].Translations[lt.lang] = append(resp.Hardest[i].Translations[lt.lang], lt.text)
			}
		}
		for i := range resp.MostPract {
			resp.MostPract[i].Translations = map[string][]string{}
			for _, lt := range transMap[resp.MostPract[i].WordID] {
				resp.MostPract[i].Translations[lt.lang] = append(resp.MostPract[i].Translations[lt.lang], lt.text)
			}
		}
	}

	return resp, nil
}

// GetTodaySessionInfo returns today's attempt and mistake counts from daily_stats,
// and the number of seen zh words whose due date is still in the future.
func (s *Store) GetTodaySessionInfo(ctx context.Context, userID int64) (attempts, mistakes, availableToAdvance int, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(attempts, 0), COALESCE(mistakes, 0) FROM daily_stats WHERE user_id = ? AND date = date('now')`, userID).
		Scan(&attempts, &mistakes)
	if err == sql.ErrNoRows {
		err = nil
		attempts, mistakes = 0, 0
	}
	if err != nil {
		err = fmt.Errorf("get today session info: %w", err)
		return
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh' AND w.user_id = ?
		  AND p.first_seen_date IS NOT NULL
		  AND p.due_date > CURRENT_TIMESTAMP`, userID).Scan(&availableToAdvance)
	if err != nil {
		err = fmt.Errorf("count available to advance: %w", err)
	}
	return
}

// AdvanceDueDates pulls forward the due dates of seen zh words so that at least
// n words become due now. It finds the Nth earliest future due date among seen
// zh words, computes the delta to now, and subtracts it from all seen zh words'
// due dates. Returns the number of zh words now due after the operation.
func (s *Store) AdvanceDueDates(ctx context.Context, userID int64, n int) (int, error) {
	var nthDueDateStr string
	err := s.db.QueryRowContext(ctx, `
		SELECT p.due_date FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh' AND w.user_id = ?
		  AND p.first_seen_date IS NOT NULL
		  AND p.due_date > CURRENT_TIMESTAMP
		ORDER BY p.due_date ASC
		LIMIT 1 OFFSET ?`, userID, n-1).Scan(&nthDueDateStr)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find nth due date: %w", err)
	}

	nthDueDate := parseDateTime(nthDueDateStr)
	delta := time.Until(nthDueDate)
	if delta <= 0 {
		return 0, nil
	}
	modifier := fmt.Sprintf("-%d seconds", int64(delta.Seconds())+1)

	if _, err := s.db.ExecContext(ctx, `
		UPDATE sm2_progress SET due_date = datetime(due_date, ?)
		WHERE first_seen_date IS NOT NULL
		  AND word_id IN (SELECT id FROM words WHERE language = 'zh' AND user_id = ?)`,
		modifier, userID); err != nil {
		return 0, fmt.Errorf("advance due dates: %w", err)
	}

	var nowDue int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		WHERE w.language = 'zh' AND w.user_id = ?
		  AND p.first_seen_date IS NOT NULL
		  AND p.due_date <= CURRENT_TIMESTAMP`, userID).Scan(&nowDue); err != nil {
		return 0, fmt.Errorf("count now due: %w", err)
	}
	return nowDue, nil
}
