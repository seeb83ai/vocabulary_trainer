package db

import (
	"context"
	"fmt"
	"sort"
	"vocabulary_trainer/models"
)

// RecordComponentStat increments today's correct or wrong count in component_stats
// and snapshots the current total number of components in training for the user.
func (s *Store) RecordComponentStat(ctx context.Context, userID int64, correct bool) error {
	col := "wrong"
	if correct {
		col = "correct"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO component_stats (user_id, date, correct, wrong, components_total) VALUES (?, date('now'), 0, 0, 0)
		 ON CONFLICT(user_id, date) DO NOTHING`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("upsert component_stats row: %w", err)
	}
	if _, err = s.db.ExecContext(ctx,
		`UPDATE component_stats SET `+col+` = `+col+` + 1 WHERE user_id = ? AND date = date('now')`,
		userID,
	); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE component_stats
		    SET components_total = (SELECT COUNT(*) FROM component_progress WHERE user_id = ?)
		  WHERE user_id = ? AND date = date('now')`,
		userID, userID,
	)
	return err
}

// GetComponentStatsHistory returns daily component training stats for a user.
func (s *Store) GetComponentStatsHistory(ctx context.Context, userID int64) ([]models.ComponentDailyStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT date, correct, wrong, components_total FROM component_stats WHERE user_id = ? ORDER BY date ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get component stats history: %w", err)
	}
	var stats []models.ComponentDailyStat
	for rows.Next() {
		var s models.ComponentDailyStat
		if err := rows.Scan(&s.Date, &s.Correct, &s.Wrong, &s.ComponentsTotal); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan component stat: %w", err)
		}
		stats = append(stats, s)
	}
	rows.Close()
	return stats, rows.Err()
}

// ComponentCoverageComponent is one qualifying component across a user's
// current zh vocabulary (not just ones already in component_progress),
// together with the zh word IDs that require it. The Settings page uses this
// to preview, for a candidate coverage-target percentage, how many
// components selectComponentsForCoverage would pick — without listing
// individual components in the UI.
type ComponentCoverageComponent struct {
	Character string  `json:"character"`
	WordIDs   []int64 `json:"word_ids"`
}

// GetComponentCoverage returns every qualifying component across userID's
// current zh vocabulary — not just components already in component_progress —
// together with the zh word IDs that require each one, the total zh word
// count, and a per-word trainable-component count (word ID → count). Used by
// the Settings page to preview how many components a candidate coverage-target
// threshold would select (see selectComponentsForCoverage /
// getComponentCoverageThreshold). Sorted by character for stability.
func (s *Store) GetComponentCoverage(ctx context.Context, userID int64) ([]ComponentCoverageComponent, map[int64]int, int, error) {
	wordSets, wordComponentCounts, totalWords, err := componentWordSets(ctx, s.db, userID)
	if err != nil {
		return nil, nil, 0, err
	}

	items := make([]ComponentCoverageComponent, 0, len(wordSets))
	for char, words := range wordSets {
		ids := make([]int64, 0, len(words))
		for id := range words {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		items = append(items, ComponentCoverageComponent{Character: char, WordIDs: ids})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Character < items[j].Character })
	return items, wordComponentCounts, totalWords, nil
}
