---
id: 027-activities-tasks-notifications
sidebar_position: 27
title: "AD-027: Activities, Tasks, and Notifications"
---
# AD-027: Activities, Tasks, and Notifications

## Context

Bowrain has a mature automation system ([AD-011](./011-automation.md)) that reacts to events — triggering flows, webhooks, and quality gates without human involvement. But localization is inherently collaborative. Translators need review assignments. Project managers need visibility into what happened and what's blocked. Reviewers need to know when work is ready. These are human concerns that automation alone cannot address.

The existing infrastructure provides strong foundations:

- **Event bus** (`platform/core/event`) — typed events with causation tracking, published by `EventEmittingStore` on every content mutation
- **Audit log** (`platform/event/audit.go`) — persists all events to PostgreSQL with workspace-scoped queries
- **Notification store** (`platform/store/notification.go`) — user-targeted notifications with type, title, body, deep links, and read state; supports both SQLite and PostgreSQL
- **Notification hub** (`platform/server/ws_notifications.go`) — WebSocket-based real-time delivery to connected clients via per-user connection tracking
- **useNotifications hook** (`platform/packages/ui/src/hooks/useNotifications.ts`) — React hook combining REST polling with WebSocket for real-time updates
- **Translation jobs** (`platform/jobs/`) — async job lifecycle with progress tracking, queued via ChannelQueue, NATS, or Azure Service Bus
- **Review queue** (`platform/store/review_queue.go`) — term candidates and entity reviews with assignment, batch decisions, and split support
- **Collaborative editing** (`platform/server/ws_collab.go`) — Yjs-based real-time co-editing with presence
- **Redis** — session store for multi-instance deployments; connection already configured in `ServerConfig`

What's missing is the connective tissue: a unified **activity feed** showing what happened across a project, a **task system** for assigning and tracking human work, **notification preferences** so users control what reaches them, and **multi-channel delivery** spanning web, desktop, mobile push, and email digests.

## Decision

### Three Concepts

| Concept | What it is | Who sees it | Persistence |
|---------|-----------|-------------|-------------|
| **Activity** | An immutable record of something that happened | Everyone in the project/workspace | PostgreSQL `activities` table |
| **Task** | An actionable work item assigned to a person | Assignee + project members | PostgreSQL `tasks` table |
| **Notification** | A user-targeted alert about something relevant | Individual user | Existing `notifications` table (extended) |

Activities are the **source of truth** — every significant event produces an activity. Tasks and notifications are **derived** from activities (or created directly for human-initiated assignments).

### Activity Feed

Activities provide a project-level and workspace-level timeline — "what happened, when, by whom."

#### Schema

```sql
CREATE TABLE activities (
    id              TEXT PRIMARY KEY,
    workspace_id    TEXT NOT NULL,
    project_id      TEXT NOT NULL DEFAULT '',
    stream          TEXT NOT NULL DEFAULT 'main',
    actor_id        TEXT NOT NULL,                    -- user ID or 'system'
    actor_name      TEXT NOT NULL DEFAULT '',
    type            TEXT NOT NULL,                    -- activity type (see below)
    entity_type     TEXT NOT NULL DEFAULT '',         -- 'block', 'item', 'project', 'stream', etc.
    entity_id       TEXT NOT NULL DEFAULT '',         -- ID of the affected entity
    summary         TEXT NOT NULL,                    -- human-readable summary
    data            JSONB NOT NULL DEFAULT '{}',      -- structured payload
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Indexes
    CONSTRAINT fk_activities_project FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_activities_workspace ON activities(workspace_id, created_at DESC);
CREATE INDEX idx_activities_project   ON activities(project_id, created_at DESC);
CREATE INDEX idx_activities_actor     ON activities(actor_id, created_at DESC);
CREATE INDEX idx_activities_type      ON activities(type, created_at DESC);
```

#### Activity Types

```go
type ActivityType string

const (
    // Content lifecycle
    ActivityItemPushed        ActivityType = "item.pushed"
    ActivityItemPulled        ActivityType = "item.pulled"
    ActivityBlockTranslated   ActivityType = "block.translated"
    ActivityBlockReviewed     ActivityType = "block.reviewed"
    ActivityBlockCommented    ActivityType = "block.commented"

    // Project management
    ActivityProjectCreated    ActivityType = "project.created"
    ActivityProjectUpdated    ActivityType = "project.updated"
    ActivityLocaleAdded       ActivityType = "locale.added"
    ActivityMemberAdded       ActivityType = "member.added"
    ActivityMemberRemoved     ActivityType = "member.removed"

    // Stream operations
    ActivityStreamCreated     ActivityType = "stream.created"
    ActivityStreamMerged      ActivityType = "stream.merged"

    // Automation & AI
    ActivityFlowCompleted     ActivityType = "flow.completed"
    ActivityFlowFailed        ActivityType = "flow.failed"
    ActivityJobCompleted      ActivityType = "job.completed"
    ActivityJobFailed         ActivityType = "job.failed"
    ActivityExtractionDone    ActivityType = "extraction.completed"

    // Quality
    ActivityGatePassed        ActivityType = "gate.passed"
    ActivityGateFailed        ActivityType = "gate.failed"
    ActivityBrandDrift        ActivityType = "brand.drift"

    // Review queue
    ActivityReviewAssigned    ActivityType = "review.assigned"
    ActivityReviewDecided     ActivityType = "review.decided"

    // Connectors
    ActivityConnectorSynced   ActivityType = "connector.synced"
    ActivityConnectorFailed   ActivityType = "connector.failed"

    // Tasks
    ActivityTaskCreated       ActivityType = "task.created"
    ActivityTaskCompleted     ActivityType = "task.completed"
    ActivityTaskReassigned    ActivityType = "task.reassigned"

    // Versions
    ActivityVersionCreated    ActivityType = "version.created"
)
```

#### Activity Generation

Activities are generated by a new `ActivityRecorder` that subscribes to the event bus, similar to `AuditLogger`:

```go
type ActivityRecorder struct {
    db  *sql.DB
    bus platev.EventBus
    sub *platev.Subscription
}
```

The recorder maps events to activities, enriching them with actor names (resolved from the auth context attached to the event) and human-readable summaries. Unlike the audit log (which records raw events), activities are curated for human consumption — they group related events (e.g., a push that updates 50 blocks produces one `item.pushed` activity, not 50 `block.updated` activities).

#### Activity Aggregation

For high-frequency operations, activities are **aggregated** rather than recorded individually:

- **Batch pushes**: 15 items pushed in one sync → single activity with `data.item_count: 15`
- **Translation jobs**: 200 blocks translated → single `job.completed` activity with block count and locale
- **Review batches**: 10 terms approved at once → single `review.decided` activity

The `data` JSONB field carries structured details for drill-down in the UI.

#### API

```
GET /api/v1/workspaces/:ws/activities
    ?project_id=...           # filter by project
    ?stream=...               # filter by stream
    ?actor_id=...             # filter by user
    ?type=...                 # filter by type prefix (e.g. "block" matches block.*)
    ?since=<ISO8601>          # after timestamp
    ?limit=50&cursor=...      # cursor pagination
```

Returns:
```json
{
  "activities": [
    {
      "id": "act_...",
      "type": "block.reviewed",
      "actor": { "id": "usr_...", "name": "Marie", "avatar_url": "..." },
      "project": { "id": "prj_...", "name": "Website" },
      "stream": "main",
      "entity_type": "block",
      "entity_id": "blk_...",
      "summary": "Marie reviewed 12 blocks in fr-FR",
      "data": { "locale": "fr-FR", "count": 12, "item_name": "homepage.json" },
      "created_at": "2026-03-16T10:30:00Z"
    }
  ],
  "next_cursor": "..."
}
```

### Task System

Tasks are human work items. They represent something a person needs to do and track whether they did it.

#### Schema

```sql
CREATE TABLE tasks (
    id              TEXT PRIMARY KEY,
    workspace_id    TEXT NOT NULL,
    project_id      TEXT NOT NULL,
    stream          TEXT NOT NULL DEFAULT 'main',
    type            TEXT NOT NULL,                    -- task type
    status          TEXT NOT NULL DEFAULT 'open',     -- open, in_progress, completed, cancelled
    priority        TEXT NOT NULL DEFAULT 'normal',   -- low, normal, high, urgent
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    assignee_id     TEXT NOT NULL DEFAULT '',
    created_by      TEXT NOT NULL,
    completed_by    TEXT NOT NULL DEFAULT '',
    data            JSONB NOT NULL DEFAULT '{}',       -- task-type-specific payload
    due_at          TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,

    CONSTRAINT fk_tasks_project FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_tasks_assignee   ON tasks(assignee_id, status, created_at DESC);
CREATE INDEX idx_tasks_project    ON tasks(project_id, status, created_at DESC);
CREATE INDEX idx_tasks_workspace  ON tasks(workspace_id, status, created_at DESC);
CREATE INDEX idx_tasks_due        ON tasks(due_at) WHERE status IN ('open', 'in_progress');
```

#### Task Types

```go
type TaskType string

const (
    // Translation tasks
    TaskTranslate       TaskType = "translate"        // translate blocks for a locale
    TaskReview          TaskType = "review"            // review translated blocks
    TaskReviewTerms     TaskType = "review_terms"      // review term candidates

    // Quality tasks
    TaskFixQuality      TaskType = "fix_quality"       // fix QA issues
    TaskFixBrandVoice   TaskType = "fix_brand_voice"   // fix brand voice violations
    TaskFixTerminology  TaskType = "fix_terminology"   // fix terminology violations

    // Management tasks
    TaskConnectorSetup  TaskType = "connector_setup"   // configure a connector
    TaskCustom          TaskType = "custom"             // free-form task
)
```

#### Task Data Payloads

The `data` JSONB field carries type-specific context:

```go
// TaskTranslate: which items and locales need translation
type TranslateTaskData struct {
    ItemNames    []string `json:"item_names"`
    TargetLocale string   `json:"target_locale"`
    BlockCount   int      `json:"block_count"`
}

// TaskReview: which items and locales need review
type ReviewTaskData struct {
    ItemNames    []string `json:"item_names"`
    Locale       string   `json:"locale"`
    BlockCount   int      `json:"block_count"`
}

// TaskFixQuality: QA issues to resolve
type FixQualityTaskData struct {
    ItemName  string   `json:"item_name"`
    Locale    string   `json:"locale"`
    IssueIDs  []string `json:"issue_ids"`
    GateName  string   `json:"gate_name"`
}
```

#### Task Lifecycle

```
                    ┌─────────┐
             ┌──────│  open   │──────┐
             │      └────┬────┘      │
          cancel         │         assign
             │      ┌────▼─────┐     │
             │      │in_progress│     │
             │      └────┬─────┘     │
             │      complete │       │
             │      ┌────▼────┐      │
             ├──────│completed│      │
             │      └─────────┘      │
        ┌────▼─────┐                 │
        │cancelled │                 │
        └──────────┘                 │
```

Tasks can be created in three ways:

1. **Manually** — a project manager creates a task and assigns it to a team member
2. **By automation** — an automation rule creates a task (e.g., "on push.completed, create review task for all new blocks")
3. **By the system** — quality gate failures, brand voice drift, or extraction completion auto-generate tasks

#### Auto-Generated Tasks from Automation

The existing `AutomationAction` type `"notify"` is extended with a new `"task"` action type:

```yaml
automations:
  - name: assign-review-on-translation
    on: job.completed
    conditions:
      - field: status
        operator: equals
        value: completed
    actions:
      - type: task
        config:
          task_type: review
          assignee: "@reviewer-role"    # role-based assignment
          priority: normal
          title: "Review AI translations for {{.data.target_locale}}"
```

#### API

```
# Task CRUD
GET    /api/v1/workspaces/:ws/tasks
       ?project_id=...&assignee_id=...&status=...&type=...&priority=...
POST   /api/v1/workspaces/:ws/tasks
GET    /api/v1/workspaces/:ws/tasks/:id
PATCH  /api/v1/workspaces/:ws/tasks/:id
DELETE /api/v1/workspaces/:ws/tasks/:id

# Task actions
POST   /api/v1/workspaces/:ws/tasks/:id/assign    { "assignee_id": "usr_..." }
POST   /api/v1/workspaces/:ws/tasks/:id/complete
POST   /api/v1/workspaces/:ws/tasks/:id/cancel

# My tasks (cross-project)
GET    /api/v1/workspaces/:ws/my/tasks
       ?status=open&sort=due_at
```

### Notification System (Extended)

The existing notification infrastructure is extended with **preferences**, **multi-channel delivery**, and **grouping**.

#### Notification Preferences

Users configure which events generate notifications and through which channels:

```sql
CREATE TABLE notification_preferences (
    user_id         TEXT NOT NULL,
    workspace_id    TEXT NOT NULL,
    category        TEXT NOT NULL,    -- e.g. "review", "task", "quality", "automation"
    channel_web     BOOLEAN NOT NULL DEFAULT TRUE,
    channel_email   BOOLEAN NOT NULL DEFAULT FALSE,
    channel_push    BOOLEAN NOT NULL DEFAULT FALSE,
    channel_desktop BOOLEAN NOT NULL DEFAULT TRUE,

    PRIMARY KEY (user_id, workspace_id, category)
);
```

**Notification categories:**

| Category | Triggers | Default channels |
|----------|----------|-----------------|
| `task` | Task assigned, task due soon, task overdue | web, desktop, email |
| `review` | Review requested, review completed | web, desktop |
| `quality` | Quality gate failed, brand voice drift | web, desktop |
| `automation` | Flow failed, connector error | web |
| `mention` | @mentioned in a comment or note | web, desktop, email |
| `project` | Member added/removed, locale added | web |
| `system` | Workspace quota warning, maintenance | web, email |

#### API for Preferences

```
GET  /api/v1/workspaces/:ws/notifications/preferences
PUT  /api/v1/workspaces/:ws/notifications/preferences
```

```json
{
  "preferences": [
    {
      "category": "task",
      "channels": { "web": true, "email": true, "push": true, "desktop": true }
    },
    {
      "category": "review",
      "channels": { "web": true, "email": false, "push": false, "desktop": true }
    }
  ]
}
```

#### Extended Notification Model

The existing `Notification` struct is extended:

```go
type Notification struct {
    // ... existing fields ...
    ID        string           `json:"id"`
    UserID    string           `json:"user_id"`
    Type      NotificationType `json:"type"`
    Title     string           `json:"title"`
    Body      string           `json:"body"`
    ProjectID string           `json:"project_id,omitempty"`
    LinkURL   string           `json:"link_url,omitempty"`
    Read      bool             `json:"read"`
    CreatedAt time.Time        `json:"created_at"`

    // New fields
    Category  string           `json:"category"`            // preference category
    GroupKey  string           `json:"group_key,omitempty"` // for grouping (e.g., push_id)
    ActorID   string           `json:"actor_id,omitempty"`
    ActorName string           `json:"actor_name,omitempty"`
    TaskID    string           `json:"task_id,omitempty"`   // linked task
    Priority  string           `json:"priority,omitempty"`  // normal, high
}
```

New notification types extending the existing set:

```go
const (
    // Existing
    NotificationReviewAssigned  NotificationType = "review.assigned"
    NotificationReviewCompleted NotificationType = "review.completed"
    NotificationExtractionDone  NotificationType = "extraction.completed"
    NotificationGeneral         NotificationType = "general"

    // New: Tasks
    NotificationTaskAssigned    NotificationType = "task.assigned"
    NotificationTaskDueSoon     NotificationType = "task.due_soon"
    NotificationTaskOverdue     NotificationType = "task.overdue"
    NotificationTaskCompleted   NotificationType = "task.completed"

    // New: Quality
    NotificationGateFailed      NotificationType = "quality.gate.failed"
    NotificationBrandDrift      NotificationType = "brand.drift"

    // New: Social
    NotificationMention         NotificationType = "mention"
    NotificationComment         NotificationType = "comment"

    // New: Automation
    NotificationFlowFailed      NotificationType = "flow.failed"
    NotificationConnectorError  NotificationType = "connector.error"

    // New: System
    NotificationQuotaWarning    NotificationType = "quota.warning"
)
```

#### Notification Grouping

High-frequency notifications are grouped by `group_key` to avoid flooding:

- All translation job completions from the same push → grouped under `push_id`
- Multiple quality gate failures on the same item → grouped under `item:locale`
- Multiple review assignments from the same batch → grouped under batch ID

The UI displays grouped notifications as a single card with a count badge and expandable details.

### Multi-Channel Delivery

#### Architecture

```
Event Bus ──► ActivityRecorder ──► activities table
         │
         └──► NotificationDispatcher
                 │
                 ├──► preference check (per user × category)
                 │
                 ├──► Web: existing notificationHub (WebSocket)
                 ├──► Desktop: Wails event emission (via gRPC stream)
                 ├──► Email: batched digest via Mailer
                 ├──► Mobile Push: FCM/APNs via push service
                 └──► (future) Slack/Teams webhook
```

#### NotificationDispatcher

A new component that bridges events to user-targeted notifications with preference-aware routing:

```go
type NotificationDispatcher struct {
    bus         platev.EventBus
    store       *NotificationStore
    prefStore   *PreferenceStore
    hub         *notificationHub     // WebSocket delivery
    mailer      *mailer.Mailer       // email delivery
    pushService PushService          // mobile push (FCM/APNs)
    server      *Server              // for desktop gRPC stream
}
```

The dispatcher:
1. Subscribes to relevant event types
2. Determines which users should be notified (project members, assignees, watchers)
3. Checks each user's notification preferences for the category
4. Persists the notification to the store
5. Fans out to enabled channels

#### Web Delivery (existing)

The existing `notificationHub` and `useNotifications` hook already handle web delivery. No changes needed — the dispatcher calls `hub.notifyUser()` as the server does today.

#### Desktop Delivery (Wails)

The desktop app connects to bowrain-server via gRPC (`ProjectWatcher`). The existing `WatchProject` stream is extended to include notification events:

```protobuf
message WatchEvent {
    oneof event {
        BlockChangeEvent block_change = 1;
        PresenceChangeEvent presence_change = 2;
        NotificationEvent notification = 3;    // NEW
        TaskEvent task = 4;                     // NEW
    }
}

message NotificationEvent {
    string id = 1;
    string type = 2;
    string title = 3;
    string body = 4;
    string link_url = 5;
    string category = 6;
    string priority = 7;
}
```

The Wails backend receives these via the existing gRPC stream and emits Wails events (`notification-received`, `task-updated`), which the React frontend handles to show native OS notifications (via Wails notification API) and update the in-app notification panel.

#### Email Delivery

Emails use the existing `Mailer` infrastructure. Two modes:

1. **Immediate** — high-priority notifications (task assigned with urgent priority, blocking gate failure) send immediately via the existing Resend/SMTP pipeline
2. **Digest** — lower-priority notifications are batched into periodic digests (hourly or daily, configurable per user)

Digest assembly runs as a periodic job (cron or ticker in the server process):

```go
// DigestWorker runs on a schedule and sends batched notification emails.
type DigestWorker struct {
    store     *NotificationStore
    prefStore *PreferenceStore
    mailer    *mailer.Mailer
    interval  time.Duration   // e.g., 1 hour
}
```

Email templates are added to the existing `mailer` package: `notification_immediate.html` and `notification_digest.html`.

#### Mobile Push (FCM/APNs)

Mobile push delivery requires:

1. **Device registration** — a new `push_tokens` table stores FCM/APNs tokens per user per device
2. **Push service** — a thin abstraction over FCM (Firebase Cloud Messaging) that handles both Android and iOS

```sql
CREATE TABLE push_tokens (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    platform    TEXT NOT NULL,    -- 'ios', 'android', 'web'
    token       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(user_id, token)
);
```

```go
type PushService interface {
    Send(ctx context.Context, userID string, payload PushPayload) error
}

type PushPayload struct {
    Title    string            `json:"title"`
    Body     string            `json:"body"`
    Data     map[string]string `json:"data"`     // deep link, task ID, etc.
    Priority string            `json:"priority"`  // "normal" or "high"
}
```

The push service is optional — when `nil`, mobile push is simply skipped.

#### Redis Pub/Sub for Multi-Instance

The current `notificationHub` is in-process only. For multi-instance deployments (horizontal scaling), notifications published on one server instance need to reach WebSocket clients connected to other instances.

Redis Pub/Sub bridges this gap:

```go
type RedisNotificationBridge struct {
    client *redis.Client
    hub    *notificationHub
    sub    *redis.PubSub
}
```

When the dispatcher creates a notification:
1. It publishes to Redis channel `notifications:{user_id}`
2. All server instances subscribed to that channel receive it
3. Each instance checks if the user has local WebSocket connections and delivers

This reuses the existing Redis connection from `ServerConfig.RedisURL`. When Redis is not configured, the system falls back to local-only delivery (single-instance mode).

### Real-Time Updates for Activities and Tasks

#### Activity Stream via WebSocket

A new WebSocket endpoint delivers real-time activity updates to the workspace or project view:

```
GET /api/v1/workspaces/:ws/activities/ws?project_id=...
```

The activity hub broadcasts new activities to all connected clients viewing the relevant project or workspace. This powers live activity feeds without polling.

#### Task Updates

Task state changes (assigned, completed, cancelled) are broadcast to:
- The assignee (via notification channels)
- All clients viewing the project's task board (via a task WebSocket or piggybacked on the activity stream)

### UI Components

All UI components live in the shared `packages/ui` library for reuse across web, desktop, and mobile.

#### Activity Feed Component

```
┌─────────────────────────────────────────────────┐
│ Activity                                    🔍   │
├─────────────────────────────────────────────────┤
│ 👤 Marie reviewed 12 blocks in fr-FR        10m │
│    homepage.json · Website                       │
│                                                  │
│ 🤖 AI translation completed                 25m │
│    47 blocks · de-DE · homepage.json             │
│                                                  │
│ 👤 Alex pushed 3 items                      1h  │
│    settings.json, nav.json, footer.json          │
│                                                  │
│ ⚠️ Quality gate failed: terminology         2h  │
│    12 violations · fr-FR · homepage.json         │
│                                                  │
│ 👤 System created stream feature/new-ui      3h │
│    branched from main                            │
│─────────────────────────────────────────────────│
│                Load more...                      │
└─────────────────────────────────────────────────┘
```

The activity feed appears in:
- **Project dashboard** — filtered to the active project
- **Workspace overview** — aggregated across all projects
- **Block detail panel** — filtered to the active block (replaces/augments block history)

#### Task Board Component

```
┌──────────────────────────────────────────────────────────────┐
│ Tasks                                      + New Task   🔍   │
├──────────────────────────────────────────────────────────────┤
│  Open (4)           In Progress (2)        Completed (12)    │
│ ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
│ │ Review fr-FR │  │ Translate    │  │ Fix QA       │       │
│ │ homepage.json│  │ de-DE nav    │  │ issues       │       │
│ │ @Marie · 🔴  │  │ @Alex        │  │ @Marie ✓     │       │
│ │ Due: Mar 17  │  │              │  │ Mar 15       │       │
│ └──────────────┘  └──────────────┘  └──────────────┘       │
│ ┌──────────────┐  ┌──────────────┐                          │
│ │ Review terms │  │ Fix brand    │                          │
│ │ @Chris       │  │ voice issues │                          │
│ │ Due: Mar 18  │  │ @Marie       │                          │
│ └──────────────┘  └──────────────┘                          │
└──────────────────────────────────────────────────────────────┘
```

The task board is accessible from:
- **Project sidebar** — project-scoped tasks
- **"My Tasks" view** — cross-project personal task list with due date sorting
- **Dashboard home** — summary cards showing overdue and upcoming tasks

#### Notification Panel (Enhanced)

The existing notification dropdown is enhanced with:
- **Grouped notifications** — collapsible groups for batch operations
- **Category badges** — colored indicators for task, review, quality, etc.
- **Inline actions** — "Mark as done", "Go to task", "View in editor" without leaving the panel
- **Preference link** — quick access to notification settings

```
┌──────────────────────────────────────────┐
│ Notifications                  Mark all ✓ │
├──────────────────────────────────────────┤
│ 🔴 Task assigned                     2m  │
│    Review fr-FR translations for         │
│    homepage.json                         │
│    [Go to task]  [Open in editor]        │
│                                          │
│ ⚠️ Quality gate failed              15m  │
│    terminology-compliance: 12 violations │
│    [View issues]                         │
│                                          │
│ 📦 3 translation jobs completed     30m  │
│    de-DE, ja-JP, ko-KR for homepage      │
│    [View details]                        │
│                                          │
│ 💬 Alex commented on block blk_7f3   1h  │
│    "Should this be formal or informal?"  │
│    [Reply]                               │
│──────────────────────────────────────────│
│ ⚙ Notification preferences              │
└──────────────────────────────────────────┘
```

#### Mobile Considerations

The mobile app (future, or PWA) uses the same notification API and WebSocket infrastructure. Key adaptations:

- **Push notifications** deliver high-priority alerts when the app is backgrounded (via FCM/APNs)
- **Deep links** in notifications and tasks navigate directly to the relevant block/item/project in the mobile editor
- **Task list** is the primary mobile view — translators and reviewers can triage and act on tasks from their phone
- **Activity feed** is swipeable and supports pull-to-refresh

### Implementation in Existing Module Structure

All new code follows the existing module boundaries:

| Component | Module | Location |
|-----------|--------|----------|
| Activity types, store interface | `platform` | `platform/store/activity.go` |
| Task types, store interface | `platform` | `platform/store/task.go` |
| Notification preferences | `platform` | `platform/store/notification_preferences.go` |
| Push token store | `platform` | `platform/store/push_token.go` |
| ActivityRecorder | `bowrain` (platform) | `platform/event/activity_recorder.go` |
| NotificationDispatcher | `bowrain` (platform) | `platform/event/notification_dispatcher.go` |
| DigestWorker | `bowrain` (platform) | `platform/event/digest_worker.go` |
| Redis notification bridge | `bowrain` (platform) | `platform/server/ws_notifications_redis.go` |
| PushService (FCM) | `bowrain` (platform) | `platform/push/fcm.go` |
| REST handlers | `bowrain` (platform) | `platform/server/handlers_activity.go`, `handlers_task.go` |
| Activity WebSocket | `bowrain` (platform) | `platform/server/ws_activities.go` |
| gRPC notification stream | `bowrain` (platform) | `platform/server/grpc_editor.go` (extend) |
| UI components | shared | `platform/packages/ui/src/components/ActivityFeed.tsx`, `TaskBoard.tsx` |
| React hooks | shared | `platform/packages/ui/src/hooks/useActivities.ts`, `useTasks.ts` |
| Desktop integration | bowrain app | `platform/apps/bowrain/backend/notifications.go` |

### Database Migrations

New tables are added via the existing migration system (`storage.Migration`):

```go
var activityMigrations = []storage.Migration{
    {
        Version:     1,
        Description: "create activities table",
        SQL:         `CREATE TABLE activities (...)`,
    },
}

var taskMigrations = []storage.Migration{
    {
        Version:     1,
        Description: "create tasks table",
        SQL:         `CREATE TABLE tasks (...)`,
    },
}

var notificationExtMigrations = []storage.Migration{
    {
        Version:     1,
        Description: "add new columns to notifications and create preferences table",
        SQL: `
            ALTER TABLE notifications ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'general';
            ALTER TABLE notifications ADD COLUMN IF NOT EXISTS group_key TEXT NOT NULL DEFAULT '';
            ALTER TABLE notifications ADD COLUMN IF NOT EXISTS actor_id TEXT NOT NULL DEFAULT '';
            ALTER TABLE notifications ADD COLUMN IF NOT EXISTS actor_name TEXT NOT NULL DEFAULT '';
            ALTER TABLE notifications ADD COLUMN IF NOT EXISTS task_id TEXT NOT NULL DEFAULT '';
            ALTER TABLE notifications ADD COLUMN IF NOT EXISTS priority TEXT NOT NULL DEFAULT 'normal';

            CREATE TABLE notification_preferences (...);
            CREATE TABLE push_tokens (...);
        `,
    },
}
```

### Relationship to Existing Systems

| Existing System | Relationship |
|----------------|-------------|
| **Event bus** | Activities and notifications are derived from events. The event bus remains the source of truth for system events. |
| **Audit log** | Audit log records raw events for compliance/debugging. Activities are curated for human consumption. Both subscribe to the same event bus. They coexist — audit log is for ops, activities are for users. |
| **Automation engine** | Extended with `"task"` action type. Automation can create tasks as a side effect of rule execution. |
| **Translation jobs** | Job lifecycle events (`job.completed`, `job.failed`) generate activities and notifications. The job system itself is unchanged. |
| **Review queue** | Review assignments generate tasks and notifications. The review queue store remains the source of truth for review item state. |
| **Collaborative editing** | Presence and block changes remain on the collab WebSocket. Notifications and activities use the separate notification WebSocket. |
| **Quality gates** | Gate failures generate activities, notifications, and optionally tasks (for fix assignments). |

## Alternatives Considered

- **Unified activity + notification table**: Simpler schema but conflates two different access patterns — activities are project-scoped timelines queried by time range, notifications are user-scoped inboxes queried by read state. Separate tables optimize for each pattern.

- **External notification service (OneSignal, Novu)**: Adds a runtime dependency and data residency concerns. The notification volume for localization workflows is modest (hundreds/day, not millions). A built-in system is simpler and keeps data in the same PostgreSQL instance.

- **GraphQL subscriptions instead of WebSocket**: Would require adding a GraphQL layer. The existing WebSocket infrastructure works well and is already battle-tested for collaborative editing. Adding another transport protocol introduces complexity without clear benefit.

- **Separate task management service**: Over-engineered for this scope. Tasks are tightly coupled to localization concepts (blocks, locales, items) and benefit from being in the same database as the content store for efficient joins and consistent transactions.

- **NATS JetStream for notification fanout**: Could replace Redis Pub/Sub for multi-instance notification delivery. However, Redis is already a configured dependency for session state, making Pub/Sub a zero-new-dependency choice. NATS is available for the job queue but is not universally deployed.

## Consequences

- **Activities provide visibility** — project managers and team leads can see exactly what happened, when, and by whom, without reading audit logs.

- **Tasks close the human loop** — automation triggers work, tasks assign it to people, notifications alert them, and activities record the outcome. The full cycle is: event → automation → task → notification → human action → activity.

- **Notifications become preference-aware** — users control what reaches them and through which channel, reducing alert fatigue. The existing notification infrastructure is extended rather than replaced.

- **Multi-channel delivery spans all platforms** — web (WebSocket), desktop (gRPC stream → Wails event → OS notification), mobile (FCM/APNs push), and email (immediate + digest). Each channel reuses existing infrastructure where possible.

- **Redis Pub/Sub enables horizontal scaling** — notifications reach users regardless of which server instance they're connected to. Falls back gracefully to local-only when Redis is unavailable.

- **The event bus remains the integration point** — all three systems (activities, tasks, notifications) subscribe to the same event bus. New event types automatically become available for activity recording, task automation, and notification routing.

- **No new external dependencies for core functionality** — activities and tasks use PostgreSQL (already required), notifications use the existing WebSocket hub, email uses the existing Mailer, and multi-instance fanout uses the existing Redis connection. Mobile push (FCM) is the only new external dependency, and it's optional.

- **The shared UI library ensures consistency** — ActivityFeed, TaskBoard, and enhanced NotificationPanel components in `packages/ui` work identically across web and desktop apps.

- **Migration path is incremental** — activities can ship first (read-only timeline), then tasks (assignment workflow), then notification preferences and multi-channel delivery. Each phase is independently useful.
