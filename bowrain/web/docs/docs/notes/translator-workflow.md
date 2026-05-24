---
sidebar_position: 18
title: Translator Workflow
---

# Translator Workflow

Implementation details for the translator workflow described in
[AD-014](/architecture-decisions/014-translator-workflow).

## New Event Types

In `platform/core/event/event.go`:

```go
EventPushAutomationsCompleted EventType = "push.automations.completed"
EventSourceReviewCompleted    EventType = "source.review.completed"
```

### `push.automations.completed` Data

| Field                | Source                          | Example                                           |
| -------------------- | ------------------------------- | ------------------------------------------------- |
| `push_id`            | Carried from `push.completed`   | `"abc123"`                                        |
| `items`              | Carried from `push.completed`   | `"en.json,src/locales/en.json"`                   |
| `workspace_slug`     | Carried from `push.completed`   | `"excalidraw-l10n"`                               |
| `translation_status` | Aggregated from jobs            | `"all_completed"` / `"some_failed"` / `"timeout"` |
| `extraction_status`  | Aggregated from extraction jobs | `"all_completed"` / `"none"`                      |

### `source.review.completed` Data

Same fields as the originating push (carried via `Task.Data`), plus:

| Field      | Source             | Example      |
| ---------- | ------------------ | ------------ |
| `reviewer` | Completing user ID | `"user-456"` |

## New Task Type

In `platform/store/task.go`:

```go
TaskSourceReview TaskType = "source_review"
```

Task `Data` map for review/translate tasks:

| Key       | Purpose                        |
| --------- | ------------------------------ |
| `push_id` | Links back to originating push |
| `locale`  | Target language for this task  |
| `items`   | Comma-separated item names     |
| `mode`    | `"review"` or `"translate"`    |

## PushCompletionTracker

**File:** `platform/event/push_completion_tracker.go`

```go
type PushCompletionTracker struct {
    bus          platev.EventBus
    jobStore     jobs.JobStore
    extractStore jobs.ExtractionJobStore
    contentStore platstore.ContentStore
    sub          *platev.Subscription

    mu      sync.Mutex
    pending map[string]*pendingPush // push_id → state
}

type pendingPush struct {
    projectID    string
    items        string
    wsSlug       string
    actor        string
    registeredAt time.Time
}
```

**Lifecycle:**

1. Subscribes to `EventPushCompleted`
2. On event: registers push_id in `pending` map
3. Ticker (5s) polls all pending pushes:
   - `JobStore.ListJobsByPushID(push_id)` → check all terminal
   - `ExtractionJobStore.ListByPushID(push_id)` → check all terminal
   - If both complete → emit `EventPushAutomationsCompleted`, remove from pending
4. Timeout: entries older than 30 min → emit with `status: timeout`

**Zero-job case:** Check project properties `auto_translate` and `auto_extract`.
If both are `"false"`, emit immediately (no jobs to wait for).

**Wire-up** in `platform/server/server.go`:

```go
if s.EventBus != nil && s.JobStore != nil {
    s.pushCompletionTracker = event.NewPushCompletionTracker(
        s.EventBus, s.JobStore, s.ExtractionJobStore, s.ContentStore,
    )
}
```

## ExtractionJobStore.ListByPushID

**Interface addition** in `platform/jobs/extraction_store.go`:

```go
ListByPushID(ctx context.Context, pushID string) ([]*ExtractionJob, error)
```

**SQLite implementation:**

```sql
SELECT * FROM extraction_jobs WHERE push_id = ? ORDER BY created_at
```

## New Automation Actions

### `create_review_tasks`

In `platform/server/automation.go`, added to `doExecuteAction()`:

```go
case "create_review_tasks":
    go s.createReviewTasks(context.Background(), action, ev)
```

**Algorithm:**

```go
func (s *Server) createReviewTasks(ctx context.Context, action AutomationAction, ev Event) {
    proj := loadProject(ev.ProjectID)
    members := s.AuthStore.ListProjectMembers(ctx, proj.ID)

    mode := action.Config["mode"]  // "review" (default) or "translate"
    if mode == "" { mode = "review" }
    taskType := bstore.TaskReview
    if mode == "translate" { taskType = bstore.TaskTranslate }

    priority := action.Config["priority"]
    if priority == "" { priority = "normal" }

    for _, locale := range proj.TargetLanguages {
        assignees := findMembersForLocale(members, locale, mode)

        for _, assignee := range assignees {
            // Dedup: skip if open task exists for (project, locale, push_id)
            task := &bstore.Task{
                WorkspaceID: wsID,
                ProjectID:   proj.ID,
                Type:        taskType,
                Priority:    priority,
                Title:       fmt.Sprintf("Review %s translations", locale),
                AssigneeID:  assignee.UserID,
                CreatedBy:   "system",
                Data: map[string]string{
                    "push_id": ev.Data["push_id"],
                    "locale":  string(locale),
                    "items":   ev.Data["items"],
                    "mode":    mode,
                },
            }
            s.TaskStore.Create(ctx, task)
            s.NotificationDispatcher.DispatchTaskNotification(ctx, task, NotificationTaskAssigned)
        }

        if len(assignees) == 0 {
            // Create unassigned task, notify admins
        }
    }
}
```

**Member matching:** `findMembersForLocale` filters project members by:

1. Language scope: `member.Languages` contains locale, or is empty (all languages)
2. Permission: `PermReview` for review mode, `PermTranslate` for translate mode

### `create_source_review`

```go
case "create_source_review":
    go s.createSourceReviewTask(context.Background(), action, ev)
```

```go
func (s *Server) createSourceReviewTask(ctx context.Context, action AutomationAction, ev Event) {
    reviewer := action.Config["reviewer"]
    if reviewer == "" {
        // Fall back to first member with PermEditSource
        reviewer = findMemberWithPerm(members, platauth.PermEditSource)
    }

    task := &bstore.Task{
        Type:       bstore.TaskSourceReview,
        Title:      "Review source content before translation",
        AssigneeID: reviewer,
        CreatedBy:  "system",
        Data:       ev.Data, // carries push_id, items, workspace_slug
    }
    s.TaskStore.Create(ctx, task)
    s.NotificationDispatcher.DispatchTaskNotification(ctx, task, NotificationTaskAssigned)
}
```

## Source Review Completion Hook

In `platform/server/handlers_task.go`, `HandleCompleteTask()`:

```go
// After TaskStore.Complete() succeeds:
if task.Type == bstore.TaskSourceReview && s.EventBus != nil {
    s.EventBus.Publish(platev.Event{
        Type:        platev.EventSourceReviewCompleted,
        ProjectID:   task.ProjectID,
        Actor:       userID,
        Data:        task.Data,
        CausationID: event.NextCausationID(platev.Event{ID: task.ID}),
    })
}
```

## Default Automation Rules

In `registerDefaultAutomations()`:

```go
// Rule 4: Create review tasks when automations complete (disabled by default).
s.AutomationEngine.AddRule(event.AutomationRule{
    Name:      "create-review-tasks-on-automation-complete",
    EventType: platev.EventPushAutomationsCompleted,
    Actions: []event.AutomationAction{
        {Type: "create_review_tasks", Config: map[string]string{"mode": "review"}},
    },
})
```

Users opt in by enabling this rule via the automations API or UI. For source
review workflows, users create two rules:

1. `push.automations.completed → create_source_review` (with `reviewer` config)
2. `source.review.completed → create_review_tasks`

## Implementation Phases

### Phase 1: Foundation (types only)

- `platform/core/event/event.go` — add 2 event types
- `platform/store/task.go` — add `TaskSourceReview`
- `platform/jobs/extraction_store.go` — add `ListByPushID` interface + implementations

### Phase 2: PushCompletionTracker

- `platform/event/push_completion_tracker.go` — new file
- `platform/event/push_completion_tracker_test.go` — new file
- `platform/server/server.go` — wire up

### Phase 3: Automation actions

- `platform/server/automation.go` — add actions + default rules
- `platform/server/handlers_task.go` — source review hook
- Tests
