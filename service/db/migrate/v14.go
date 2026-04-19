package migrate

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"
)

func init() {
	register(migration{
		// v14: shuffle due_date of all unseen pinyin_progress rows so training
		// order is random instead of alphabetical (insertion) order.
		version: 14,
		fn: func(db *sql.DB) error {
			rows, err := db.Query(`SELECT sound_id FROM pinyin_progress WHERE first_seen_date IS NULL`)
			if err != nil {
				return fmt.Errorf("query unseen pinyin progress: %w", err)
			}
			var ids []int64
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return fmt.Errorf("scan sound_id: %w", err)
				}
				ids = append(ids, id)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterate unseen pinyin progress: %w", err)
			}

			rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })

			now := time.Now().UTC()
			for i, id := range ids {
				// Assign unique past timestamps in shuffled order (1 second apart).
				dueDate := now.Add(-time.Duration(len(ids)-i) * time.Second).Format("2006-01-02 15:04:05")
				if _, err := db.Exec(`UPDATE pinyin_progress SET due_date = ? WHERE sound_id = ?`, dueDate, id); err != nil {
					return fmt.Errorf("update due_date for sound_id %d: %w", id, err)
				}
			}
			return nil
		},
	})
}
