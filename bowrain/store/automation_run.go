package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/id"
)

// ---------------------------------------------------------------------------
// Model types (Bowrain AD-013)
// ---------------------------------------------------------------------------

// RunStatus tracks the lifecycle of an automation run.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusPartial   RunStatus = "partial" // some steps succeeded, some failed
)

// StepStatus tracks the lifecycle of a single step within a run.
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
)

// AutomationRun groups all automation actions triggered by a single event.
type AutomationRun struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	TriggerType string            `json:"trigger_type"`
	TriggerID   string            `json:"trigger_id"`
	TriggerData map[string]string `json:"trigger_data"`
	Status      RunStatus         `json:"status"`
	StepCount   int               `json:"step_count"`
	DoneCount   int               `json:"done_count"`
	Error       string            `json:"error,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	EndedAt     *time.Time        `json:"ended_at,omitempty"`
}

// AutomationStep represents a single automation action execution within a run.
type AutomationStep struct {
	ID         string            `json:"id"`
	RunID      string            `json:"run_id"`
	RuleName   string            `json:"rule_name"`
	ActionType string            `json:"action_type"`
	Status     StepStatus        `json:"status"`
	Config     map[string]string `json:"config,omitempty"`
	JobIDs     []string          `json:"job_ids,omitempty"`
	TaskIDs    []string          `json:"task_ids,omitempty"`
	TotalJobs  int               `json:"total_jobs"`
	DoneJobs   int               `json:"done_jobs"`
	Error      string            `json:"error,omitempty"`
	StartedAt  time.Time         `json:"started_at"`
	EndedAt    *time.Time        `json:"ended_at,omitempty"`
}

// AutomationLog is a structured log entry attached to a step.
type AutomationLog struct {
	ID        string            `json:"id"`
	StepID    string            `json:"step_id"`
	RunID     string            `json:"run_id"`
	Level     string            `json:"level"` // "info", "warn", "error"
	Message   string            `json:"message"`
	Data      map[string]string `json:"data,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

// AutomationRunStore persists automation runs, steps, and logs.
type AutomationRunStore struct {
	db *sql.DB
}

// NewAutomationRunStore creates a PostgreSQL-backed AutomationRunStore.
func NewAutomationRunStore(db *sql.DB) *AutomationRunStore {
	return &AutomationRunStore{db: db}
}

// ---------------------------------------------------------------------------
// Run CRUD
// ---------------------------------------------------------------------------

// CreateRun inserts a new automation run.
func (s *AutomationRunStore) CreateRun(ctx context.Context, run *AutomationRun) error {
	if run.ID == "" {
		run.ID = id.New()
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	if run.Status == "" {
		run.Status = RunStatusPending
	}

	triggerData, _ := json.Marshal(run.TriggerData)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO automation_runs
			(id, project_id, trigger_type, trigger_id, trigger_data, status, step_count, done_count, error, started_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		run.ID, run.ProjectID, run.TriggerType, run.TriggerID,
		string(triggerData), string(run.Status), run.StepCount, run.DoneCount,
		run.Error, run.StartedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create automation run: %w", err)
	}
	return nil
}

// GetRun retrieves an automation run by ID.
func (s *AutomationRunStore) GetRun(ctx context.Context, runID string) (*AutomationRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, trigger_type, trigger_id, trigger_data,
			status, step_count, done_count, error, started_at, ended_at
		 FROM automation_runs WHERE id = $1`, runID)
	return scanRun(row)
}

// ListRuns returns automation runs for a project, newest first.
func (s *AutomationRunStore) ListRuns(ctx context.Context, projectID, status string, limit, offset int) ([]*AutomationRun, error) {
	var args []any
	argN := 1
	where := []string{fmt.Sprintf("project_id = $%d", argN)}
	args = append(args, projectID)
	argN++

	if status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argN))
		args = append(args, status)
		argN++
	}

	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit, offset)

	var qb strings.Builder
	qb.WriteString(`SELECT id, project_id, trigger_type, trigger_id, trigger_data,
		status, step_count, done_count, error, started_at, ended_at
		FROM automation_runs WHERE `)
	qb.WriteString(strings.Join(where, " AND "))
	qb.WriteString(fmt.Sprintf(" ORDER BY started_at DESC LIMIT $%d OFFSET $%d", argN, argN+1))
	q := qb.String()

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list automation runs: %w", err)
	}
	defer rows.Close()

	var result []*AutomationRun
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// UpdateRunStatus updates the run status and optionally sets error and ended_at.
func (s *AutomationRunStore) UpdateRunStatus(ctx context.Context, runID string, status RunStatus, errMsg string) error {
	var endedAt any
	if status == RunStatusCompleted || status == RunStatusFailed || status == RunStatusPartial {
		endedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_runs SET status = $1, error = $2, ended_at = $3 WHERE id = $4`,
		string(status), errMsg, endedAt, runID)
	return err
}

// IncrementDoneCount atomically increments the done step count and recomputes run status.
func (s *AutomationRunStore) IncrementDoneCount(ctx context.Context, runID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_runs SET done_count = done_count + 1 WHERE id = $1`, runID)
	return err
}

// ---------------------------------------------------------------------------
// Step CRUD
// ---------------------------------------------------------------------------

// CreateStep inserts a new automation step.
func (s *AutomationRunStore) CreateStep(ctx context.Context, step *AutomationStep) error {
	if step.ID == "" {
		step.ID = id.New()
	}
	if step.StartedAt.IsZero() {
		step.StartedAt = time.Now().UTC()
	}
	if step.Status == "" {
		step.Status = StepStatusPending
	}

	config, _ := json.Marshal(step.Config)
	jobIDs, _ := json.Marshal(step.JobIDs)
	taskIDs, _ := json.Marshal(step.TaskIDs)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO automation_steps
			(id, run_id, rule_name, action_type, status, config, job_ids, task_ids, total_jobs, done_jobs, error, started_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		step.ID, step.RunID, step.RuleName, step.ActionType,
		string(step.Status), string(config), string(jobIDs), string(taskIDs),
		step.TotalJobs, step.DoneJobs, step.Error, step.StartedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create automation step: %w", err)
	}

	// Increment step count on the parent run.
	_, _ = s.db.ExecContext(ctx,
		`UPDATE automation_runs SET step_count = step_count + 1, status = $1 WHERE id = $2`,
		string(RunStatusRunning), step.RunID)

	return nil
}

// GetStep retrieves a single step by ID.
func (s *AutomationRunStore) GetStep(ctx context.Context, stepID string) (*AutomationStep, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, run_id, rule_name, action_type, status, config,
			job_ids, task_ids, total_jobs, done_jobs, error, started_at, ended_at
		 FROM automation_steps WHERE id = $1`, stepID)
	return scanStep(row)
}

// ListSteps returns all steps for a run.
func (s *AutomationRunStore) ListSteps(ctx context.Context, runID string) ([]*AutomationStep, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, rule_name, action_type, status, config,
			job_ids, task_ids, total_jobs, done_jobs, error, started_at, ended_at
		 FROM automation_steps WHERE run_id = $1 ORDER BY started_at`, runID)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	defer rows.Close()

	var result []*AutomationStep
	for rows.Next() {
		step, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, step)
	}
	return result, rows.Err()
}

// UpdateStepStatus updates a step's status and optionally error.
func (s *AutomationRunStore) UpdateStepStatus(ctx context.Context, stepID string, status StepStatus, errMsg string) error {
	var endedAt any
	if status == StepStatusCompleted || status == StepStatusFailed || status == StepStatusSkipped {
		endedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_steps SET status = $1, error = $2, ended_at = $3 WHERE id = $4`,
		string(status), errMsg, endedAt, stepID)
	return err
}

// RegisterStepJobs records spawned job IDs on a step.
func (s *AutomationRunStore) RegisterStepJobs(ctx context.Context, stepID string, jobIDs []string) error {
	data, _ := json.Marshal(jobIDs)
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_steps SET job_ids = $1, total_jobs = $2 WHERE id = $3`,
		string(data), len(jobIDs), stepID)
	return err
}

// RegisterStepTasks records created task IDs on a step.
func (s *AutomationRunStore) RegisterStepTasks(ctx context.Context, stepID string, taskIDs []string) error {
	data, _ := json.Marshal(taskIDs)
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_steps SET task_ids = $1 WHERE id = $2`,
		string(data), stepID)
	return err
}

// UpdateStepJobProgress updates the completed job count for a step.
func (s *AutomationRunStore) UpdateStepJobProgress(ctx context.Context, stepID string, doneJobs int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_steps SET done_jobs = $1 WHERE id = $2`,
		doneJobs, stepID)
	return err
}

// ---------------------------------------------------------------------------
// Log CRUD
// ---------------------------------------------------------------------------

// AppendLogs inserts a batch of log entries.
func (s *AutomationRunStore) AppendLogs(ctx context.Context, logs []AutomationLog) error {
	if len(logs) == 0 {
		return nil
	}
	for i := range logs {
		if logs[i].ID == "" {
			logs[i].ID = id.New()
		}
		if logs[i].Timestamp.IsZero() {
			logs[i].Timestamp = time.Now().UTC()
		}
		data, _ := json.Marshal(logs[i].Data)
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO automation_logs (id, step_id, run_id, level, message, data, timestamp)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			logs[i].ID, logs[i].StepID, logs[i].RunID, logs[i].Level,
			logs[i].Message, string(data), logs[i].Timestamp.Format(time.RFC3339Nano))
		if err != nil {
			return fmt.Errorf("append automation log: %w", err)
		}
	}
	return nil
}

// ListLogs returns logs for a step, oldest first.
func (s *AutomationRunStore) ListLogs(ctx context.Context, stepID string, limit int) ([]AutomationLog, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, step_id, run_id, level, message, data, timestamp
		 FROM automation_logs WHERE step_id = $1 ORDER BY timestamp LIMIT $2`,
		stepID, limit)
	if err != nil {
		return nil, fmt.Errorf("list logs: %w", err)
	}
	defer rows.Close()

	var result []AutomationLog
	for rows.Next() {
		var l AutomationLog
		var dataJSON string
		if err := rows.Scan(&l.ID, &l.StepID, &l.RunID, &l.Level, &l.Message, &dataJSON, &l.Timestamp); err != nil {
			return nil, err
		}
		if dataJSON != "" && dataJSON != "{}" {
			if err := json.Unmarshal([]byte(dataJSON), &l.Data); err != nil {
				return nil, fmt.Errorf("unmarshal log data for %s: %w", l.ID, err)
			}
		}
		result = append(result, l)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Pending pushes (cross-instance push tracking, #169)
// ---------------------------------------------------------------------------

// PendingPush records a push that needs automation tracking.
// Written by any server instance; polled by the leader's PushCompletionTracker.
type PendingPush struct {
	PushID    string
	ProjectID string
	Items     string
	WsSlug    string
	Actor     string
	CreatedAt time.Time
}

// InsertPendingPush records a push for the tracker to pick up.
func (s *AutomationRunStore) InsertPendingPush(ctx context.Context, pp *PendingPush) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pending_pushes (push_id, project_id, items, ws_slug, actor, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (push_id) DO NOTHING`,
		pp.PushID, pp.ProjectID, pp.Items, pp.WsSlug, pp.Actor, now)
	return err
}

// ListPendingPushes returns all unprocessed pushes.
func (s *AutomationRunStore) ListPendingPushes(ctx context.Context) ([]PendingPush, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT push_id, project_id, items, ws_slug, actor, created_at FROM pending_pushes ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PendingPush
	for rows.Next() {
		var pp PendingPush
		if err := rows.Scan(&pp.PushID, &pp.ProjectID, &pp.Items, &pp.WsSlug, &pp.Actor, &pp.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, pp)
	}
	return result, rows.Err()
}

// DeletePendingPush removes a processed push.
func (s *AutomationRunStore) DeletePendingPush(ctx context.Context, pushID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pending_pushes WHERE push_id = $1`, pushID)
	return err
}

// ---------------------------------------------------------------------------
// Retention
// ---------------------------------------------------------------------------

// DeleteRunsOlderThan removes automation runs (and cascading steps/logs) older than the given duration.
func (s *AutomationRunStore) DeleteRunsOlderThan(ctx context.Context, age time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-age).Format(time.RFC3339Nano)

	// Delete logs for old runs first.
	_, _ = s.db.ExecContext(ctx,
		`DELETE FROM automation_logs WHERE run_id IN
			(SELECT id FROM automation_runs WHERE started_at < $1)`, cutoff)

	// Steps cascade via FK on runs.
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM automation_runs WHERE started_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old runs: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

type scannable interface {
	Scan(dest ...any) error
}

func scanRun(row scannable) (*AutomationRun, error) {
	var r AutomationRun
	var triggerData, status string
	var endedAt sql.NullTime

	if err := row.Scan(&r.ID, &r.ProjectID, &r.TriggerType, &r.TriggerID,
		&triggerData, &status, &r.StepCount, &r.DoneCount, &r.Error,
		&r.StartedAt, &endedAt); err != nil {
		return nil, fmt.Errorf("scan automation run: %w", err)
	}

	r.Status = RunStatus(status)
	if endedAt.Valid {
		t := endedAt.Time.UTC()
		r.EndedAt = &t
	}
	if triggerData != "" && triggerData != "{}" {
		if err := json.Unmarshal([]byte(triggerData), &r.TriggerData); err != nil {
			return nil, fmt.Errorf("unmarshal trigger data for run %s: %w", r.ID, err)
		}
	}
	return &r, nil
}

func scanStep(row scannable) (*AutomationStep, error) {
	var step AutomationStep
	var config, jobIDs, taskIDs, status string
	var endedAt sql.NullTime

	if err := row.Scan(&step.ID, &step.RunID, &step.RuleName, &step.ActionType,
		&status, &config, &jobIDs, &taskIDs,
		&step.TotalJobs, &step.DoneJobs, &step.Error,
		&step.StartedAt, &endedAt); err != nil {
		return nil, fmt.Errorf("scan automation step: %w", err)
	}

	step.Status = StepStatus(status)
	if endedAt.Valid {
		t := endedAt.Time.UTC()
		step.EndedAt = &t
	}
	if config != "" && config != "{}" {
		if err := json.Unmarshal([]byte(config), &step.Config); err != nil {
			return nil, fmt.Errorf("unmarshal step config for %s: %w", step.ID, err)
		}
	}
	if jobIDs != "" && jobIDs != "[]" {
		if err := json.Unmarshal([]byte(jobIDs), &step.JobIDs); err != nil {
			return nil, fmt.Errorf("unmarshal step job IDs for %s: %w", step.ID, err)
		}
	}
	if taskIDs != "" && taskIDs != "[]" {
		if err := json.Unmarshal([]byte(taskIDs), &step.TaskIDs); err != nil {
			return nil, fmt.Errorf("unmarshal step task IDs for %s: %w", step.ID, err)
		}
	}
	return &step, nil
}
