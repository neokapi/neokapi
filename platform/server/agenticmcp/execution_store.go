package agenticmcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
)

// agenticMigrations defines the PostgreSQL schema for agentic execution tracking.
var agenticMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create agentic execution tracking tables",
		SQL: `
			CREATE TABLE agentic_executions (
				id             TEXT PRIMARY KEY,
				workspace_slug TEXT NOT NULL,
				agent          TEXT NOT NULL,
				role           TEXT NOT NULL,
				status         TEXT NOT NULL DEFAULT 'running',
				task           TEXT NOT NULL DEFAULT '',
				locale         TEXT NOT NULL DEFAULT '',
				summary        TEXT NOT NULL DEFAULT '',
				tokens_used    INTEGER NOT NULL DEFAULT 0,
				error          TEXT NOT NULL DEFAULT '',
				started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				completed_at   TIMESTAMPTZ,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_agentic_exec_ws ON agentic_executions(workspace_slug, created_at DESC);
			CREATE INDEX idx_agentic_exec_agent ON agentic_executions(agent, created_at DESC);
			CREATE INDEX idx_agentic_exec_status ON agentic_executions(status) WHERE status = 'running';

			CREATE TABLE agentic_events (
				id             TEXT PRIMARY KEY,
				execution_id   TEXT NOT NULL,
				workspace_slug TEXT NOT NULL,
				agent          TEXT NOT NULL,
				role           TEXT NOT NULL,
				event_type     TEXT NOT NULL,
				data           JSONB NOT NULL DEFAULT '{}',
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_agentic_events_exec ON agentic_events(execution_id, created_at);
			CREATE INDEX idx_agentic_events_ws ON agentic_events(workspace_slug, created_at DESC);
			CREATE INDEX idx_agentic_events_type ON agentic_events(event_type, created_at DESC);
		`,
	},
}

// PostgresExecutionStore persists agentic execution history to PostgreSQL.
type PostgresExecutionStore struct {
	db *sql.DB
}

// NewPostgresExecutionStore creates a new execution store, running migrations
// on the shared PostgreSQL database under the "agentic_schema_migrations" namespace.
func NewPostgresExecutionStore(pgDB *storage.PgDB) (*PostgresExecutionStore, error) {
	if err := storage.MigratePostgresNS(pgDB, "agentic_schema_migrations", agenticMigrations); err != nil {
		return nil, fmt.Errorf("migrate agentic schema: %w", err)
	}
	return &PostgresExecutionStore{db: pgDB.DB}, nil
}

// UpsertExecution inserts or updates an execution row from a lifecycle event.
// exec.started creates the row; exec.completed/exec.failed update it.
func (s *PostgresExecutionStore) UpsertExecution(ctx context.Context, ev *AgenticEvent) error {
	switch ev.Type {
	case EventExecStarted:
		task, _ := ev.Data["task"].(string)
		locale, _ := ev.Data["locale"].(string)
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO agentic_executions (id, workspace_slug, agent, role, status, task, locale, started_at)
			VALUES ($1, $2, $3, $4, 'running', $5, $6, $7)
			ON CONFLICT (id) DO UPDATE SET
				status = 'running',
				task = EXCLUDED.task,
				locale = EXCLUDED.locale,
				started_at = EXCLUDED.started_at
		`, ev.ExecutionID, ev.Workspace, ev.Agent, ev.Role, task, locale, ev.Timestamp)
		return err

	case EventExecCompleted:
		summary, _ := ev.Data["summary"].(string)
		tokensUsed := intFromData(ev.Data, "tokens_used")
		_, err := s.db.ExecContext(ctx, `
			UPDATE agentic_executions
			SET status = 'completed', summary = $2, tokens_used = $3, completed_at = $4
			WHERE id = $1
		`, ev.ExecutionID, summary, tokensUsed, ev.Timestamp)
		return err

	case EventExecFailed:
		errMsg, _ := ev.Data["error"].(string)
		tokensUsed := intFromData(ev.Data, "tokens_used")
		_, err := s.db.ExecContext(ctx, `
			UPDATE agentic_executions
			SET status = 'failed', error = $2, tokens_used = $3, completed_at = $4
			WHERE id = $1
		`, ev.ExecutionID, errMsg, tokensUsed, ev.Timestamp)
		return err

	default:
		return nil // non-lifecycle events don't affect the executions table
	}
}

// InsertEvent appends an event to the agentic_events log table.
func (s *PostgresExecutionStore) InsertEvent(ctx context.Context, ev *AgenticEvent) error {
	dataJSON, err := json.Marshal(ev.Data)
	if err != nil {
		dataJSON = []byte("{}")
	}

	eventID := fmt.Sprintf("evt_%d", time.Now().UnixNano())
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO agentic_events (id, execution_id, workspace_slug, agent, role, event_type, data, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, eventID, ev.ExecutionID, ev.Workspace, ev.Agent, ev.Role, string(ev.Type), dataJSON, ev.Timestamp)
	return err
}

// ListExecutions returns recent execution records matching the filter.
func (s *PostgresExecutionStore) ListExecutions(ctx context.Context, filter ExecutionFilter) ([]Execution, error) {
	query := `SELECT id, workspace_slug, agent, role, status, task, locale, summary, tokens_used, error, started_at, completed_at
		FROM agentic_executions WHERE 1=1`
	var args []any
	argN := 1

	if filter.WorkspaceSlug != "" {
		query += fmt.Sprintf(" AND workspace_slug = $%d", argN)
		args = append(args, filter.WorkspaceSlug)
		argN++
	}
	if filter.Agent != "" {
		query += fmt.Sprintf(" AND agent = $%d", argN)
		args = append(args, filter.Agent)
		argN++
	}
	if filter.Since != "" {
		query += fmt.Sprintf(" AND created_at >= $%d", argN)
		args = append(args, filter.Since)
		argN++
	}

	query += " ORDER BY created_at DESC"

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	query += fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query executions: %w", err)
	}
	defer rows.Close()

	var executions []Execution
	for rows.Next() {
		var e Execution
		var completedAt sql.NullString
		if err := rows.Scan(
			&e.ID, &e.Workspace, &e.Agent, &e.Role, &e.Status,
			&e.Task, &e.Locale, &e.Summary, &e.TokensUsed, &e.Error,
			&e.StartedAt, &completedAt,
		); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		if completedAt.Valid {
			e.CompletedAt = completedAt.String
		}
		executions = append(executions, e)
	}
	return executions, rows.Err()
}

// EventFilter controls which events to return.
type EventFilter struct {
	ExecutionID   string
	WorkspaceSlug string
	EventType     string
	Limit         int
}

// ListEvents returns events matching the filter.
func (s *PostgresExecutionStore) ListEvents(ctx context.Context, filter EventFilter) ([]AgenticEvent, error) {
	query := `SELECT execution_id, workspace_slug, agent, role, event_type, data, created_at
		FROM agentic_events WHERE 1=1`
	var args []any
	argN := 1

	if filter.ExecutionID != "" {
		query += fmt.Sprintf(" AND execution_id = $%d", argN)
		args = append(args, filter.ExecutionID)
		argN++
	}
	if filter.WorkspaceSlug != "" {
		query += fmt.Sprintf(" AND workspace_slug = $%d", argN)
		args = append(args, filter.WorkspaceSlug)
		argN++
	}
	if filter.EventType != "" {
		query += fmt.Sprintf(" AND event_type = $%d", argN)
		args = append(args, filter.EventType)
		argN++
	}

	query += " ORDER BY created_at DESC"

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []AgenticEvent
	for rows.Next() {
		var ev AgenticEvent
		var eventType string
		var dataJSON []byte
		var ts string
		if err := rows.Scan(
			&ev.ExecutionID, &ev.Workspace, &ev.Agent, &ev.Role,
			&eventType, &dataJSON, &ts,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		ev.Type = AgenticEventType(eventType)
		ev.Timestamp = ts
		if len(dataJSON) > 0 {
			_ = json.Unmarshal(dataJSON, &ev.Data)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// intFromData extracts an integer from a map[string]any, handling float64 (JSON default).
func intFromData(data map[string]any, key string) int {
	if v, ok := data[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return 0
}
