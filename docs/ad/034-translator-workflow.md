---
id: 034-translator-workflow
sidebar_position: 34
title: "AD-034: Translator Workflow"
---
# AD-034: Translator Workflow

## Context

When a developer pushes source content to Bowrain (via CLI sync or GitHub Action), several automations fire asynchronously: AI translation pre-fills target languages, entity/term extraction identifies new terminology. These automations are the "prep work" that should complete before human translators engage.

Today, there is no signal when prep work finishes. The automation system ([AD-011](./011-automation.md)) triggers jobs but does not track their collective completion. Translators have no way to know when content is ready for review. Tasks exist ([AD-032](./032-permissions-and-access-control.md)) but are only created manually. The agentic testing framework's persona agents bypass the workflow entirely — polling for untranslated blocks on a cron schedule rather than responding to tasks.

This creates a gap: **between automation completion and translator engagement, nothing happens.**

## Decision

### Design Principle: Automations All the Way Down

The translator workflow is not a separate system — it extends the existing automation engine with new events and new actions. Users configure the workflow through automation rules, the same mechanism that configures AI translation and term extraction.

Today's built-in automation rules:

| Rule | Trigger | Action |
|------|---------|--------|
| `auto-translate-on-push` | `push.completed` | Create translation jobs per (item, locale) |
| `auto-extract-on-push` | `push.completed` | Run entity/term extraction on pushed content |
| `auto-translate-new-locale` | `project.updated` | Translate all items for new locales |

New rules added by this AD:

| Rule | Trigger | Action |
|------|---------|--------|
| `create-review-tasks` | `push.automations.completed` | Create per-locale review/translate tasks |
| `create-source-review` | `push.automations.completed` | Create source review task (optional gate) |
| `fan-out-after-source-review` | `source.review.completed` | Create per-locale tasks after source review |

### Event Flow

```
CI push → push.completed (existing)
  → auto_translate (existing)
  → auto_extract (existing)
  → PushCompletionTracker monitors jobs
    → push.automations.completed (NEW)

Option A — direct fan-out:
  push.automations.completed → create_review_tasks
    → per-locale TaskReview, assigned to project members
    → task.assigned notifications

Option B — source review gate:
  push.automations.completed → create_source_review
    → single TaskSourceReview for source reviewer
    → reviewer completes task
      → source.review.completed (NEW)
        → create_review_tasks (same fan-out)
```

### PushCompletionTracker

A new server component that bridges the gap between individual job completion and collective push readiness. It subscribes to `push.completed` events and monitors all translation and extraction jobs spawned for each `push_id`.

**Behavior:**
- Polls `JobStore.ListJobsByPushID()` and `ExtractionJobStore.ListByPushID()` every 5 seconds
- When all jobs reach terminal status (completed or failed), emits `push.automations.completed`
- If the project has both `auto_translate` and `auto_extract` disabled (0 jobs), emits immediately
- Times out after 30 minutes, emitting with `status: timeout` to prevent indefinite waiting

The tracker runs alongside existing server components (ProgressTracker, DeadlineChecker, ActivityRecorder) and uses the same EventBus subscribe/publish pattern.

### New Event Types

```go
EventPushAutomationsCompleted EventType = "push.automations.completed"
EventSourceReviewCompleted    EventType = "source.review.completed"
```

`push.automations.completed` carries the original push data (`push_id`, `items`, `workspace_slug`, `project_id`) plus aggregated job status (`translation_status`, `extraction_status`).

`source.review.completed` is emitted when a `TaskSourceReview` task is completed, carrying the same push data forward.

### New Automation Actions

#### `create_review_tasks`

Creates per-locale tasks for project members with appropriate permissions.

**Algorithm:**
1. Load project to get target languages
2. Load project members via AuthStore
3. For each target locale, find members with matching language scope and `PermReview` or `PermTranslate` permission
4. Create one task per (locale, assignee) pair
5. If no member assigned to a locale, create an unassigned task and notify project admins
6. Dispatch `task.assigned` notification for each task

**Configuration** (via `AutomationAction.Config`):
- `mode`: `"review"` (default) or `"translate"` — controls whether tasks are `TaskReview` or `TaskTranslate`
- `priority`: task priority (default: `"normal"`)

#### `create_source_review`

Creates a single source review task before language fan-out. This prevents fixing the same source issue N times across target languages.

**Configuration:**
- `reviewer`: user ID (optional — falls back to first project member with `PermEditSource`)

The source reviewer inspects pushed content for placeholder correctness, terminology consistency, Do Not Translate (DNT) identification, and context notes. When the reviewer completes the task, `source.review.completed` fires and the automation chain continues.

### New Task Type

```go
TaskSourceReview TaskType = "source_review"
```

Added alongside existing types (`TaskTranslate`, `TaskReview`, `TaskReviewTerms`, `TaskFixQuality`, etc.). Source review tasks carry `push_id`, `items`, and `workspace_slug` in their `Data` map for traceability.

### Source Review Completion Hook

`HandleCompleteTask` emits `source.review.completed` when a `TaskSourceReview` task is completed. This event re-enters the automation engine, allowing rules like `source.review.completed → create_review_tasks` to fan out per-locale tasks.

### Review-Only vs. Translate Workflow

The `mode` config on `create_review_tasks` controls the workflow:

- **`mode: review`** (default): AI translation has already run. Tasks are `TaskReview`. The translator opens the editor and sees blocks pre-filled with AI translations. They accept, edit, or reject per block.
- **`mode: translate`**: AI translation is disabled or insufficient. Tasks are `TaskTranslate`. The translator works from source.

The editor already displays AI-translated blocks in the target column — no UI change is needed for review mode.

### Persona Agent Integration

Agentic testing persona agents ([agentic testing framework](../notes/bravo-agent-implementation.md)) should work through the task system like human translators. Each persona is registered as a project member with language scope and `PermTranslate`/`PermReview` role. The workflow orchestrator assigns tasks to agents the same way it assigns to human translators.

**Agent workflow change:**
```
OLD: Poll /sync/blocks → filter untranslated → translate → push
NEW: Poll /my/tasks → claim task → translate/review scoped to task → complete task
```

New MCP tools expose the task API to persona agents: `list_my_tasks`, `claim_task`, `complete_task`.

## Alternatives Considered

- **Separate workflow orchestrator**: A dedicated component outside the automation engine. Rejected because it duplicates the event-matching and action-execution logic that already exists. Building on automations means the workflow is configurable, visible in the automation UI, and follows the same execution/history patterns.

- **Callback from translation worker**: Instead of polling, the worker could emit an event when a job completes, and a tracker could count completions. Rejected because the worker is intentionally decoupled and may run in a separate process. Polling the job store is simpler and works regardless of worker deployment topology.

- **Always-on workflow (no opt-in)**: Auto-create tasks on every push. Rejected because many projects use Bowrain for automated pipelines where humans never review — forcing tasks would create noise. The workflow is opt-in via automation rules.

- **Subscription model for translators**: Translators "watch" projects and get notified of new content. Considered but deferred — the task assignment model (based on project membership and language scope) achieves the same goal with less user-facing complexity. Subscription could be added later as a refinement.

## Consequences

- Translator engagement is no longer ad-hoc — the system creates tasks when content is ready, not when content arrives.

- The automation engine gains two new event types and two new action types, all following the existing pattern. No new architectural concepts.

- Source review is an optional gate that prevents "fix once, fix N times" across target languages.

- Persona agents transition from polling-based to task-based workflows, making their behavior indistinguishable from human translators.

- The workflow is fully configurable per project through automation rules — the same UI and API used for AI translation and extraction.

- Backward compatible: `workflow_enabled` defaults to `false`. Existing projects see no behavior change until automation rules are added.

- The `push.automations.completed` event is useful beyond this workflow — it signals "content is ready" for any downstream processing.
