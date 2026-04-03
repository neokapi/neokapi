package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
)

var pgExtractionMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create extraction_jobs table",
		SQL: `
			CREATE TABLE IF NOT EXISTS extraction_jobs (
				id             TEXT PRIMARY KEY,
				workspace_slug TEXT NOT NULL,
				project_id     TEXT NOT NULL,
				item_name      TEXT NOT NULL,
				locale         TEXT NOT NULL DEFAULT '',
				push_id        TEXT NOT NULL DEFAULT '',
				step_id        TEXT NOT NULL DEFAULT '',
				model          TEXT NOT NULL DEFAULT '',
				status         TEXT NOT NULL DEFAULT 'queued',
				total_blocks   INTEGER NOT NULL DEFAULT 0,
				done_blocks    INTEGER NOT NULL DEFAULT 0,
				items_created  INTEGER NOT NULL DEFAULT 0,
				error          TEXT NOT NULL DEFAULT '',
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_extraction_jobs_project ON extraction_jobs(project_id);
			CREATE INDEX IF NOT EXISTS idx_extraction_jobs_status ON extraction_jobs(status);
			CREATE INDEX IF NOT EXISTS idx_extraction_jobs_push_id ON extraction_jobs(push_id);
		`,
	},
}

// PgExtractionJobStore implements ExtractionJobStore using PostgreSQL.
type PgExtractionJobStore struct {
	db *storage.PgDB
}

// NewPgExtractionJobStore creates a PostgreSQL-backed ExtractionJobStore.
func NewPgExtractionJobStore(db *storage.PgDB) (*PgExtractionJobStore, error) {
	if err := storage.MigratePostgresNS(db, "extraction_schema_migrations", pgExtractionMigrations); err != nil {
		return nil, fmt.Errorf("migrate extraction schema: %w", err)
	}
	return &PgExtractionJobStore{db: db}, nil
}

func (s *PgExtractionJobStore) CreateExtractionJob(ctx context.Context, job *ExtractionJob) error {
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = ExtractionStatusQueued
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO extraction_jobs
			(id, workspace_slug, project_id, item_name, locale, push_id, step_id, model,
			 status, total_blocks, done_blocks, items_created, error, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		job.ID, job.WorkspaceSlug, job.ProjectID, job.ItemName, job.Locale,
		job.PushID, job.StepID, job.Model, string(job.Status), job.TotalBlocks,
		job.DoneBlocks, job.ItemsCreated, job.Error, now, now)
	if err != nil {
		return fmt.Errorf("insert extraction job: %w", err)
	}
	return nil
}

func (s *PgExtractionJobStore) GetExtractionJob(ctx context.Context, id string) (*ExtractionJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, locale, push_id, step_id, model,
				status, total_blocks, done_blocks, items_created, error, created_at, updated_at
		 FROM extraction_jobs WHERE id = $1`, id)

	var j ExtractionJob
	var status string
	err := row.Scan(
		&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.Locale,
		&j.PushID, &j.StepID, &j.Model, &status, &j.TotalBlocks, &j.DoneBlocks,
		&j.ItemsCreated, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan extraction job: %w", err)
	}
	j.Status = ExtractionJobStatus(status)
	return &j, nil
}

func (s *PgExtractionJobStore) UpdateExtractionJobStatus(ctx context.Context, id string, status ExtractionJobStatus, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs SET status = $1, error = $2, updated_at = NOW() WHERE id = $3`,
		string(status), errMsg, id)
	if err != nil {
		return fmt.Errorf("update extraction job status: %w", err)
	}
	return nil
}

func (s *PgExtractionJobStore) UpdateExtractionJobProgress(ctx context.Context, id string, doneBlocks, totalBlocks, itemsCreated int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs SET done_blocks = $1, total_blocks = $2, items_created = $3, updated_at = NOW() WHERE id = $4`,
		doneBlocks, totalBlocks, itemsCreated, id)
	if err != nil {
		return fmt.Errorf("update extraction job progress: %w", err)
	}
	return nil
}

func (s *PgExtractionJobStore) ClaimExtractionJob(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs SET status = 'processing', updated_at = NOW()
		 WHERE id = $1 AND status = 'queued'`, id)
	if err != nil {
		return false, fmt.Errorf("claim extraction job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *PgExtractionJobStore) ListByPushID(ctx context.Context, pushID string) ([]*ExtractionJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, locale, push_id, step_id, model,
				status, total_blocks, done_blocks, items_created, error, created_at, updated_at
		 FROM extraction_jobs WHERE push_id = $1 ORDER BY created_at`, pushID)
	if err != nil {
		return nil, fmt.Errorf("list extraction jobs by push: %w", err)
	}
	defer rows.Close()

	var result []*ExtractionJob
	for rows.Next() {
		var j ExtractionJob
		var status string
		if err := rows.Scan(
			&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.Locale,
			&j.PushID, &j.StepID, &j.Model, &status, &j.TotalBlocks, &j.DoneBlocks,
			&j.ItemsCreated, &j.Error, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan extraction job: %w", err)
		}
		j.Status = ExtractionJobStatus(status)
		result = append(result, &j)
	}
	return result, rows.Err()
}
