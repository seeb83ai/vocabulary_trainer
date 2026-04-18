package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"vocabulary_trainer/models"
)

// GetAllTags returns all tag names for words belonging to the given user, ordered alphabetically.
func (s *Store) GetAllTags(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT tg.name FROM tags tg
		 JOIN word_tags wt ON wt.tag_id = tg.id
		 JOIN words w ON w.id = wt.word_id
		 WHERE w.user_id = ?
		 ORDER BY tg.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("get all tags: %w", err)
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, rows.Err()
}

// GetTagDetails returns all tags owned by userID with their description and importable flag.
func (s *Store) GetTagDetails(ctx context.Context, userID int64) ([]models.TagDetail, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT tg.name, tg.description, tg.importable
		 FROM tags tg
		 JOIN word_tags wt ON wt.tag_id = tg.id
		 JOIN words w ON w.id = wt.word_id
		 WHERE w.user_id = ?
		 ORDER BY tg.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("get tag details: %w", err)
	}
	defer rows.Close()
	var out []models.TagDetail
	for rows.Next() {
		var td models.TagDetail
		var imp int
		if err := rows.Scan(&td.Name, &td.Description, &imp); err != nil {
			return nil, err
		}
		td.Importable = imp != 0
		out = append(out, td)
	}
	if out == nil {
		out = []models.TagDetail{}
	}
	return out, rows.Err()
}

// UpsertTagMeta updates description and importable on the tag row that is linked to
// the given user's words. Tags are created globally (user_id = NULL) by getOrCreateTag,
// so we update by name matching the tag used by this user's words via word_tags.
func (s *Store) UpsertTagMeta(ctx context.Context, userID int64, name, description string, importable bool) error {
	imp := 0
	if importable {
		imp = 1
	}
	// Update the tag row that is referenced by this user's words.
	// The sub-select finds the tag ID used by at least one of the user's words.
	if _, err := s.db.ExecContext(ctx, `
		UPDATE tags SET description = ?, importable = ?
		WHERE name = ?
		  AND id IN (
		    SELECT DISTINCT wt.tag_id FROM word_tags wt
		    JOIN words w ON w.id = wt.word_id
		    WHERE w.user_id = ?
		  )`, description, imp, name, userID); err != nil {
		return fmt.Errorf("upsert tag meta: %w", err)
	}
	return nil
}

// GetImportableSourceTags returns tags for userID where importable = 1, ordered alphabetically.
// Each TagDetail includes WithEn/WithDe flags indicating whether any zh word in the tag
// has at least one EN or DE translation respectively.
func (s *Store) GetImportableSourceTags(ctx context.Context, userID int64) ([]models.TagDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
		  tg.name,
		  tg.description,
		  tg.importable,
		  EXISTS (
		    SELECT 1 FROM word_tags wt2
		    JOIN words zh ON zh.id = wt2.word_id AND zh.user_id = ? AND zh.language = 'zh'
		    JOIN translations tr ON tr.zh_word_id = zh.id
		    JOIN words en ON en.id = tr.en_word_id AND en.language = 'en'
		    WHERE wt2.tag_id = tg.id
		  ) AS with_en,
		  EXISTS (
		    SELECT 1 FROM word_tags wt3
		    JOIN words zh ON zh.id = wt3.word_id AND zh.user_id = ? AND zh.language = 'zh'
		    JOIN translations tr ON tr.zh_word_id = zh.id
		    JOIN words de ON de.id = tr.en_word_id AND de.language = 'de'
		    WHERE wt3.tag_id = tg.id
		  ) AS with_de
		FROM tags tg
		JOIN word_tags wt ON wt.tag_id = tg.id
		JOIN words w ON w.id = wt.word_id
		WHERE w.user_id = ? AND tg.importable = 1
		GROUP BY tg.id
		ORDER BY tg.name`, userID, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("get importable source tags: %w", err)
	}
	defer rows.Close()
	var out []models.TagDetail
	for rows.Next() {
		var td models.TagDetail
		var imp, withEn, withDe int
		if err := rows.Scan(&td.Name, &td.Description, &imp, &withEn, &withDe); err != nil {
			return nil, err
		}
		td.Importable = imp != 0
		td.WithEn = withEn != 0
		td.WithDe = withDe != 0
		out = append(out, td)
	}
	if out == nil {
		out = []models.TagDetail{}
	}
	return out, rows.Err()
}

func (s *Store) getTagsForWord(ctx context.Context, wordID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tg.name FROM tags tg
		 JOIN word_tags wt ON wt.tag_id = tg.id
		 WHERE wt.word_id = ?
		 ORDER BY tg.name`, wordID)
	if err != nil {
		return nil, fmt.Errorf("get tags: %w", err)
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, rows.Err()
}

func getOrCreateTag(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO tags (name) VALUES (?)`, name); err != nil {
		return 0, fmt.Errorf("upsert tag: %w", err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM tags WHERE name = ?`, name).Scan(&id); err != nil {
		return 0, fmt.Errorf("get tag id: %w", err)
	}
	return id, nil
}

func setWordTags(ctx context.Context, tx *sql.Tx, wordID int64, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM word_tags WHERE word_id = ?`, wordID); err != nil {
		return fmt.Errorf("delete word tags: %w", err)
	}
	for _, name := range tags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		tagID, err := getOrCreateTag(ctx, tx, name)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO word_tags (word_id, tag_id) VALUES (?, ?)`,
			wordID, tagID); err != nil {
			return fmt.Errorf("link tag: %w", err)
		}
	}
	return nil
}

func (s *Store) cleanOrphanTags(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM tags WHERE id NOT IN (SELECT DISTINCT tag_id FROM word_tags)`)
	if err != nil {
		return fmt.Errorf("clean orphan tags: %w", err)
	}
	return nil
}
