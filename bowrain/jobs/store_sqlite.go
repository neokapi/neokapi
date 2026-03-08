package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/gokapi/gokapi/bowrain/storage"
)

// sqliteJobMigrations defines the SQLite schema for translation jobs.
var sqliteJobMigrations = []storage.Migration{
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
				created_at         TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX IF NOT EXISTS idx_jobs_workspace ON translation_jobs(workspace_slug, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_jobs_status ON translation_jobs(status);
		`,
	},
	{
		Version:     2,
		Description: "add model and tokens_used columns",
		SQL: `
			ALTER TABLE translation_jobs ADD COLUMN model TEXT NOT NULL DEFAULT '';
			ALTER TABLE translation_jobs ADD COLUMN tokens_used INTEGER NOT NULL DEFAULT 0;
		`,
	},
}

// SQLiteJobStore implements JobStore using SQLite.
type SQLiteJobStore struct {
	db *storage.DB
}

// NewSQLiteJobStore creates a SQLite-backed JobStore.
// It runs migrations to ensure the translation_jobs table exists.
func NewSQLiteJobStore(db *storage.DB) (*SQLiteJobStore, error) {
	if err := storage.Migrate(db, sqliteJobMigrations); err != nil {
		return nil, fmt.Errorf("migrate job schema: %w", err)
	}
	return &SQLiteJobStore{db: db}, nil
}

func (s *SQLiteJobStore) CreateJob(ctx context.Context, job *TranslationJob) error {
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = StatusQueued
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO translation_jobs
			(id, workspace_slug, project_id, item_name, target_locale, provider_config_id,
			 model, status, progress, total_blocks, done_blocks, tokens_used, error, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		job.ID, job.WorkspaceSlug, job.ProjectID, job.ItemName, job.TargetLocale,
		job.ProviderConfigID, job.Model, string(job.Status), job.Progress, job.TotalBlocks,
		job.DoneBlocks, job.TokensUsed, job.Error, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

func (s *SQLiteJobStore) GetJob(ctx context.Context, id string) (*TranslationJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, target_locale,
				provider_config_id, model, status, progress, total_blocks, done_blocks,
				tokens_used, error, created_at, updated_at
		 FROM translation_jobs WHERE id = ?`, id)

	var j TranslationJob
	var status, createdAt, updatedAt string
	err := row.Scan(
		&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.TargetLocale,
		&j.ProviderConfigID, &j.Model, &status, &j.Progress, &j.TotalBlocks, &j.DoneBlocks,
		&j.TokensUsed, &j.Error, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan job: %w", err)
	}
	j.Status = JobStatus(status)
	j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	j.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &j, nil
}

func (s *SQLiteJobStore) ListJobs(ctx context.Context, workspaceSlug string, limit int) ([]*TranslationJob, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, target_locale,
				provider_config_id, model, status, progress, total_blocks, done_blocks,
				tokens_used, error, created_at, updated_at
		 FROM translation_jobs
		 WHERE workspace_slug = ?
		 ORDER BY created_at DESC
		 LIMIT ?`, workspaceSlug, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var result []*TranslationJob
	for rows.Next() {
		var j TranslationJob
		var status, createdAt, updatedAt string
		err := rows.Scan(
			&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.TargetLocale,
			&j.ProviderConfigID, &j.Model, &status, &j.Progress, &j.TotalBlocks, &j.DoneBlocks,
			&j.TokensUsed, &j.Error, &createdAt, &updatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan job row: %w", err)
		}
		j.Status = JobStatus(status)
		j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		j.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		result = append(result, &j)
	}
	return result, rows.Err()
}

func (s *SQLiteJobStore) UpdateJobProgress(ctx context.Context, id string, doneBlocks, totalBlocks int) error {
	progress := 0
	if totalBlocks > 0 {
		progress = doneBlocks * 100 / totalBlocks
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET done_blocks = ?, total_blocks = ?, progress = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		doneBlocks, totalBlocks, progress, id)
	if err != nil {
		return fmt.Errorf("update job progress: %w", err)
	}
	return nil
}

func (s *SQLiteJobStore) UpdateJobStatus(ctx context.Context, id string, status JobStatus, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET status = ?, error = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		string(status), errMsg, id)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	return nil
}

func (s *SQLiteJobStore) DeleteJob(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM translation_jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete job: %w", err)
	}
	return nil
}
