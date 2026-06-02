---
id: 013-automation-engine
sidebar_position: 13
title: "AD-013: Automation Engine"
---

# AD-013: Automation Engine

## Summary

Bowrain's server-side automation engine evaluates YAML-defined rules
against events on the distributed event bus
([AD-012](012-distributed-event-bus.md)) and dispatches actions — flow
execution, AI translation, entity extraction, task creation, webhook
delivery, Bravo invocations. A `RunManager` groups the actions from each
triggering event into an `AutomationRun` with per-step status and
structured logs, visible via REST and streamed over SSE.

## Context

Localization pipelines fail silently when reactive behavior is absent:
content changes in a CMS but translations do not update; quality checks
are forgotten; terminology violations accumulate. Polling-based
schedulers react too slowly and over-consume resources. Event-driven
automation — reacting to what actually happened — closes this gap, but
only if users can see what the automations did. A run visibility layer
is part of the automation engine, not a separate feature.

## Decision

### Rules

A rule matches an event type with optional conditions and produces an
ordered list of actions.

```yaml
automations:
  - name: auto-translate-on-extract
    on: content.extracted
    conditions:
      project: "website-*"
    actions:
      - type: ai_translate
        config:
          target_locales: [fr, de, ja]

  - name: qa-on-translation
    on: translation.updated
    conditions:
      origin: ["ai", "mt"]
    actions:
      - type: run_flow
        config:
          flow: qa-check
      - type: notify
        config:
          channel: "#localization"
          on_failure: true
```

Conditions use CEL expressions (`event.payload.blocks_changed > 0`,
`event.source.startsWith("connector:")`). The rule store (SQLite or
PostgreSQL) is mutated through the REST API and a visual editor in
the web app. Rules are project-scoped and can be toggled without
deletion.

### Default rules

Every project is created with a set of built-in rules:

| Rule                          | Trigger                       | Action                                           |
| ----------------------------- | ----------------------------- | ------------------------------------------------ |
| `auto-translate-on-push`                      | `push.completed`              | Create translation jobs per (item, locale) pair  |
| `auto-extract-on-push`                        | `push.completed`              | Run entity/term extraction on changed blocks     |
| `auto-translate-new-locale`                   | `project.updated`             | Translate all items when a locale is added       |
| `create-review-tasks-on-automation-complete`  | `push.automations.completed`  | Per-locale review tasks (see AD-014)             |
| `fan-out-after-source-review`                 | `source.review.completed`     | Fan out per-locale review tasks after source review (see AD-014) |

Projects opt out by disabling individual rules or setting
`auto_translate: false` in project properties.

### Actions

```
run_flow         # execute a flow in the project's flow registry
ai_translate     # enqueue async translation jobs (AD-015)
mt_translate     # enqueue MT-backed translation jobs
extract          # run entity/term extraction (AD-015)
publish          # push via a connector
notify           # dispatch a notification (AD-014)
webhook          # send signed HTTP POST to an external URL
run_bravo        # invoke the Bravo agent (AD-016)
write_overlay    # upsert a targets/annotations/plugin overlay on a block
```

Actions execute server-side. Each action has a typed config object; the
action executor receives the event payload plus the config and drives
the underlying subsystem.

### In-process BlockStore

Actions that touch project content (`run_flow`, `write_overlay`,
future `import_overlay`) reach the blockstore through
`Server.OpenBlockstore(projectID, stream)`. This is the in-process
adapter from [AD-004](004-content-store.md) that maps
`blockstore.Store` onto the Bowrain content store — the automation
engine shares one `Store` interface with the kapi CLI's local flows,
so rule-driven `ai_translate` lands the same
`translations` / `annotations` rows a CLI `kapi run translate` does.
No HTTP loops through the REST API from inside the engine.

### Quality gates

Quality gates are rules whose action fails the triggering workflow. A
gate specifies a check (a flow that produces pass/fail per block), a
threshold, and a scope:

```yaml
quality_gates:
  - name: terminology-compliance
    type: blocking      # or advisory
    check: term-enforce
    threshold: 0.95
    scope: per-project
    applies_to: [push, export]

  - name: qa-pass-rate
    type: advisory
    check: qa-check
    threshold: 0.90
    scope: per-document
    applies_to: [push]
```

Blocking gates hold up the protected operation (push, export,
publish) until the pass rate clears the threshold. Advisory gates warn
in the UI and activity feed but do not block.

### Loop prevention

Every event carries a `CausationID` that traces the lineage back to the
originating event. Three safeguards keep rules from cascading:

1. **Causation chain** — if an event's lineage already includes a rule
   that matches the current event, the duplicate trigger is suppressed.
2. **Max depth** — a configurable depth limit (default 5) stops
   runaway cascades regardless of causation.
3. **Global pause** — rules can be paused globally or individually for
   bulk imports, debugging, or emergency stops.

### Webhooks

External notifications sign the payload with HMAC-SHA256:

```yaml
webhooks:
  - url: https://hooks.slack.com/services/...
    events: [quality.gate.failed, flow.completed]
    secret: ${WEBHOOK_SECRET}
```

Delivery retries with exponential backoff (three attempts) before the
failure is logged and surfaced as an `automation` notification.

### Run visibility

An `AutomationRun` groups every action executed in response to one
triggering event. When `push.completed` fires and matches three rules,
one run opens with three steps.

| Concept            | Stores                                                                       |
| ------------------ | ---------------------------------------------------------------------------- |
| `AutomationRun`    | `automation_runs` — trigger type, trigger event ID, status, step counts, timing |
| `AutomationStep`   | `automation_steps` — rule name, action type, spawned job IDs, per-step status |
| `AutomationLog`    | `automation_logs` — structured info/warn/error entries written by workers     |

Run statuses: `pending`, `running`, `completed`, `failed`, `partial`.
Step statuses: `pending`, `running`, `completed`, `failed`, `skipped`.

### RunManager

The `RunManager` wraps the action executor. `AutomationEngine` continues
to evaluate rules event-by-event; the manager groups each event's
actions into a single run using a short debounce window.

```go
type AutomationRunManager struct {
    store    AutomationRunStore
    executor ActionExecutor
    bus      EventBus
}
```

On each `Execute(action, event)`:

1. Find or create the run for `event.ID`.
2. Create a step for the action.
3. Call the real action executor.
4. Update step status based on the result.
5. Record spawned job IDs on the step (for async actions).
6. A background goroutine finalizes the run when all steps finish.

### Step completion tracking

Async steps (AI translation, entity extraction) spawn jobs that finish
later. A `StepCompletionTracker` watches the relevant job stores and
updates per-step status. The separate `PushCompletionTracker` used by
the translator workflow ([AD-014](014-translator-workflow.md)) tracks
push-level completion; both trackers coexist because they answer
different questions.

### Log capture

A `RunLogger` passed through context buffers structured log entries and
flushes them in batches to `automation_logs`. Worker jobs gain a
`StepID` field so their logs route to the parent step. Standard
milestones are:

- `"Processing <item_name> for <locale>"` (info)
- `"Translating blocks <start>-<end> of <total>"` (info, per chunk)
- `"Chunk completed: <tokens> tokens"` (info)
- `"Job completed: <total_blocks> blocks, <tokens_used> tokens"` (info)
- `"Translation failed: <error>"` (error)

### Real-time delivery

```
GET /api/v1/:ws/:proj/automations/runs/:runId/events    # SSE
```

The stream emits typed events: `step_started`, `step_progress`,
`step_completed`, `step_failed`, `run_completed`, `log`. SSE is
preferred over WebSocket because it reconnects on its own, works
through proxies, and fits one-way progress streaming.

### REST API

```
GET    /:ws/:proj/automations                      # rules
POST   /:ws/:proj/automations
PUT    /:ws/:proj/automations/:rid
DELETE /:ws/:proj/automations/:rid
PATCH  /:ws/:proj/automations/:rid/toggle
GET    /:ws/:proj/automations/events               # event types catalog
GET    /:ws/:proj/automations/history              # per-rule execution history

GET    /:ws/:proj/automations/runs                 # run list
GET    /:ws/:proj/automations/runs/:runId
GET    /:ws/:proj/automations/runs/:runId/steps
GET    /:ws/:proj/automations/runs/:runId/steps/:stepId/logs
POST   /:ws/:proj/automations/runs/:runId/cancel
GET    /:ws/:proj/automations/runs/:runId/events   # SSE
```

### UI

The automation run UI mirrors GitHub Actions:

- **Run list** — status badges, trigger summary ("Content pushed:
  en.json"), duration, auto-refresh.
- **Run detail** — step graph with per-step cards, progress bars for
  async steps (142/418 blocks), duration, expandable log viewer.
- **Log viewer** — structured entries with timestamps, levels, and
  metadata, drillable to the underlying job or task.

### CLI hooks complement server automation

Local automation hooks declared on the project's `.kapi` recipe
([AD-010](010-bowrain-cli-and-project-model.md)) are the *client-side*
counterpart. They coordinate local commands around sync boundaries
(`pre-push`, `post-pull`, `wait_translate`, …). The server engine
handles multi-step orchestration across connectors, flows, and quality
gates. The two are complementary:

- **Server automation** — asynchronous, event-driven, organization-wide.
- **CLI hooks** — synchronous, sync-boundary, local-to-the-developer.

## Consequences

- Users see exactly what automations are doing. Debugging is
  self-service.
- The run/step/log hierarchy supports drill-down from "something is
  happening" to "here is the exact worker log line".
- The same automation engine powers AI translation, extraction, task
  creation, notifications, and Bravo invocations, avoiding parallel
  orchestrators.
- Structured logs enable future features: search, error aggregation,
  performance monitoring, cost attribution.
- Loop prevention keeps reactive behavior safe at scale.

## Related

- [AD-012: Distributed Event Bus](012-distributed-event-bus.md) — delivery substrate
- [AD-014: Translator Workflow](014-translator-workflow.md) — `push.automations.completed` and task actions
- [AD-015: Server-Side AI Operations](015-server-ai-operations.md) — spawned translation and extraction jobs
- [AD-016: Bravo Agent](016-bravo-agent.md) — `run_bravo` action
- [AD-010: Bowrain CLI and Project Model](010-bowrain-cli-and-project-model.md) — local automation hooks
- [Automation Run Visibility](/notes/automation-run-visibility) — schema, tracker details
