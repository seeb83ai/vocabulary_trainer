package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"vocabulary_trainer/models"
)

const importJobColumns = `id, user_id, tag, import_langs, apply_tags, and_tags, import_mode, status, total, done, imported, tagged, skipped, error, created_at, updated_at`

func scanImportJob(row interface{ Scan(...any) error }) (*models.ImportJob, error) {
	var j models.ImportJob
	var langs, applyTags, andTags, createdAt, updatedAt string
	if err := row.Scan(&j.ID, &j.UserID, &j.Tag, &langs, &applyTags, &andTags, &j.ImportMode, &j.Status,
		&j.Total, &j.Done, &j.Imported, &j.Tagged, &j.Skipped, &j.Error, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(langs), &j.ImportLangs); err != nil {
		return nil, fmt.Errorf("decode import_langs: %w", err)
	}
	if err := json.Unmarshal([]byte(applyTags), &j.ApplyTags); err != nil {
		return nil, fmt.Errorf("decode apply_tags: %w", err)
	}
	if err := json.Unmarshal([]byte(andTags), &j.AndTags); err != nil {
		return nil, fmt.Errorf("decode and_tags: %w", err)
	}
	j.CreatedAt = parseDateTime(createdAt)
	j.UpdatedAt = parseDateTime(updatedAt)
	return &j, nil
}

// CreateImportJob queues an import for userID from the Tag, ImportLangs,
// ApplyTags, AndTags and ImportMode of spec. When the user already has a
// queued or running job for the same tag, and_tags and import_mode, that job
// is returned instead of a second one.
func (s *Store) CreateImportJob(ctx context.Context, userID int64, spec models.ImportJob) (*models.ImportJob, error) {
	jsonList := func(list []string) string {
		if list == nil {
			list = []string{}
		}
		b, _ := json.Marshal(list)
		return string(b)
	}
	if spec.ImportMode == "" {
		spec.ImportMode = "include"
	}
	andTags := jsonList(spec.AndTags)
	active, err := scanImportJob(s.db.QueryRowContext(ctx,
		`SELECT `+importJobColumns+` FROM import_jobs
		 WHERE user_id = ? AND tag = ? AND and_tags = ? AND import_mode = ?
		   AND status IN ('queued', 'running') ORDER BY id LIMIT 1`,
		userID, spec.Tag, andTags, spec.ImportMode))
	if err == nil {
		return active, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("find active import job: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO import_jobs (user_id, tag, import_langs, apply_tags, and_tags, import_mode)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, spec.Tag, jsonList(spec.ImportLangs), jsonList(spec.ApplyTags), andTags, spec.ImportMode)
	if err != nil {
		return nil, fmt.Errorf("create import job: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetImportJob(ctx, userID, id)
}

// GetImportJob returns the job with the given ID if it belongs to userID,
// or nil when there is none.
func (s *Store) GetImportJob(ctx context.Context, userID, id int64) (*models.ImportJob, error) {
	j, err := scanImportJob(s.db.QueryRowContext(ctx,
		`SELECT `+importJobColumns+` FROM import_jobs WHERE id = ? AND user_id = ?`, id, userID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get import job: %w", err)
	}
	return j, nil
}

func (s *Store) listImportJobs(ctx context.Context, where string, args ...any) ([]models.ImportJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+importJobColumns+` FROM import_jobs WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list import jobs: %w", err)
	}
	defer rows.Close()
	jobs := []models.ImportJob{}
	for rows.Next() {
		j, err := scanImportJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

// ListRunnableImportJobs returns every queued or running job of all users,
// oldest first. The worker calls it at startup to resume interrupted jobs.
func (s *Store) ListRunnableImportJobs(ctx context.Context) ([]models.ImportJob, error) {
	return s.listImportJobs(ctx, `status IN ('queued', 'running')`)
}

// ListActiveImportJobs returns userID's queued or running jobs, oldest first.
func (s *Store) ListActiveImportJobs(ctx context.Context, userID int64) ([]models.ImportJob, error) {
	return s.listImportJobs(ctx, `user_id = ? AND status IN ('queued', 'running')`, userID)
}

// StartImportJob marks the job running and resets its counters, because a
// resumed job starts again from the first source word.
func (s *Store) StartImportJob(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE import_jobs SET status = 'running', total = 0, done = 0, imported = 0, tagged = 0, skipped = 0,
		        error = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

// UpdateImportJobProgress stores the counters of p (Total, Done, Imported,
// Tagged, Skipped) on the job.
func (s *Store) UpdateImportJobProgress(ctx context.Context, id int64, p models.ImportJob) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE import_jobs SET total = ?, done = ?, imported = ?, tagged = ?, skipped = ?,
		        updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		p.Total, p.Done, p.Imported, p.Tagged, p.Skipped, id)
	return err
}

// FinishImportJob marks the job done, or failed with errMsg when errMsg is set.
func (s *Store) FinishImportJob(ctx context.Context, id int64, errMsg string) error {
	status := "done"
	if errMsg != "" {
		status = "failed"
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE import_jobs SET status = ?, error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, errMsg, id)
	return err
}
