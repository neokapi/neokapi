package jobs

import (
	"context"
	"database/sql"
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
	ProjectID     string
	JobID         string
	Model         string
	PromptTokens  int
	OutputTokens  int
	TotalTokens   int
}

// UsageSummary summarizes AI usage for a workspace.
type UsageSummary struct {
	WorkspaceSlug  string `json:"workspace_slug"`
	MonthlyLimit   int64  `json:"monthly_limit"`
	UsedTokens     int64  `json:"used_tokens"`
	RemainingTokens int64 `json:"remaining_tokens"`
	PeriodStart    time.Time `json:"period_start"`
}

// DefaultMonthlyQuota is the default token quota per workspace per month.
const DefaultMonthlyQuota int64 = 10_000_000 // 10M tokens

// pgQuotaMigrations defines the schema for AI usage tracking.
var pgQuotaMigrations = []storage.Migration{
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
}

// PgQuotaStore implements QuotaStore using PostgreSQL.
type PgQuotaStore struct {
	db *storage.PgDB
}

// NewPgQuotaStore creates a PostgreSQL-backed QuotaStore.
func NewPgQuotaStore(db *storage.PgDB) (*PgQuotaStore, error) {
	if err := storage.MigratePostgresNS(db, "quota_schema_migrations", pgQuotaMigrations); err != nil {
		return nil, fmt.Errorf("migrate quota schema: %w", err)
	}
	return &PgQuotaStore{db: db}, nil
}

func (s *PgQuotaStore) CheckQuota(ctx context.Context, workspaceSlug string) (int64, error) {
	// Get the quota limit (use default if not set).
	var limit int64
	err := s.db.QueryRowContext(ctx,
		`SELECT monthly_limit FROM ai_quotas WHERE workspace_slug = $1`,
		workspaceSlug).Scan(&limit)
	if err == sql.ErrNoRows {
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

func (s *PgQuotaStore) RecordUsage(ctx context.Context, usage AIUsageRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ai_usage
			(workspace_slug, project_id, job_id, model, prompt_tokens, output_tokens, total_tokens)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		usage.WorkspaceSlug, usage.ProjectID, usage.JobID, usage.Model,
		usage.PromptTokens, usage.OutputTokens, usage.TotalTokens)
	if err != nil {
		return fmt.Errorf("record usage: %w", err)
	}
	return nil
}

func (s *PgQuotaStore) GetUsageSummary(ctx context.Context, workspaceSlug string) (*UsageSummary, error) {
	var limit int64
	err := s.db.QueryRowContext(ctx,
		`SELECT monthly_limit FROM ai_quotas WHERE workspace_slug = $1`,
		workspaceSlug).Scan(&limit)
	if err == sql.ErrNoRows {
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

// currentPeriodStart returns the start of the current billing month (1st of month, UTC).
func currentPeriodStart() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}
