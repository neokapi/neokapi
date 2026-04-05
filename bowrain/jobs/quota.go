package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
)

// QuotaStore tracks AI token usage per workspace and project.
type QuotaStore interface {
	// CheckQuota returns the remaining tokens for a workspace (across all models).
	// Returns (remaining, error). A negative remaining means over quota.
	CheckQuota(ctx context.Context, workspaceSlug string) (int64, error)

	// RecordUsage records token usage for a translation job.
	RecordUsage(ctx context.Context, usage AIUsageRecord) error

	// GetUsageSummary returns usage summary for a workspace in the current billing period.
	GetUsageSummary(ctx context.Context, workspaceSlug string) (*UsageSummary, error)
}

// AIUsageRecord represents a single AI API call's token usage.
type AIUsageRecord struct {
	WorkspaceSlug string
	WorkspaceID   string // preferred; aligns with billing system
	ProjectID     string
	JobID         string
	Model         string
	Operation     string // e.g., "translate", "qa_check", "review", "entity_extract"
	PromptTokens  int
	OutputTokens  int
	TotalTokens   int
}

// UsageSummary summarizes AI usage for a workspace.
type UsageSummary struct {
	WorkspaceSlug   string    `json:"workspace_slug"`
	MonthlyLimit    int64     `json:"monthly_limit"`
	UsedTokens      int64     `json:"used_tokens"`
	RemainingTokens int64     `json:"remaining_tokens"`
	PeriodStart     time.Time `json:"period_start"`
}

// DefaultMonthlyQuota is the default token quota per workspace per month.
const DefaultMonthlyQuota int64 = 10_000_000 // 10M tokens

// quotaMigrations defines the schema for AI usage tracking.
var quotaMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create ai_usage table",
		SQL: `
			CREATE TABLE IF NOT EXISTS ai_usage (
				id              BIGSERIAL PRIMARY KEY,
				workspace_slug  TEXT NOT NULL,
				project_id      TEXT NOT NULL,
				job_id          TEXT NOT NULL DEFAULT '',
				model           TEXT NOT NULL,
				prompt_tokens   INTEGER NOT NULL DEFAULT 0,
				output_tokens   INTEGER NOT NULL DEFAULT 0,
				total_tokens    INTEGER NOT NULL DEFAULT 0,
				created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_ai_usage_workspace_period
				ON ai_usage(workspace_slug, created_at);

			CREATE TABLE IF NOT EXISTS ai_quotas (
				workspace_slug  TEXT PRIMARY KEY,
				monthly_limit   BIGINT NOT NULL DEFAULT 10000000,
				updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
		`,
	},
	{
		Version:     2,
		Description: "add operation column to ai_usage",
		SQL: `
			ALTER TABLE ai_usage ADD COLUMN IF NOT EXISTS operation TEXT NOT NULL DEFAULT '';
			CREATE INDEX IF NOT EXISTS idx_ai_usage_operation
				ON ai_usage(workspace_slug, operation, created_at);
		`,
	},
	{
		Version:     3,
		Description: "add workspace_id column to ai_usage for billing alignment",
		SQL: `
			ALTER TABLE ai_usage ADD COLUMN IF NOT EXISTS workspace_id TEXT NOT NULL DEFAULT '';
			CREATE INDEX IF NOT EXISTS idx_ai_usage_workspace_id
				ON ai_usage(workspace_id, created_at);
			ALTER TABLE ai_quotas ADD COLUMN IF NOT EXISTS workspace_id TEXT NOT NULL DEFAULT '';
		`,
	},
}

// QuotaStoreDB implements QuotaStore using PostgreSQL.
// Exported because callers use the concrete type for additional methods
// beyond the QuotaStore interface (e.g., GetUsageByModel, RecordRunnerUsage).
type QuotaStoreDB struct {
	db *storage.PgDB
}

// NewQuotaStore creates a PostgreSQL-backed QuotaStore.
func NewQuotaStore(db *storage.PgDB) (*QuotaStoreDB, error) {
	if err := storage.MigratePostgresNS(db, "quota_schema_migrations", quotaMigrations); err != nil {
		return nil, fmt.Errorf("migrate quota schema: %w", err)
	}
	s := &QuotaStoreDB{db: db}
	if err := s.initRunnerSchema(); err != nil {
		return nil, fmt.Errorf("migrate runner schema: %w", err)
	}
	return s, nil
}

func (s *QuotaStoreDB) CheckQuota(ctx context.Context, workspaceSlug string) (int64, error) {
	// Get the quota limit (use default if not set).
	var limit int64
	err := s.db.QueryRowContext(ctx,
		`SELECT monthly_limit FROM ai_quotas WHERE workspace_slug = $1`,
		workspaceSlug).Scan(&limit)
	if errors.Is(err, sql.ErrNoRows) {
		limit = DefaultMonthlyQuota
	} else if err != nil {
		return 0, fmt.Errorf("get quota: %w", err)
	}

	// Sum usage for current month.
	periodStart := currentPeriodStart()
	var used int64
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(total_tokens), 0) FROM ai_usage
		 WHERE workspace_slug = $1 AND created_at >= $2`,
		workspaceSlug, periodStart).Scan(&used)
	if err != nil {
		return 0, fmt.Errorf("sum usage: %w", err)
	}

	return limit - used, nil
}

func (s *QuotaStoreDB) RecordUsage(ctx context.Context, usage AIUsageRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ai_usage
			(workspace_slug, workspace_id, project_id, job_id, model, operation, prompt_tokens, output_tokens, total_tokens)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		usage.WorkspaceSlug, usage.WorkspaceID, usage.ProjectID, usage.JobID, usage.Model, usage.Operation,
		usage.PromptTokens, usage.OutputTokens, usage.TotalTokens)
	if err != nil {
		return fmt.Errorf("record usage: %w", err)
	}
	return nil
}

func (s *QuotaStoreDB) GetUsageSummary(ctx context.Context, workspaceSlug string) (*UsageSummary, error) {
	var limit int64
	err := s.db.QueryRowContext(ctx,
		`SELECT monthly_limit FROM ai_quotas WHERE workspace_slug = $1`,
		workspaceSlug).Scan(&limit)
	if errors.Is(err, sql.ErrNoRows) {
		limit = DefaultMonthlyQuota
	} else if err != nil {
		return nil, fmt.Errorf("get quota: %w", err)
	}

	periodStart := currentPeriodStart()
	var used int64
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(total_tokens), 0) FROM ai_usage
		 WHERE workspace_slug = $1 AND created_at >= $2`,
		workspaceSlug, periodStart).Scan(&used)
	if err != nil {
		return nil, fmt.Errorf("sum usage: %w", err)
	}

	return &UsageSummary{
		WorkspaceSlug:   workspaceSlug,
		MonthlyLimit:    limit,
		UsedTokens:      used,
		RemainingTokens: limit - used,
		PeriodStart:     periodStart,
	}, nil
}

// ModelUsage summarizes token usage for a specific model and operation.
type ModelUsage struct {
	Model        string `json:"model"`
	Operation    string `json:"operation"`
	PromptTokens int64  `json:"prompt_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	CallCount    int64  `json:"call_count"`
}

// GetUsageByModel returns token usage grouped by model and operation for a workspace.
// Uses workspace_id (aligned with billing system) with fallback to workspace_slug.
func (s *QuotaStoreDB) GetUsageByModel(ctx context.Context, workspaceID string, from, to time.Time) ([]ModelUsage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT model, operation,
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COUNT(*)
		 FROM ai_usage
		 WHERE (workspace_id = $1 OR workspace_slug = $1) AND created_at >= $2 AND created_at < $3
		 GROUP BY model, operation
		 ORDER BY SUM(total_tokens) DESC`,
		workspaceID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query model usage: %w", err)
	}
	defer rows.Close()

	var result []ModelUsage
	for rows.Next() {
		var mu ModelUsage
		if err := rows.Scan(&mu.Model, &mu.Operation, &mu.PromptTokens, &mu.OutputTokens, &mu.TotalTokens, &mu.CallCount); err != nil {
			return nil, fmt.Errorf("scan model usage: %w", err)
		}
		result = append(result, mu)
	}
	return result, rows.Err()
}

// RunnerUsageRecord represents a single runner/container duration record.
type RunnerUsageRecord struct {
	WorkspaceID string
	ProjectID   string  // empty for agent containers
	Operation   string  // "bravo_container", "auto_translate", "auto_extract"
	DurationSec float64 // wall-clock seconds
	ReferenceID string  // step ID, conversation ID, etc.
}

// runnerMigrations extends the quota schema with runner usage.
var runnerMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create runner_usage table",
		SQL: `
			CREATE TABLE IF NOT EXISTS runner_usage (
				id              BIGSERIAL PRIMARY KEY,
				workspace_id    TEXT NOT NULL,
				project_id      TEXT NOT NULL DEFAULT '',
				operation       TEXT NOT NULL,
				duration_sec    REAL NOT NULL DEFAULT 0,
				reference_id    TEXT NOT NULL DEFAULT '',
				created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_runner_usage_workspace
				ON runner_usage(workspace_id, created_at);
		`,
	},
}

// initRunnerSchema runs runner_usage migrations (separate namespace from ai_usage).
func (s *QuotaStoreDB) initRunnerSchema() error {
	return storage.MigratePostgresNS(s.db, "runner_schema_migrations", runnerMigrations)
}

// RecordRunnerUsage records a runner/container duration.
func (s *QuotaStoreDB) RecordRunnerUsage(ctx context.Context, usage RunnerUsageRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runner_usage
			(workspace_id, project_id, operation, duration_sec, reference_id)
		 VALUES ($1, $2, $3, $4, $5)`,
		usage.WorkspaceID, usage.ProjectID, usage.Operation, usage.DurationSec, usage.ReferenceID)
	if err != nil {
		return fmt.Errorf("record runner usage: %w", err)
	}
	return nil
}

// RunnerUsageSummary summarizes runner time for a specific operation.
type RunnerUsageSummary struct {
	Operation    string  `json:"operation"`
	TotalSeconds float64 `json:"total_seconds"`
	Count        int64   `json:"count"`
}

// GetRunnerUsage returns runner time grouped by operation for a workspace.
func (s *QuotaStoreDB) GetRunnerUsage(ctx context.Context, workspaceID string, from, to time.Time) ([]RunnerUsageSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT operation,
			COALESCE(SUM(duration_sec), 0),
			COUNT(*)
		 FROM runner_usage
		 WHERE workspace_id = $1 AND created_at >= $2 AND created_at < $3
		 GROUP BY operation
		 ORDER BY SUM(duration_sec) DESC`,
		workspaceID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query runner usage: %w", err)
	}
	defer rows.Close()

	var result []RunnerUsageSummary
	for rows.Next() {
		var ru RunnerUsageSummary
		if err := rows.Scan(&ru.Operation, &ru.TotalSeconds, &ru.Count); err != nil {
			return nil, fmt.Errorf("scan runner usage: %w", err)
		}
		result = append(result, ru)
	}
	return result, rows.Err()
}

// currentPeriodStart returns the start of the current billing month (1st of month, UTC).
func currentPeriodStart() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}
