package db

import (
	"context"
	"database/sql"
	"fmt"
)

// GetWordImageURL returns the cached Unsplash image URL for a zh word owned
// by userID, or nil if none has been fetched yet (or the word doesn't exist
// / isn't owned by userID).
func (s *Store) GetWordImageURL(ctx context.Context, userID, wordID int64) (*string, error) {
	var imageURL sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT image_url FROM words WHERE id = ? AND user_id = ? AND language = 'zh'`,
		wordID, userID,
	).Scan(&imageURL)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get word image url: %w", err)
	}
	if !imageURL.Valid || imageURL.String == "" {
		return nil, nil
	}
	return &imageURL.String, nil
}

// SetWordImageURL caches a freshly fetched image URL for a word and stamps
// the fetch time.
func (s *Store) SetWordImageURL(ctx context.Context, userID, wordID int64, url string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE words SET image_url = ?, image_fetched_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND user_id = ? AND language = 'zh'`,
		url, wordID, userID,
	)
	if err != nil {
		return fmt.Errorf("set word image url: %w", err)
	}
	return nil
}
