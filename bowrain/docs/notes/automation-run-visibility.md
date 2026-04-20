---
sidebar_position: 19
title: Automation Run Visibility
---

# Automation Run Visibility

Implementation details for [AD-035](/docs/ad/035-automation-run-visibility).

## Database Schema

New migration in `platform/store/migrations_pg.go` (and `migrations.go` for SQLite):

```sql
CREATE TABLE automation_runs (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    trigger_type TEXT NOT NULL,
    trigger_id   TEXT NOT NULL DEFAULT '',
    trigger_data JSONB NOT NULL DEFAULT '{}',
    status       TEXT NOT NULL DEFAULT 'pending',
    step_count   INT NOT NULL DEFAULT 0,
    done_count   INT NOT NULL DEFAULT 0,
    error        TEXT NOT NULL DEFAULT '',
    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at     TIMESTAMPTZ
);
CREATE INDEX idx_automation_runs_project ON automation_runs(project_id, started_at DESC);

CREATE TABLE automation_steps (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES automation_runs(id) ON DELETE CASCADE,
    rule_name   TEXT NOT NULL DEFAULT '',
    action_type TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    config      JSONB NOT NULL DEFAULT '{}',
    job_ids     JSONB NOT NULL DEFAULT '[]',
    task_ids    JSONB NOT NULL DEFAULT '[]',
    total_jobs  INT NOT NULL DEFAULT 0,
    done_jobs   INT NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT '',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at    TIMESTAMPTZ
);
CREATE INDEX idx_automation_steps_run ON automation_steps(run_id);

CREATE TABLE automation_logs (
    id        TEXT PRIMARY KEY,
    step_id   TEXT NOT NULL,
    run_id    TEXT NOT NULL,
    level     TEXT NOT NULL DEFAULT 'info',
    message   TEXT NOT NULL,
    data      JSONB NOT NULL DEFAULT '{}',
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_automation_logs_step ON automation_logs(step_id, timestamp);
CREATE INDEX idx_automation_logs_run ON automation_logs(run_id, timestamp);
```

## AutomationRunStore Interface

```go
// In platform/event/automation_run_store.go

type AutomationRunStore interface {
    CreateRun(ctx context.Context, run *AutomationRun) error
    GetRun(ctx context.Context, id string) (*AutomationRun, error)
    ListRuns(ctx context.Context, projectID string, status string, limit, offset int) ([]*AutomationRun, error)
    UpdateRunStatus(ctx context.Context, id string, status RunStatus, err string) error
    IncrementDoneCount(ctx context.Context, id string) error

    CreateStep(ctx context.Context, step *AutomationStep) error
    GetStep(ctx context.Context, id string) (*AutomationStep, error)
    ListSteps(ctx context.Context, runID string) ([]*AutomationStep, error)
    UpdateStepStatus(ctx context.Context, id string, status StepStatus, err string) error
    RegisterStepJobs(ctx context.Context, stepID string, jobIDs []string) error
    RegisterStepTasks(ctx context.Context, stepID string, taskIDs []string) error
    UpdateStepJobProgress(ctx context.Context, stepID string, doneJobs int) error

    AppendLogs(ctx context.Context, logs []AutomationLog) error
    ListLogs(ctx context.Context, stepID string, limit int, afterCursor string) ([]AutomationLog, error)
}
```

## AutomationRunManager

```go
// In platform/event/automation_run_manager.go

type AutomationRunManager struct {
    store    AutomationRunStore
    executor ActionExecutor
    bus      platev.EventBus

    mu       sync.Mutex
    eventRuns map[string]string // event.ID → run.ID (debounce window)
}

func (m *AutomationRunManager) Execute(action AutomationAction, ev platev.Event) error {
    // 1. Find or create run for this event.
    runID := m.getOrCreateRun(ev)

    // 2. Create step.
    step := &AutomationStep{
        ID:         id.New(),
        RunID:      runID,
        RuleName:   action.ruleName, // needs to be passed through
        ActionType: action.Type,
        Status:     StepStatusRunning,
        Config:     action.Config,
        StartedAt:  time.Now().UTC(),
    }
    m.store.CreateStep(ctx, step)

    // 3. Execute the real action with step context.
    err := m.executor(action, ev, step.ID) // executor gains stepID param

    // 4. Update step status.
    if err != nil {
        m.store.UpdateStepStatus(ctx, step.ID, StepStatusFailed, err.Error())
    }
    // For async actions (auto_translate, auto_extract), status stays "running"
    // until StepCompletionTracker detects all jobs are done.

    return err
}
```

## StepCompletionTracker

Generalizes `PushCompletionTracker` for per-step job tracking:

```go
// In platform/event/step_completion_tracker.go

type StepCompletionTracker struct {
    runStore     AutomationRunStore
    jobStore     jobs.JobStore
    extractStore jobs.ExtractionJobStore

    mu      sync.Mutex
    pending map[string]*pendingStep // stepID → tracking state
}

type pendingStep struct {
    runID   string
    jobIDs  []string
    isExtraction bool
}
```

Poll loop (5s interval):

1. For each pending step, query job statuses
2. Update `done_jobs` on the step
3. When all jobs complete → update step status → increment run done_count
4. When all steps done → update run status

## Worker Log Integration (Phase 2)

Add `StepID` to job models:

```go
// In jobs/model.go
type TranslationJob struct {
    // ... existing fields ...
    StepID string `json:"step_id,omitempty"`
}
```

The `RunLogger` is created per-step and passed to the worker:

```go
type RunLogger struct {
    store  AutomationRunStore
    runID  string
    stepID string
    buf    []AutomationLog
    mu     sync.Mutex
}

func (l *RunLogger) Info(msg string, data map[string]string)
func (l *RunLogger) Warn(msg string, data map[string]string)
func (l *RunLogger) Error(msg string, data map[string]string)
func (l *RunLogger) Flush(ctx context.Context) error
```

## SSE Endpoint (Phase 2)

```go
// In platform/server/handlers_automation_runs.go

func (s *Server) HandleAutomationRunSSE(c echo.Context) error {
    runID := c.Param("runId")
    // Set SSE headers
    c.Response().Header().Set("Content-Type", "text/event-stream")
    c.Response().Header().Set("Cache-Control", "no-cache")

    // Register client, stream events until run completes or client disconnects
    // Events: step_started, step_progress, step_completed, log, run_completed
}
```

## Action Executor Changes

`doExecuteAction` in `platform/server/automation.go` gains a `stepID` parameter:

```go
func (s *Server) doExecuteAction(action event.AutomationAction, ev platev.Event, stepID string) error {
    switch action.Type {
    case "auto_translate":
        go s.triggerAutoTranslate(ctx, ev.ProjectID, itemNames, nil, pushID, wsSlug, stepID)
    case "auto_extract":
        go s.triggerAutoExtract(ctx, ev.ProjectID, itemNames, pushID, wsSlug, stepID)
    case "create_review_tasks":
        go s.createReviewTasks(ctx, action, ev, stepID)
    // ...
    }
}
```

`triggerAutoTranslate` records job IDs on the step after creating them:

```go
func (s *Server) triggerAutoTranslate(ctx context.Context, ..., stepID string) {
    var jobIDs []string
    for _, itemName := range itemNames {
        for _, locale := range locales {
            job := &jobs.TranslationJob{..., StepID: stepID}
            s.JobStore.CreateJob(ctx, job)
            s.JobQueue.Enqueue(ctx, job.ID)
            jobIDs = append(jobIDs, job.ID)
        }
    }
    if stepID != "" && s.AutomationRunStore != nil {
        s.AutomationRunStore.RegisterStepJobs(ctx, stepID, jobIDs)
    }
}
```

## REST API Endpoints

All registered under the workspace project route group:

```go
// In server.go registerWorkspaceContentRoutes
g.GET("/projects/:id/automation-runs", s.HandleListAutomationRuns)
g.GET("/projects/:id/automation-runs/:runId", s.HandleGetAutomationRun)
g.GET("/projects/:id/automation-runs/:runId/steps", s.HandleListAutomationRunSteps)
g.GET("/projects/:id/automation-runs/:runId/steps/:stepId/logs", s.HandleListStepLogs)
g.POST("/projects/:id/automation-runs/:runId/cancel", s.HandleCancelAutomationRun)
// Phase 2:
g.GET("/projects/:id/automation-runs/:runId/events", s.HandleAutomationRunSSE)
```

## Files to Create

| File                                            | Purpose                                   |
| ----------------------------------------------- | ----------------------------------------- |
| `platform/event/automation_run.go`              | Model types (Run, Step, Log, statuses)    |
| `platform/event/automation_run_store.go`        | Store interface                           |
| `platform/event/automation_run_store_sqlite.go` | SQLite implementation                     |
| `platform/event/automation_run_store_pg.go`     | PostgreSQL implementation                 |
| `platform/event/automation_run_manager.go`      | Lifecycle management, run/step creation   |
| `platform/event/step_completion_tracker.go`     | Polls job stores, updates step/run status |
| `platform/server/handlers_automation_runs.go`   | REST handlers                             |

## Files to Modify

| File                                | Change                                              |
| ----------------------------------- | --------------------------------------------------- |
| `platform/store/migrations_pg.go`   | Add migration for new tables                        |
| `platform/store/migrations.go`      | SQLite equivalent                                   |
| `platform/server/automation.go`     | Pass stepID to action handlers, register jobs/tasks |
| `platform/server/server.go`         | Wire RunManager, StepCompletionTracker, routes      |
| `platform/jobs/model.go`            | Add StepID to TranslationJob (Phase 2)              |
| `platform/jobs/extraction_model.go` | Add StepID to ExtractionJob (Phase 2)               |
| `platform/jobs/worker.go`           | Use RunLogger (Phase 2)                             |

## Implementation Sequence

### Phase 1: Core (REST + polling)

1. Model types + store interface + SQLite implementation
2. AutomationRunManager
3. StepCompletionTracker
4. REST handlers
5. Integration in server.go and automation.go
6. Tests

### Phase 2: Logs + SSE

7. AutomationLog + RunLogger
8. StepID on jobs
9. Worker log integration
10. SSE endpoint + automationRunHub

### Phase 3: UI

11. Run list page
12. Run detail with step graph
13. Per-step log viewer
14. Real-time updates
