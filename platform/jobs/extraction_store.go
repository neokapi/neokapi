package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/gokapi/gokapi/bowrain/storage"
)

// ExtractionJobStore persists extraction job state.
type ExtractionJobStore interface {
	CreateExtractionJob(ctx context.Context, job *ExtractionJob) error
	GetExtractionJob(ctx context.Context, id string) (*ExtractionJob, error)
	UpdateExtractionJobStatus(ctx context.Context, id string, status ExtractionJobStatus, errMsg string) error
	UpdateExtractionJobProgress(ctx context.Context, id string, doneBlocks, totalBlocks, itemsCreated int) error
}

// SQLiteExtractionJobStore implements ExtractionJobStore using SQLite.
// It shares the same database as SQLiteJobStore — the extraction_jobs table
// is created by migration 4 in sqliteJobMigrations.
type SQLiteExtractionJobStore struct {
	db *storage.DB
}

// NewSQLiteExtractionJobStore creates a SQLite-backed ExtractionJobStore.
// The DB must have been initialized via NewSQLiteJobStore (which runs all job migrations).
func NewSQLiteExtractionJobStore(db *storage.DB) *SQLiteExtractionJobStore {
	return &SQLiteExtractionJobStore{db: db}
}

func (s *SQLiteExtractionJobStore) CreateExtractionJob(ctx context.Context, job *ExtractionJob) error {
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = ExtractionStatusQueued
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO extraction_jobs
			(id, workspace_slug, project_id, item_name, locale, push_id, model,
			 status, total_blocks, done_blocks, items_created, error, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		job.ID, job.WorkspaceSlug, job.ProjectID, job.ItemName, job.Locale,
		job.PushID, job.Model, string(job.Status), job.TotalBlocks,
		job.DoneBlocks, job.ItemsCreated, job.Error,
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert extraction job: %w", err)
	}
	return nil
}

func (s *SQLiteExtractionJobStore) GetExtractionJob(ctx context.Context, id string) (*ExtractionJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, locale, push_id, model,
				status, total_blocks, done_blocks, items_created, error, created_at, updated_at
		 FROM extraction_jobs WHERE id = ?`, id)

	var j ExtractionJob
	var status, createdAt, updatedAt string
	err := row.Scan(
		&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.Locale,
		&j.PushID, &j.Model, &status, &j.TotalBlocks, &j.DoneBlocks,
		&j.ItemsCreated, &j.Error, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan extraction job: %w", err)
	}
	j.Status = ExtractionJobStatus(status)
	j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	j.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &j, nil
}

func (s *SQLiteExtractionJobStore) UpdateExtractionJobStatus(ctx context.Context, id string, status ExtractionJobStatus, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs SET status = ?, error = ?, updated_at = datetime('now') WHERE id = ?`,
		string(status), errMsg, id)
	if err != nil {
		return fmt.Errorf("update extraction job status: %w", err)
	}
	return nil
}

func (s *SQLiteExtractionJobStore) UpdateExtractionJobProgress(ctx context.Context, id string, doneBlocks, totalBlocks, itemsCreated int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs SET done_blocks = ?, total_blocks = ?, items_created = ?, updated_at = datetime('now') WHERE id = ?`,
		doneBlocks, totalBlocks, itemsCreated, id)
	if err != nil {
		return fmt.Errorf("update extraction job progress: %w", err)
	}
	return nil
}
