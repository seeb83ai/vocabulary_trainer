package db

import (
	"context"
	"fmt"
)

// GetLookalikes returns the characters that look like text (e.g. 口 for 囗),
// sorted by character. Only whole-text matches count; returns nil when none.
func (s *Store) GetLookalikes(ctx context.Context, text string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT lookalike FROM lookalike_chars WHERE character = ? ORDER BY lookalike`, text)
	if err != nil {
		return nil, fmt.Errorf("get lookalikes for %q: %w", text, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
