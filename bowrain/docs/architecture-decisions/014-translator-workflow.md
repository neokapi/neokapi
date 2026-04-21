---
id: 014-translator-workflow
sidebar_position: 14
title: "AD-014: Translator Workflow"
---

# AD-014: Translator Workflow

## Summary

Bowrain's translator workflow coordinates the handoff between
automation and humans. An immutable **activity feed** records what
happened; **tasks** assign work; **notifications** reach the right
people on the right channel. A `PushCompletionTracker` emits
`push.automations.completed` when prep-work automations finish, gating
task fan-out so translators engage only on content that is ready. An
optional source-review gate prevents fixing the same source issue once
per locale.

## Context

Localization is collaborative. Automation can translate, extract
entities, and enforce quality gates, but humans still review,
translate, and sign off. The gap to close is between "automation has
produced content" and "a human sees it in their inbox and acts on it".
Everything that fills that gap — activity, tasks, notifications, and
the events that drive them — lives in one coherent subsystem so the
data model, UI, and event flow stay consistent.

## Decision

### Three concepts, one substrate

| Concept          | Record of                                     | Audience                         | Persistence                    |
| ---------------- | --------------------------------------------- | -------------------------------- | ------------------------------ |
| **Activity**     | Something that happened                       | Everyone in the project/workspace | `activities` table             |
| **Task**         | An actionable work item assigned to a person  | Assignee + project members       | `tasks` table                  |
| **Notification** | A user-targeted alert                         | Individual user                  | `notifications` table          |

Activities are the source of truth — every significant event produces
an activity. Tasks and notifications derive from activities (or from
human-initiated assignments).

### Activity feed

```sql
CREATE TABLE activities (
    id              TEXT PRIMARY KEY,
    workspace_id    TEXT NOT NULL,
    project_id      TEXT NOT NULL DEFAULT '',
    stream          TEXT NOT NULL DEFAULT 'main',
    actor_id        TEXT NOT NULL,
    actor_name      TEXT NOT NULL DEFAULT '',
    type            TEXT NOT NULL,
    entity_type     TEXT NOT NULL DEFAULT '',
    entity_id       TEXT NOT NULL DEFAULT '',
    summary         TEXT NOT NULL,
    data            JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Activity types cover content lifecycle (`item.pushed`,
`block.translated`, `block.reviewed`), project management
(`project.updated`, `member.added`), streams (`stream.created`,
`stream.merged`), automation and AI (`flow.completed`, `job.completed`,
`extraction.completed`), quality (`gate.passed`, `gate.failed`,
`brand.drift`), review queue (`review.assigned`, `review.decided`),
connectors, tasks, and versions.

The `ActivityRecorder` subscribes to the event bus
([AD-012](012-distributed-event-bus.md)). Unlike the audit log (raw
events for compliance), activities are curated for humans — they
aggregate related events (a push that updates 50 blocks produces one
`item.pushed` activity, not 50 `block.updated` activities).

### Activity feed by role

The UI defaults a role-based filter when a user opens the feed, and
users can always override. Defaults:

| Role       | Default filter            |
| ---------- | ------------------------- |
| PM / Admin | All                       |
| Developer  | Technical (connectors, flows, pushes, streams) |
| Translator | My work + content         |
| Reviewer   | Review pipeline           |
| Observer   | Milestones only           |

### Tasks

```sql
CREATE TABLE tasks (
    id              TEXT PRIMARY KEY,
    workspace_id    TEXT NOT NULL,
    project_id      TEXT NOT NULL,
    stream          TEXT NOT NULL DEFAULT 'main',
    type            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open',
    priority        TEXT NOT NULL DEFAULT 'normal',
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    assignee_id     TEXT NOT NULL DEFAULT '',
    created_by      TEXT NOT NULL,
    completed_by    TEXT NOT NULL DEFAULT '',
    data            JSONB NOT NULL DEFAULT '{}',
    due_at          TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);
```

Task types:

| Type              | What it means                                   |
| ----------------- | ----------------------------------------------- |
| `translate`       | Translate blocks for a locale                   |
| `review`          | Review translated blocks                        |
| `review_terms`    | Decide on term candidates in the review queue   |
| `source_review`   | Inspect source content before language fan-out  |
| `fix_quality`     | Resolve QA issues                               |
| `fix_brand_voice` | Resolve brand voice violations                  |
| `fix_terminology` | Resolve terminology violations                  |
| `connector_setup` | Configure a connector                           |
| `custom`          | Free-form                                       |

Task lifecycle: `open` → `in_progress` → (`done` | `cancelled`). A
`blocked` status is available for explicit dependencies.

Tasks originate from three sources: manual creation by a project
manager, automation actions (`create_review_tasks`,
`create_source_review`), or system-generated (quality gate failures,
brand voice drift, extraction completion).

### Source review gate

Fixing a source-language issue once per locale is expensive. An
optional `TaskSourceReview` task sits between automation completion and
language fan-out:

```
push.completed
  → ai_translate, auto_extract run
  → PushCompletionTracker waits for all jobs
    → push.automations.completed emitted

Option A — direct fan-out:
  push.automations.completed
    → create_review_tasks
      → one TaskReview per locale, assigned to members with matching scope

Option B — source review gate:
  push.automations.completed
    → create_source_review
      → single TaskSourceReview for a source reviewer
      → reviewer completes task
        → source.review.completed
          → create_review_tasks (same fan-out)
```

The source reviewer inspects placeholder correctness, terminology
consistency, DNT identification, and context notes before language
fan-out. `HandleCompleteTask` emits `source.review.completed` when the
task closes, re-entering the automation engine.

### PushCompletionTracker

A server component bridges individual job completion and collective
push readiness:

- Subscribes to `push.completed`.
- Polls `JobStore.ListJobsByPushID()` and
  `ExtractionJobStore.ListByPushID()` every five seconds.
- When all jobs reach terminal status (completed or failed) it emits
  `push.automations.completed` carrying `push_id`, `items`,
  `workspace_slug`, `project_id`, `translation_status`, and
  `extraction_status`.
- Emits immediately if the project has neither AI translation nor
  extraction enabled (0 jobs).
- Times out after 30 minutes with `status: timeout`.

### Task fan-out actions

Two automation actions ([AD-013](013-automation-engine.md)) drive
fan-out:

**`create_review_tasks`** — for each target locale, find project
members with matching language scope and `PermReview` or `PermTranslate`,
create one task per (locale, assignee). If no member is assigned to a
locale, create an unassigned task and notify project admins. Config:
`mode` (`review` or `translate`), `priority`.

**`create_source_review`** — create a single source review task.
Config: `reviewer` (user ID or fallback to the first member with
`PermEditSource`).

### Notifications

The existing notification store extends with categories, grouping, and
multi-channel preferences.

```sql
CREATE TABLE notification_preferences (
    user_id         TEXT NOT NULL,
    workspace_id    TEXT NOT NULL,
    category        TEXT NOT NULL,
    channel_web     BOOLEAN NOT NULL DEFAULT TRUE,
    channel_email   BOOLEAN NOT NULL DEFAULT FALSE,
    channel_push    BOOLEAN NOT NULL DEFAULT FALSE,
    channel_desktop BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (user_id, workspace_id, category)
);
```

Categories: `task`, `review`, `quality`, `automation`, `mention`,
`project`, `system`.

### Channels

| Channel   | Transport                                                       |
| --------- | --------------------------------------------------------------- |
| Web       | WebSocket via the existing `notificationHub` + cross-instance Redis Pub/Sub |
| Desktop   | gRPC `WatchProject` stream → Wails event → OS notification      |
| Email     | Immediate or digest via the Mailer                              |
| Push      | FCM / APNs via a `PushService` abstraction and `push_tokens` table |

The `NotificationDispatcher` resolves recipients (project membership +
assignee + watchers), checks per-user preferences per category,
persists the notification, and fans out to enabled channels.

### Role-based defaults

Notification defaults are seeded from the user's project role when they
first join:

| Notification type      | PM / Admin          | Developer           | Translator          | Reviewer            |
| ---------------------- | ------------------- | ------------------- | ------------------- | ------------------- |
| `task.assigned`        | web                 | web                 | web, email, desktop | web, email, desktop |
| `task.overdue`         | web, email          | web                 | web, email, desktop | web, email, desktop |
| `review.assigned`      | web                 | —                   | —                   | web, email, desktop |
| `quality.gate.failed`  | web, email          | web, email          | web                 | web, desktop        |
| `brand.drift`          | web, email          | —                   | web                 | web, desktop        |
| `mention`              | web, email, desktop | web, email, desktop | web, email, desktop | web, email, desktop |
| `flow.failed`          | web, email          | web, email          | —                   | —                   |
| `content.available`    | web                 | —                   | web, email, desktop | web, desktop        |
| `progress.milestone`   | web, email          | web                 | web                 | web                 |
| `quota.warning`        | web, email          | —                   | —                   | —                   |

Users override per category.

### Digest and quiet hours

A `DigestWorker` runs daily (and weekly for executives). For each user
with `digest_frequency != off` it fetches unread notifications since
`last_sent_at`, groups by category, renders the template, sends via
Mailer, and updates `digest_state`.

Smart delivery rules:

- **Seen on web, skip email** — if the batch is all read, skip. If the
  user has been actively connected in the last hour, defer non-urgent
  emails.
- **Quiet hours** — per-user quiet window with timezone; non-urgent
  email, push, and desktop are queued until it ends. Urgent
  (`priority: high`) always delivers.
- **Escalation** — `task.assigned` (normal) → `task.due_soon` (email
  even if off for the category) → `task.overdue` (`priority: high`,
  immediate email). Connector errors escalate on three consecutive
  failures.
- **Auto-mute resolved issues** — when a gate that previously failed
  now passes, related failure notifications mark as read. When a task
  completes, its `due_soon` and `overdue` notifications auto-dismiss.
- **@mentions** — comments and notes scan for `@username`, resolve,
  and dispatch the `mention` notification type, always via web, email,
  and desktop.

### MCP integration

The bravo agent ([AD-016](016-bravo-agent.md)) and other persona agents
consume tasks through MCP tools: `list_my_tasks`, `claim_task`,
`complete_task`. Agents appear as project members with language scope
and the appropriate permission, so task assignment treats them
identically to human translators.

### UI

- **Activity Feed** — timeline appearing on project dashboards, the
  workspace overview, and block detail. Role-based default filter.
- **Task Board** — Kanban by status, drill-down to editor. Accessible
  from the project sidebar, the "My Tasks" cross-project view, and the
  dashboard.
- **Notification Panel** — grouped cards with category badges, inline
  actions ("Go to task", "Open in editor"), and direct access to
  preferences.

All three are shared React components in `packages/ui/` so the web app,
desktop app, and future mobile surfaces render the same primitives.

## Consequences

- Translator engagement is deterministic — tasks appear exactly when
  prep work finishes, not when content arrives.
- Source review prevents duplicated fixes and upgrades the quality of
  downstream translations.
- Notifications reach the right channel without flooding inboxes.
  Preferences, quiet hours, and digest batching keep alert fatigue
  low.
- Role-based defaults make the system usable without configuration; the
  first time a user opens their inbox they see only what matters for
  their role.
- Agents and humans share one task queue, so hybrid teams scale without
  special cases.

## Related

- [AD-012: Distributed Event Bus](012-distributed-event-bus.md) — event substrate
- [AD-013: Automation Engine](013-automation-engine.md) — action types, run visibility
- [AD-015: Server-Side AI Operations](015-server-ai-operations.md) — job completion feeds the tracker
- [AD-016: Bravo Agent](016-bravo-agent.md) — MCP consumer of tasks
- [Translator Workflow](/docs/notes/translator-workflow) — detailed algorithms
