package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
)

// pgJobMigrations defines the PostgreSQL schema for translation jobs.
var pgJobMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create translation_jobs table",
		SQL: `
			CREATE TABLE IF NOT EXISTS translation_jobs (
				id                 TEXT PRIMARY KEY,
				workspace_slug     TEXT NOT NULL,
				project_id         TEXT NOT NULL,
				item_name          TEXT NOT NULL,
				target_locale      TEXT NOT NULL,
				provider_config_id TEXT NOT NULL DEFAULT '',
				status             TEXT NOT NULL DEFAULT 'queued',
				progress           INTEGER NOT NULL DEFAULT 0,
				total_blocks       INTEGER NOT NULL DEFAULT 0,
				done_blocks        INTEGER NOT NULL DEFAULT 0,
				error              TEXT NOT NULL DEFAULT '',
				created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_jobs_workspace ON translation_jobs(workspace_slug, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_jobs_status ON translation_jobs(status);
		`,
	},
	{
		Version:     2,
		Description: "add model and tokens_used columns",
		SQL: `
			ALTER TABLE translation_jobs ADD COLUMN IF NOT EXISTS model TEXT NOT NULL DEFAULT '';
			ALTER TABLE translation_jobs ADD COLUMN IF NOT EXISTS tokens_used INTEGER NOT NULL DEFAULT 0;
		`,
	},
	{
		Version:     3,
		Description: "add push_id column",
		SQL: `
			ALTER TABLE translation_jobs ADD COLUMN IF NOT EXISTS push_id TEXT NOT NULL DEFAULT '';
			CREATE INDEX IF NOT EXISTS idx_jobs_push_id ON translation_jobs(push_id) WHERE push_id != '';
		`,
	},
}

// PgJobStore implements JobStore using PostgreSQL.
type PgJobStore struct {
	db *storage.PgDB
}

// NewPgJobStore creates a PostgreSQL-backed JobStore.
// It runs migrations to ensure the translation_jobs table exists.
func NewPgJobStore(db *storage.PgDB) (*PgJobStore, error) {
	if err := storage.MigratePostgresNS(db, "jobs_schema_migrations", pgJobMigrations); err != nil {
		return nil, fmt.Errorf("migrate job schema: %w", err)
	}
	return &PgJobStore{db: db}, nil
}

func (s *PgJobStore) CreateJob(ctx context.Context, job *TranslationJob) error {
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = StatusQueued
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO translation_jobs
			(id, workspace_slug, project_id, item_name, target_locale, provider_config_id,
			 model, push_id, status, progress, total_blocks, done_blocks, tokens_used, error, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		job.ID, job.WorkspaceSlug, job.ProjectID, job.ItemName, job.TargetLocale,
		job.ProviderConfigID, job.Model, job.PushID, string(job.Status), job.Progress, job.TotalBlocks,
		job.DoneBlocks, job.TokensUsed, job.Error, now, now)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

func (s *PgJobStore) GetJob(ctx context.Context, id string) (*TranslationJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, target_locale,
				provider_config_id, model, push_id, status, progress, total_blocks, done_blocks,
				tokens_used, error, created_at, updated_at
		 FROM translation_jobs WHERE id = $1`, id)
	return scanJob(row)
}

func (s *PgJobStore) ListJobs(ctx context.Context, workspaceSlug string, limit int) ([]*TranslationJob, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, target_locale,
				provider_config_id, model, push_id, status, progress, total_blocks, done_blocks,
				tokens_used, error, created_at, updated_at
		 FROM translation_jobs
		 WHERE workspace_slug = $1
		 ORDER BY created_at DESC
		 LIMIT $2`, workspaceSlug, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()
	return scanJobs(rows)
}

func (s *PgJobStore) UpdateJobProgress(ctx context.Context, id string, doneBlocks, totalBlocks int) error {
	progress := 0
	if totalBlocks > 0 {
		progress = doneBlocks * 100 / totalBlocks
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET done_blocks = $1, total_blocks = $2, progress = $3, updated_at = NOW()
		 WHERE id = $4`,
		doneBlocks, totalBlocks, progress, id)
	if err != nil {
		return fmt.Errorf("update job progress: %w", err)
	}
	return nil
}

func (s *PgJobStore) UpdateJobStatus(ctx context.Context, id string, status JobStatus, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET status = $1, error = $2, updated_at = NOW()
		 WHERE id = $3`,
		string(status), errMsg, id)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	return nil
}

func (s *PgJobStore) DeleteJob(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM translation_jobs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete job: %w", err)
	}
	return nil
}

func (s *PgJobStore) ListJobsByPushID(ctx context.Context, pushID string) ([]*TranslationJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, target_locale,
				provider_config_id, model, push_id, status, progress, total_blocks, done_blocks,
				tokens_used, error, created_at, updated_at
		 FROM translation_jobs
		 WHERE push_id = $1
		 ORDER BY created_at ASC`, pushID)
	if err != nil {
		return nil, fmt.Errorf("list jobs by push_id: %w", err)
	}
	defer rows.Close()
	return scanJobs(rows)
}

// scanJob scans a single TranslationJob from a sql.Row.
func scanJob(row *sql.Row) (*TranslationJob, error) {
	var j TranslationJob
	var status string
	err := row.Scan(
		&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.TargetLocale,
		&j.ProviderConfigID, &j.Model, &j.PushID, &status, &j.Progress, &j.TotalBlocks, &j.DoneBlocks,
		&j.TokensUsed, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan job: %w", err)
	}
	j.Status = JobStatus(status)
	return &j, nil
}

// scanJobs scans multiple TranslationJob rows.
func scanJobs(rows *sql.Rows) ([]*TranslationJob, error) {
	var result []*TranslationJob
	for rows.Next() {
		var j TranslationJob
		var status string
		err := rows.Scan(
			&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.TargetLocale,
			&j.ProviderConfigID, &j.Model, &j.PushID, &status, &j.Progress, &j.TotalBlocks, &j.DoneBlocks,
			&j.TokensUsed, &j.Error, &j.CreatedAt, &j.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan job row: %w", err)
		}
		j.Status = JobStatus(status)
		result = append(result, &j)
	}
	return result, rows.Err()
}
