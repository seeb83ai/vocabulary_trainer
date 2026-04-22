package migrate

import (
	"database/sql"
	"fmt"
	"time"
)

func init() {
	register(migration{
		// v37: clear component_progress rows that were incorrectly populated by
		// the v36 migration (which inserted word characters instead of their
		// decomposition components), re-backfill with the correct logic, then
		// spread initial due dates so no user sees more than 5 new components
		// on any single day.
		version: 37,
		fn:      rebackfillComponents,
	})
}

func rebackfillComponents(db *sql.DB) error {
	if _, err := db.Exec(`DELETE FROM component_progress`); err != nil {
		return fmt.Errorf("v37 clear component_progress: %w", err)
	}
	if err := backfillComponents(db); err != nil {
		return err
	}
	return spreadComponentDueDates(db)
}

// spreadComponentDueDates ensures that, per user, no more than 5 component_progress
// rows share the same calendar day as their initial due_date.  Rows are processed
// in ascending due_date order; any overflow beyond 5 per day is pushed to the
// next calendar day, cascading until a slot is available.
func spreadComponentDueDates(db *sql.DB) error {
	const maxPerDay = 5

	userRows, err := db.Query(`SELECT DISTINCT user_id FROM component_progress`)
	if err != nil {
		return fmt.Errorf("spread: list users: %w", err)
	}
	var userIDs []int64
	for userRows.Next() {
		var uid int64
		if err := userRows.Scan(&uid); err != nil {
			userRows.Close()
			return fmt.Errorf("spread: scan user_id: %w", err)
		}
		userIDs = append(userIDs, uid)
	}
	userRows.Close()
	if err := userRows.Err(); err != nil {
		return fmt.Errorf("spread: iterate users: %w", err)
	}

	for _, uid := range userIDs {
		rows, err := db.Query(
			`SELECT id, due_date FROM component_progress WHERE user_id = ? ORDER BY due_date ASC`,
			uid,
		)
		if err != nil {
			return fmt.Errorf("spread: query components for user %d: %w", uid, err)
		}
		type comp struct {
			id      int64
			dueDate string
		}
		var comps []comp
		for rows.Next() {
			var c comp
			if err := rows.Scan(&c.id, &c.dueDate); err != nil {
				rows.Close()
				return fmt.Errorf("spread: scan component: %w", err)
			}
			comps = append(comps, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("spread: iterate components: %w", err)
		}

		counts := map[string]int{}
		for _, c := range comps {
			// Extract YYYY-MM-DD portion.
			date := c.dueDate
			if len(date) > 10 {
				date = date[:10]
			}
			// Advance day until a slot is available.
			for counts[date] >= maxPerDay {
				t, err := time.Parse("2006-01-02", date)
				if err != nil {
					return fmt.Errorf("spread: parse date %q: %w", date, err)
				}
				date = t.AddDate(0, 0, 1).Format("2006-01-02")
			}
			counts[date]++
			newDue := date + " 00:00:00"
			original := c.dueDate
			if len(original) > 10 {
				original = original[:10]
			}
			if date != original {
				if _, err := db.Exec(
					`UPDATE component_progress SET due_date = ? WHERE id = ?`,
					newDue, c.id,
				); err != nil {
					return fmt.Errorf("spread: update due_date for id %d: %w", c.id, err)
				}
			}
		}
	}
	return nil
}
