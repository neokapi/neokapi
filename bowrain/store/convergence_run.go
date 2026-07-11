package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/id"
)

// Convergence-run lifecycle states (strategy 2026-07-kapi-up doc 03). The
// terminal states ARE the core/convergence wire values — a run row and the
// stream's done event report the same string, so they are defined once.
const (
	ConvergenceRunRunning   = "running" // no wire equivalent: a run is only "running" server-side
	ConvergenceRunConverged = convergence.RunConverged
	ConvergenceRunParked    = convergence.RunParked
	ConvergenceRunCanceled  = convergence.RunCanceled
	ConvergenceRunFailed    = convergence.RunFailed
)

// ConvergenceRun is one server-side goal-seeking reconciliation of a project
// toward its ship gates: the venue-neutral core/convergence.Loop driven with
// server-backed IO. Standing is the per-locale rollup derived from the run's
// convergence.Event stream; every emitted event is persisted separately for
// SSE replay (ConvergenceRunEvents).
type ConvergenceRun struct {
	ID            string                      `json:"id"`
	ProjectID     string                      `json:"project_id"`
	Trigger       string                      `json:"trigger"` // cli | push | manual
	State         string                      `json:"state"`
	Passes        int                         `json:"passes"`
	Standing      []ConvergenceLocaleStanding `json:"standing,omitempty"`
	FailingChecks int                         `json:"failing_checks"`
	// Error carries a terminal failure reason (state=failed/canceled), e.g.
	// "interrupted by server restart" or the loop's error. Empty otherwise.
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// ConvergenceLocaleStanding is one locale's rollup within a run. It is the
// framework's standing type: the run row persists exactly what
// core/convergence.Standing folds out of the event stream.
type ConvergenceLocaleStanding = convergence.LocaleStanding

// ConvergenceRunStore persists convergence runs and their event streams. A
// single store serves both PostgreSQL and SQLite: SQLite-compatible `$N`
// placeholders and TEXT timestamps (RFC3339Nano UTC, chronological under
// ORDER BY) scan identically on both drivers.
type ConvergenceRunStore struct {
	db *sql.DB
}

// NewConvergenceRunStore builds a store over a shared *sql.DB (PostgreSQL or
// SQLite).
func NewConvergenceRunStore(db *sql.DB) *ConvergenceRunStore {
	return &ConvergenceRunStore{db: db}
}

const rfc3339Nano = time.RFC3339Nano

// CreateRun inserts a new run, defaulting ID, CreatedAt, and State.
func (s *ConvergenceRunStore) CreateRun(ctx context.Context, run *ConvergenceRun) error {
	if run.ID == "" {
		run.ID = id.New()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	if run.State == "" {
		run.State = ConvergenceRunRunning
	}
	standing, _ := json.Marshal(run.Standing)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO convergence_runs
			(id, project_id, trigger, state, passes, standing, failing_checks, error, created_at, finished_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		run.ID, run.ProjectID, run.Trigger, run.State, run.Passes,
		string(standing), run.FailingChecks, run.Error, run.CreatedAt.UTC().Format(rfc3339Nano), finishedText(run.FinishedAt))
	if err != nil {
		return fmt.Errorf("create convergence run: %w", err)
	}
	return nil
}

// ErrActiveRunExists is returned by CreateRun when the one-running-run-per-
// project unique constraint rejects a second concurrent run. Callers treat it
// as "join the existing run" rather than an error.
var ErrActiveRunExists = errors.New("a convergence run is already active for this project")

// CreateRunGuarded inserts a run and maps the unique-constraint violation from
// the partial index (one running run per project) to ErrActiveRunExists, so a
// racing starter can fall back to the in-flight run. The mapping is driver-
// agnostic: on any insert error it re-checks ActiveRun and, if one now exists,
// reports the conflict.
func (s *ConvergenceRunStore) CreateRunGuarded(ctx context.Context, run *ConvergenceRun) error {
	if err := s.CreateRun(ctx, run); err != nil {
		if active, aerr := s.ActiveRun(ctx, run.ProjectID); aerr == nil && active != nil {
			return ErrActiveRunExists
		}
		return err
	}
	return nil
}

// UpdateRun updates the mutable fields of a run (state, passes, standing,
// failing checks, finished_at).
func (s *ConvergenceRunStore) UpdateRun(ctx context.Context, run *ConvergenceRun) error {
	standing, _ := json.Marshal(run.Standing)
	_, err := s.db.ExecContext(ctx,
		`UPDATE convergence_runs
			SET state=$1, passes=$2, standing=$3, failing_checks=$4, error=$5, finished_at=$6
			WHERE id=$7`,
		run.State, run.Passes, string(standing), run.FailingChecks, run.Error, finishedText(run.FinishedAt), run.ID)
	if err != nil {
		return fmt.Errorf("update convergence run: %w", err)
	}
	return nil
}

// FailInterruptedRuns marks every run still in 'running' as failed with the
// given reason — the startup sweep that reconciles zombie runs left behind by a
// crash or restart (their in-process loop is gone, so they would otherwise
// block the one-run guard forever). Returns how many were swept. Runs it before
// the orchestrator accepts new work.
func (s *ConvergenceRunStore) FailInterruptedRuns(ctx context.Context, reason string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE convergence_runs SET state=$1, error=$2, finished_at=$3 WHERE state=$4`,
		ConvergenceRunFailed, reason, time.Now().UTC().Format(rfc3339Nano), ConvergenceRunRunning)
	if err != nil {
		return 0, fmt.Errorf("sweep interrupted convergence runs: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// GetRun retrieves a run by ID.
func (s *ConvergenceRunStore) GetRun(ctx context.Context, runID string) (*ConvergenceRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, trigger, state, passes, standing, failing_checks, error, created_at, finished_at
		 FROM convergence_runs WHERE id = $1`, runID)
	return scanConvergenceRun(row)
}

// ListRuns returns a project's runs, newest first. limit <= 0 uses 20.
func (s *ConvergenceRunStore) ListRuns(ctx context.Context, projectID string, limit int) ([]*ConvergenceRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, trigger, state, passes, standing, failing_checks, error, created_at, finished_at
		 FROM convergence_runs WHERE project_id = $1 ORDER BY created_at DESC LIMIT $2`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list convergence runs: %w", err)
	}
	defer rows.Close()
	var result []*ConvergenceRun
	for rows.Next() {
		r, err := scanConvergenceRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ActiveRun returns the project's currently-running run, or (nil, nil) when
// none is in flight — the guard a start uses to avoid a second concurrent run.
func (s *ConvergenceRunStore) ActiveRun(ctx context.Context, projectID string) (*ConvergenceRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, trigger, state, passes, standing, failing_checks, error, created_at, finished_at
		 FROM convergence_runs WHERE project_id = $1 AND state = $2 ORDER BY created_at DESC LIMIT 1`,
		projectID, ConvergenceRunRunning)
	run, err := scanConvergenceRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

// AppendEvent persists one convergence.Event payload at the next sequence
// number for the run and returns that sequence. The orchestrator's Emitter
// serializes emissions, so the read-then-insert of the next seq has a single
// writer per run.
func (s *ConvergenceRunStore) AppendEvent(ctx context.Context, runID string, payload []byte) (int, error) {
	var next int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), -1) + 1 FROM convergence_run_events WHERE run_id = $1`, runID).Scan(&next); err != nil {
		return 0, fmt.Errorf("next event seq: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO convergence_run_events (run_id, seq, payload) VALUES ($1, $2, $3)`,
		runID, next, string(payload)); err != nil {
		return 0, fmt.Errorf("append convergence run event: %w", err)
	}
	return next, nil
}

// ListEvents returns a run's persisted event payloads at or after fromSeq, in
// order — the SSE replay source.
func (s *ConvergenceRunStore) ListEvents(ctx context.Context, runID string, fromSeq int) ([]int, [][]byte, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, payload FROM convergence_run_events WHERE run_id = $1 AND seq >= $2 ORDER BY seq`,
		runID, fromSeq)
	if err != nil {
		return nil, nil, fmt.Errorf("list convergence run events: %w", err)
	}
	defer rows.Close()
	var seqs []int
	var payloads [][]byte
	for rows.Next() {
		var seq int
		var payload string
		if err := rows.Scan(&seq, &payload); err != nil {
			return nil, nil, err
		}
		seqs = append(seqs, seq)
		payloads = append(payloads, []byte(payload))
	}
	return seqs, payloads, rows.Err()
}

// DeleteRunsOlderThan removes runs (and cascading events) created before the
// cutoff, returning how many runs were removed.
func (s *ConvergenceRunStore) DeleteRunsOlderThan(ctx context.Context, age time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-age).Format(rfc3339Nano)
	_, _ = s.db.ExecContext(ctx,
		`DELETE FROM convergence_run_events WHERE run_id IN
			(SELECT id FROM convergence_runs WHERE created_at < $1)`, cutoff)
	res, err := s.db.ExecContext(ctx, `DELETE FROM convergence_runs WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old convergence runs: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// finishedText renders an optional finish time for storage (” when unset).
func finishedText(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(rfc3339Nano)
}

func scanConvergenceRun(row scannable) (*ConvergenceRun, error) {
	var r ConvergenceRun
	var standing, createdStr string
	var finishedStr sql.NullString
	if err := row.Scan(&r.ID, &r.ProjectID, &r.Trigger, &r.State, &r.Passes,
		&standing, &r.FailingChecks, &r.Error, &createdStr, &finishedStr); err != nil {
		return nil, err
	}
	if standing != "" && standing != "{}" {
		if err := json.Unmarshal([]byte(standing), &r.Standing); err != nil {
			return nil, fmt.Errorf("unmarshal run standing for %s: %w", r.ID, err)
		}
	}
	if createdStr != "" {
		r.CreatedAt, _ = time.Parse(rfc3339Nano, createdStr)
	}
	if finishedStr.Valid && finishedStr.String != "" {
		if t, err := time.Parse(rfc3339Nano, finishedStr.String); err == nil {
			tu := t.UTC()
			r.FinishedAt = &tu
		}
	}
	return &r, nil
}
