---
id: 035-automation-run-visibility
sidebar_position: 35
title: "AD-035: Automation Run Visibility"
---
# AD-035: Automation Run Visibility

## Context

When a developer pushes content to Bowrain, automations fire: AI translation, entity extraction, task creation, notifications. Today these automations are invisible to users. There is no indication that automations are running, no progress feedback, and no way to inspect what happened after they complete. Users see content appear "magically" or — worse — wonder why nothing is happening.

The automation system ([AD-011](./011-automation.md)) has the right event-driven architecture, and the translator workflow ([AD-034](./034-translator-workflow.md)) chains multiple automations together. But without visibility, debugging and confidence-building are impossible.

The goal is **GitHub Actions-level visibility**: a run list with status badges, drill-down into individual steps, real-time progress, and structured logs.

## Decision

### Concept: Automation Runs

An **AutomationRun** groups everything that happens in response to a single triggering event. When `push.completed` fires and matches three rules (auto-translate, auto-extract, create-review-tasks), one Run is created with three Steps.

```
GitHub Actions               Bowrain Automation Runs
─────────────               ──────────────────────
Workflow run             →   AutomationRun (triggered by push/event)
Job (build, test)        →   AutomationStep (auto_translate, auto_extract)
Step within a job        →   Individual TranslationJob or ExtractionJob
Step log output          →   Structured AutomationLog entries
Run status badge         →   Aggregate status across all steps
Live log streaming       →   SSE endpoint for progress + logs
```

### Data Model

#### AutomationRun

Created when an event triggers one or more automation rules.

```go
type AutomationRun struct {
    ID          string
    ProjectID   string
    TriggerType string            // event type, e.g. "connector.push.completed"
    TriggerID   string            // event ID
    TriggerData map[string]string // snapshot of event data (push_id, items, actor)
    Status      RunStatus         // pending → running → completed/failed/partial
    StepCount   int
    DoneCount   int
    Error       string
    StartedAt   time.Time
    EndedAt     *time.Time
}
```

Status values: `pending`, `running`, `completed`, `failed`, `partial` (some steps succeeded, some failed).

#### AutomationStep

Each matched rule execution within a run. Tracks spawned child entities.

```go
type AutomationStep struct {
    ID         string
    RunID      string
    RuleName   string            // "auto-translate-on-push"
    ActionType string            // "auto_translate", "create_review_tasks", etc.
    Status     StepStatus        // pending → running → completed/failed/skipped
    Config     map[string]string // action config snapshot
    JobIDs     []string          // translation/extraction job IDs spawned
    TaskIDs    []string          // task IDs created
    TotalJobs  int
    DoneJobs   int
    Error      string
    StartedAt  time.Time
    EndedAt    *time.Time
}
```

#### AutomationLog

Structured log entries attached to steps. Workers and action executors write these.

```go
type AutomationLog struct {
    ID        string
    StepID    string
    RunID     string
    Level     string // "info", "warn", "error"
    Message   string
    Data      map[string]string
    Timestamp time.Time
}
```

### AutomationRunManager

Wraps the existing `ActionExecutor` callback. The `AutomationEngine` continues to evaluate rules and call the executor per-action. The RunManager groups actions from the same event into a single Run using a brief debounce window.

```go
type AutomationRunManager struct {
    store    AutomationRunStore
    executor ActionExecutor // the real server.executeAutomationAction
    bus      EventBus
}
```

When `Execute(action, event)` is called:
1. Check if a Run exists for `event.ID`. If not, create one.
2. Create a Step for this action.
3. Call the real executor.
4. Update step status based on result.
5. For async actions (auto_translate, auto_extract), record spawned job IDs on the step.
6. A background goroutine finalizes the run when all steps complete.

### Step Completion Tracking

Async steps (auto_translate, auto_extract) spawn jobs that complete later. A `StepCompletionTracker` — generalizing the existing `PushCompletionTracker` pattern — polls job stores and updates step/run status.

The existing `PushCompletionTracker` continues to emit `push.automations.completed` for the translator workflow. The `StepCompletionTracker` adds per-step granularity for the visibility UI.

### Log Capture

A `RunLogger` passed through context to workers. It buffers log entries and flushes in batches to the `automation_logs` table.

Workers log at key milestones:
- "Processing {item_name} for {locale}" (info)
- "Translating blocks {start}-{end} of {total}" (info, per chunk)
- "Chunk completed: {tokens} tokens" (info)
- "Job completed: {total_blocks} blocks, {tokens_used} tokens" (info)
- "Translation failed: {error}" (error)

Jobs gain a `StepID` field linking them to their parent step for log routing.

### Real-Time Delivery

**Phase 1**: Polling via REST endpoints. The UI fetches run/step status on an interval.

**Phase 2**: SSE (Server-Sent Events) for live streaming.

```
GET /api/v1/workspaces/:ws/projects/:id/automation-runs/:runId/events
```

Streams: `step_started`, `step_progress`, `step_completed`, `step_failed`, `run_completed`, `log`.

SSE is preferred over WebSocket because it's simpler, auto-reconnects, and works through proxies. The existing notification WebSocket remains for user-scoped alerts.

### API Endpoints

```
# Run list (paginated)
GET  /projects/:id/automation-runs?status=running&limit=20

# Run detail
GET  /projects/:id/automation-runs/:runId

# Steps for a run
GET  /projects/:id/automation-runs/:runId/steps

# Logs for a step
GET  /projects/:id/automation-runs/:runId/steps/:stepId/logs?limit=100

# SSE stream (Phase 2)
GET  /projects/:id/automation-runs/:runId/events

# Cancel a run
POST /projects/:id/automation-runs/:runId/cancel
```

### UI

The UI renders runs as a timeline/graph similar to GitHub Actions:

**Run list page**: Status badge (green/red/blue spinner), trigger info ("Content pushed: en.json"), duration, step count. Auto-refreshes.

**Run detail page**: Step graph showing the execution flow. Each step is a card with:
- Status icon (spinner/checkmark/X)
- Action type label
- Progress bar (for async steps: 142/418 blocks)
- Duration
- Expandable to show spawned jobs and logs

**Log viewer**: Expandable section per step showing structured log entries with timestamps, levels, and metadata.

### Integration with Existing Systems

The `handleEvent` loop in `AutomationEngine` stays unchanged — it still calls the executor per matching action. The executor is now the `RunManager`, which creates runs/steps as a side effect before delegating to the real action handler.

The `PushCompletionTracker` coexists: it tracks push-level completion for the translator workflow, while the `StepCompletionTracker` tracks step-level completion for the visibility UI. In the future, the PushCompletionTracker could be replaced by a run-level completion event.

The `executeAutomationAction` function in `server/automation.go` gains a `stepID` parameter that it passes to `triggerAutoTranslate`, `triggerAutoExtract`, etc., so these functions can register spawned job IDs on the step.

## Implementation Phases

### Phase 1: Core Model + REST API
- Data model: `automation_runs`, `automation_steps` tables
- `AutomationRunStore` interface + PostgreSQL/SQLite implementations
- `AutomationRunManager` wrapping the action executor
- Step completion tracking via polling
- REST handlers for run list, detail, steps
- Integration in `server.go`

### Phase 2: Logs + Real-Time
- `automation_logs` table
- `RunLogger` for structured log capture in workers
- `StepID` field on jobs
- SSE endpoint for live run streaming
- Worker integration for translation and extraction logs

### Phase 3: UI
- Run list page with live status badges
- Run detail with step graph/timeline
- Per-step expandable log viewer
- Navigation from steps to tasks/jobs

### Phase 4: Polish
- Migrate from `automation_history` to runs
- Run cancellation (cancel pending jobs)
- Retention policy (auto-delete old runs)

## Alternatives Considered

- **Extend automation_history**: Add fields to `HistoryEntry` instead of new tables. Rejected because the run→step→log hierarchy doesn't fit a flat table. The grouping of multiple rule executions per event is the core innovation.

- **WebSocket for streaming**: Use the existing notification WebSocket. Rejected because it's user-scoped (notifications), not run-scoped (automation progress). Mixing concerns would complicate both systems.

- **Event-based step completion**: Workers emit events on job completion instead of polling. Cleaner but requires adding EventBus dependency to workers, which are intentionally decoupled. Deferred to Phase 2 as an optimization.

- **Full workflow engine (Temporal)**: Overkill for the current scale. The polling-based tracker is simple and sufficient. If Bowrain grows to need distributed workflow orchestration, Temporal could replace the tracker.

## Consequences

- Users gain complete visibility into what automations are doing and why.

- Debugging automation issues becomes self-service — no need to check server logs.

- The run→step→log hierarchy provides a natural drill-down path.

- Real-time progress (Phase 2) gives users confidence that "something is happening" after pushing content.

- The data model supports the translator workflow chain: a push triggers a run with auto-translate + auto-extract steps, which completes and triggers another run with create-review-tasks steps.

- Structured logs enable future features: log search, error aggregation, performance monitoring.

- The SSE approach keeps the architecture simple — no message broker, no persistent connections beyond active viewers.

- Backward compatible: the existing automation history endpoint remains for older clients.
