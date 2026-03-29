package db

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"time"
	"vocabulary_trainer/models"
)

// InsertPinyinSound inserts a pinyin sound and initialises its progress row.
func (s *Store) InsertPinyinSound(ctx context.Context, sound models.PinyinSound) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO pinyin_sounds (initial, final, tone, syllable, filename, tag)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sound.Initial, sound.Final, sound.Tone, sound.Syllable, sound.Filename, sound.Tag)
	if err != nil {
		return 0, fmt.Errorf("insert pinyin sound: %w", err)
	}
	affected, _ := res.RowsAffected()
	var id int64
	if affected > 0 {
		id, _ = res.LastInsertId()
	} else {
		// Already existed; look up the ID by syllable+tone (filename may differ
		// for duplicates like de.mp3 vs de5.mp3).
		if err := s.db.QueryRowContext(ctx,
			`SELECT id FROM pinyin_sounds WHERE syllable = ? AND tone = ?`,
			sound.Syllable, sound.Tone).Scan(&id); err != nil {
			return 0, fmt.Errorf("get pinyin sound id: %w", err)
		}
	}
	// Assign a random past due_date so that when many sounds are imported at
	// once they don't all share the same timestamp and end up in insertion
	// (alphabetical) order. Spread over the last hour.
	dueDate := time.Now().UTC().Add(-time.Duration(rand.Intn(3600)) * time.Second).Format("2006-01-02 15:04:05")
	_, err = s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO pinyin_progress (sound_id, due_date) VALUES (?, ?)`, id, dueDate)
	if err != nil {
		return 0, fmt.Errorf("init pinyin progress: %w", err)
	}
	return id, nil
}

// GetPinyinSoundByID returns a single pinyin sound by ID.
func (s *Store) GetPinyinSoundByID(ctx context.Context, id int64) (*models.PinyinSound, error) {
	var ps models.PinyinSound
	err := s.db.QueryRowContext(ctx,
		`SELECT id, initial, final, tone, syllable, filename, tag
		 FROM pinyin_sounds WHERE id = ?`, id).
		Scan(&ps.ID, &ps.Initial, &ps.Final, &ps.Tone, &ps.Syllable, &ps.Filename, &ps.Tag)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get pinyin sound: %w", err)
	}
	return &ps, nil
}

// GetPinyinSoundBySyllableTone finds a sound by syllable+tone.
func (s *Store) GetPinyinSoundBySyllableTone(ctx context.Context, syllable string, tone int) (*models.PinyinSound, error) {
	var ps models.PinyinSound
	err := s.db.QueryRowContext(ctx,
		`SELECT id, initial, final, tone, syllable, filename, tag
		 FROM pinyin_sounds WHERE syllable = ? AND tone = ?`, syllable, tone).
		Scan(&ps.ID, &ps.Initial, &ps.Final, &ps.Tone, &ps.Syllable, &ps.Filename, &ps.Tag)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get pinyin sound by syllable+tone: %w", err)
	}
	return &ps, nil
}

// GetPinyinToneVariants returns all tone variants for a given syllable, ordered by tone.
func (s *Store) GetPinyinToneVariants(ctx context.Context, syllable string) ([]models.PinyinSound, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, initial, final, tone, syllable, filename, tag
		 FROM pinyin_sounds WHERE syllable = ? ORDER BY tone`, syllable)
	if err != nil {
		return nil, fmt.Errorf("get pinyin tone variants: %w", err)
	}
	defer rows.Close()
	var results []models.PinyinSound
	for rows.Next() {
		var ps models.PinyinSound
		if err := rows.Scan(&ps.ID, &ps.Initial, &ps.Final, &ps.Tone, &ps.Syllable, &ps.Filename, &ps.Tag); err != nil {
			return nil, fmt.Errorf("scan pinyin tone variant: %w", err)
		}
		results = append(results, ps)
	}
	return results, rows.Err()
}

// GetNextPinyinCard returns the next pinyin sound to study using the same
// 3-tier priority as GetNextCard: overdue → non-retry-window → retry-window.
func (s *Store) GetNextPinyinCard(ctx context.Context, tags []string, skipNew bool) (*models.PinyinSound, *models.SM2Progress, error) {
	tagFilter := ""
	var tagArgs []any
	if len(tags) > 0 {
		placeholders := make([]string, len(tags))
		for i, t := range tags {
			placeholders[i] = "?"
			tagArgs = append(tagArgs, t)
		}
		tagFilter = ` AND s.tag IN (` + strings.Join(placeholders, ",") + `)`
	}

	newFilter := ""
	if skipNew {
		newFilter = " AND p.first_seen_date IS NOT NULL"
	}

	query := `
		SELECT s.id, s.initial, s.final, s.tone, s.syllable, s.filename, s.tag,
		       p.sound_id, p.repetitions, p.easiness, p.interval_days, p.due_date,
		       p.total_correct, p.total_attempts, p.streak_bonus, p.learning
		FROM pinyin_sounds s
		JOIN pinyin_progress p ON p.sound_id = s.id
		WHERE 1=1` + tagFilter + newFilter + ` %s
		ORDER BY p.due_date ASC
		LIMIT 1`

	tryQuery := func(extra string) (*models.PinyinSound, *models.SM2Progress, error) {
		args := append([]any{}, tagArgs...)
		row := s.db.QueryRowContext(ctx, fmt.Sprintf(query, extra), args...)
		var ps models.PinyinSound
		var prog models.SM2Progress
		var dueDate string
		var learning int
		err := row.Scan(
			&ps.ID, &ps.Initial, &ps.Final, &ps.Tone, &ps.Syllable, &ps.Filename, &ps.Tag,
			&prog.WordID, &prog.Repetitions, &prog.Easiness, &prog.IntervalDays, &dueDate,
			&prog.TotalCorrect, &prog.TotalAttempts, &prog.StreakBonus, &learning,
		)
		prog.DueDate = parseDateTime(dueDate)
		prog.LearningNewWord = learning == 1
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get next pinyin card: %w", err)
		}
		return &ps, &prog, nil
	}

	todayBound := "AND p.due_date < date('now', '+1 day')"

	ps, prog, err := tryQuery("AND p.due_date <= CURRENT_TIMESTAMP")
	if err != nil || ps != nil {
		return ps, prog, err
	}
	ps, prog, err = tryQuery(fmt.Sprintf("AND p.due_date > datetime('now', '+%d seconds') %s", 180, todayBound))
	if err != nil || ps != nil {
		return ps, prog, err
	}
	return tryQuery(todayBound)
}

// GetPinyinDistractors returns up to count wrong options for a given sound.
// Strategy: mix of same-syllable-different-tone and same-tone-different-syllable.
func (s *Store) GetPinyinDistractors(ctx context.Context, target models.PinyinSound, count int) ([]models.PinyinSound, error) {
	var distractors []models.PinyinSound

	// First: same syllable, different tone (best for tone discrimination)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, initial, final, tone, syllable, filename, tag
		 FROM pinyin_sounds
		 WHERE syllable = ? AND tone != ?
		 ORDER BY RANDOM()
		 LIMIT ?`, target.Syllable, target.Tone, count)
	if err != nil {
		return nil, fmt.Errorf("get same-syllable distractors: %w", err)
	}
	for rows.Next() {
		var ps models.PinyinSound
		if err := rows.Scan(&ps.ID, &ps.Initial, &ps.Final, &ps.Tone, &ps.Syllable, &ps.Filename, &ps.Tag); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan distractor: %w", err)
		}
		distractors = append(distractors, ps)
	}
	rows.Close()

	if len(distractors) >= count {
		return distractors[:count], nil
	}

	// Fill remaining with same-tone, different syllable
	remaining := count - len(distractors)
	excludeIDs := []any{target.ID}
	excludePlaceholders := []string{"?"}
	for _, d := range distractors {
		excludeIDs = append(excludeIDs, d.ID)
		excludePlaceholders = append(excludePlaceholders, "?")
	}
	args := append([]any{target.Tone}, excludeIDs...)
	args = append(args, remaining)
	rows, err = s.db.QueryContext(ctx,
		`SELECT id, initial, final, tone, syllable, filename, tag
		 FROM pinyin_sounds
		 WHERE tone = ? AND id NOT IN (`+strings.Join(excludePlaceholders, ",")+`)
		 ORDER BY RANDOM()
		 LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("get same-tone distractors: %w", err)
	}
	for rows.Next() {
		var ps models.PinyinSound
		if err := rows.Scan(&ps.ID, &ps.Initial, &ps.Final, &ps.Tone, &ps.Syllable, &ps.Filename, &ps.Tag); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan distractor: %w", err)
		}
		distractors = append(distractors, ps)
	}
	rows.Close()

	if len(distractors) >= count {
		return distractors[:count], nil
	}

	// Last resort: any other sound
	remaining = count - len(distractors)
	excludeIDs = []any{target.ID}
	excludePlaceholders = []string{"?"}
	for _, d := range distractors {
		excludeIDs = append(excludeIDs, d.ID)
		excludePlaceholders = append(excludePlaceholders, "?")
	}
	args2 := append(excludeIDs, remaining)
	rows, err = s.db.QueryContext(ctx,
		`SELECT id, initial, final, tone, syllable, filename, tag
		 FROM pinyin_sounds
		 WHERE id NOT IN (`+strings.Join(excludePlaceholders, ",")+`)
		 ORDER BY RANDOM()
		 LIMIT ?`, args2...)
	if err != nil {
		return nil, fmt.Errorf("get fallback distractors: %w", err)
	}
	for rows.Next() {
		var ps models.PinyinSound
		if err := rows.Scan(&ps.ID, &ps.Initial, &ps.Final, &ps.Tone, &ps.Syllable, &ps.Filename, &ps.Tag); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan distractor: %w", err)
		}
		distractors = append(distractors, ps)
	}
	rows.Close()

	return distractors, nil
}

// GetPinyinProgress returns the SM2 progress for a pinyin sound.
func (s *Store) GetPinyinProgress(ctx context.Context, soundID int64) (*models.SM2Progress, error) {
	var p models.SM2Progress
	var dueDate string
	var learning int
	err := s.db.QueryRowContext(ctx,
		`SELECT sound_id, repetitions, easiness, interval_days, due_date,
		        total_correct, total_attempts, streak_bonus, learning
		 FROM pinyin_progress WHERE sound_id = ?`, soundID).
		Scan(&p.WordID, &p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDate,
			&p.TotalCorrect, &p.TotalAttempts, &p.StreakBonus, &learning)
	p.DueDate = parseDateTime(dueDate)
	p.LearningNewWord = learning == 1
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get pinyin progress: %w", err)
	}
	return &p, nil
}

// UpdatePinyinProgress saves updated progress for a pinyin sound.
func (s *Store) UpdatePinyinProgress(ctx context.Context, p models.SM2Progress) error {
	learningInt := 0
	if p.LearningNewWord {
		learningInt = 1
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE pinyin_progress
		 SET repetitions = ?, easiness = ?, interval_days = ?, due_date = ?,
		     total_correct = ?, total_attempts = ?, streak_bonus = ?, learning = ?
		 WHERE sound_id = ?`,
		p.Repetitions, p.Easiness, p.IntervalDays,
		p.DueDate.UTC().Format("2006-01-02 15:04:05"),
		p.TotalCorrect, p.TotalAttempts, p.StreakBonus, learningInt, p.WordID)
	if err != nil {
		return fmt.Errorf("update pinyin progress: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AcknowledgePinyinSound marks a sound as "seen" by setting first_seen_date.
func (s *Store) AcknowledgePinyinSound(ctx context.Context, soundID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE pinyin_progress SET first_seen_date = date('now')
		 WHERE sound_id = ? AND first_seen_date IS NULL`, soundID)
	if err != nil {
		return fmt.Errorf("acknowledge pinyin sound: %w", err)
	}
	return nil
}

// GetPinyinStats returns due/total/mastered counts, optionally filtered by tags.
func (s *Store) GetPinyinStats(ctx context.Context, tags []string) (due, total int, err error) {
	tagFilter := ""
	var tagArgs []any
	if len(tags) > 0 {
		placeholders := make([]string, len(tags))
		for i, t := range tags {
			placeholders[i] = "?"
			tagArgs = append(tagArgs, t)
		}
		tagFilter = ` AND s.tag IN (` + strings.Join(placeholders, ",") + `)`
	}

	args := append([]any{}, tagArgs...)
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pinyin_progress p
		 JOIN pinyin_sounds s ON s.id = p.sound_id
		 WHERE p.due_date <= CURRENT_TIMESTAMP AND p.first_seen_date IS NOT NULL`+tagFilter, args...).Scan(&due)
	if err != nil {
		return 0, 0, fmt.Errorf("count pinyin due: %w", err)
	}

	args = append([]any{}, tagArgs...)
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pinyin_sounds s WHERE 1=1`+tagFilter, args...).Scan(&total)
	if err != nil {
		return 0, 0, fmt.Errorf("count pinyin total: %w", err)
	}

	return due, total, nil
}

// ListPinyinTags returns all distinct tags from pinyin_sounds.
func (s *Store) ListPinyinTags(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT tag FROM pinyin_sounds WHERE tag != '' ORDER BY tag`)
	if err != nil {
		return nil, fmt.Errorf("list pinyin tags: %w", err)
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan pinyin tag: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// UpsertPinyinConfusion increments the confusion count between two sounds.
func (s *Store) UpsertPinyinConfusion(ctx context.Context, soundID, confusedWithID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pinyin_confusions (sound_id, confused_with_id, count, last_seen)
		 VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		 ON CONFLICT(sound_id, confused_with_id) DO UPDATE
		 SET count = count + 1, last_seen = CURRENT_TIMESTAMP`,
		soundID, confusedWithID)
	if err != nil {
		return fmt.Errorf("upsert pinyin confusion: %w", err)
	}
	return nil
}

// GetPinyinConfusionDetail returns confusion info between two sounds.
func (s *Store) GetPinyinConfusionDetail(ctx context.Context, soundID, confusedWithID int64) (*models.PinyinConfusionDetail, error) {
	var detail models.PinyinConfusionDetail
	var s1Syllable, s2Syllable string
	var s1Tone, s2Tone, count int
	err := s.db.QueryRowContext(ctx,
		`SELECT pc.sound_id, s1.syllable, s1.tone,
		        pc.confused_with_id, s2.syllable, s2.tone,
		        pc.count
		 FROM pinyin_confusions pc
		 JOIN pinyin_sounds s1 ON s1.id = pc.sound_id
		 JOIN pinyin_sounds s2 ON s2.id = pc.confused_with_id
		 WHERE pc.sound_id = ? AND pc.confused_with_id = ?`,
		soundID, confusedWithID).
		Scan(&detail.SoundID, &s1Syllable, &s1Tone,
			&detail.ConfusedWithID, &s2Syllable, &s2Tone,
			&count)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get pinyin confusion detail: %w", err)
	}
	detail.Count = count
	detail.SoundLabel = fmt.Sprintf("%s%d", s1Syllable, s1Tone)
	detail.ConfusedWithLabel = fmt.Sprintf("%s%d", s2Syllable, s2Tone)
	return &detail, nil
}

// ShufflePinyinOptions shuffles a slice of PinyinOption in place.
func ShufflePinyinOptions(opts []models.PinyinOption) {
	rand.Shuffle(len(opts), func(i, j int) {
		opts[i], opts[j] = opts[j], opts[i]
	})
}
