---
id: 011-automation
sidebar_position: 11
title: "AD-011: Automation and Event System"
---
# AD-011: Automation and Event System

## Context

Manual localization workflows are error-prone and brittle. Content changes in a CMS but translations are not updated. Quality checks are forgotten. Terminology violations accumulate silently until they reach production. A translator finishes work but nobody pushes it back to the source system. These failures are not caused by bad tools — they are caused by the absence of reactive behavior.

The key insight is that automation should be event-driven rather than scheduled — reacting to what actually happened rather than polling on a timer. Quality checks should run when content changes. Compliance rules should be enforced before data ships.

**This AD establishes the automation architecture for Bowrain Server.** Automation is a **server-side concern** — it orchestrates multi-step workflows across connectors, flows, and quality gates. This is distinct from Kapi's simpler **flow hooks** ([AD-016](./016-kapi-project-model.md)), which run local tools before/after sync operations.

## Decision

### Automation Scope

**Bowrain Server** provides full event-driven automation:
- Event bus for system-wide events
- Automation rules with visual editor UI and YAML backing
- Built-in default rules for common patterns
- Quality gates (blocking checks)
- Continuous sync with connectors
- Webhook notifications
- GitHub Action for CI/CD integration

**Bowrain CLI** provides local automation hooks:
- Six trigger points: `pre-push`, `post-push`, `pre-pull`, `post-pull`,
  `pre-flow`, `post-flow`
- Four action types: `run_flow`, `wait_translate`, `pull`, `push`
- Defined in `.bowrain/config.yaml` under `automations`
- Execute local flows or coordinate with server

**Kapi** provides simple flow hooks:
- Pre-push hooks (run before sync)
- Post-pull hooks (run after sync)
- Execute local flows (e.g., QA check, term enforce)

**Clear separation:**
- **Server automation** = orchestrate complex multi-system workflows
- **Bowrain CLI hooks** = coordinate local+server operations around sync
- **Kapi hooks** = run local file processing before/after sync

### Event Types (Server-Side)

The Bowrain Server platform emits events when significant state changes occur. Events are the atoms of automation — each one represents something that happened, not something that should happen.

```go
type Event struct {
    ID          string
    Type        EventType
    Source      string      // connector ID, tool name, or "user"
    ProjectID   string
    CausationID string      // ID of the event that caused this one (loop prevention)
    Payload     any
    Timestamp   time.Time
}

type EventType string

const (
    EventContentChanged      EventType = "content.changed"       // source content modified
    EventContentExtracted    EventType = "content.extracted"     // new content pulled from connector
    EventTranslationUpdated  EventType = "translation.updated"  // translation added or modified
    EventTranslationReviewed EventType = "translation.reviewed"  // translation marked as reviewed
    EventConnectorSynced     EventType = "connector.synced"     // connector pull/push completed
    EventFlowCompleted       EventType = "flow.completed"       // processing flow finished
    EventQualityGateFailed   EventType = "quality.gate.failed"  // quality gate blocked an action
    EventTerminologyChanged  EventType = "terminology.changed"  // termbase updated
    EventPushCompleted       EventType = "push.completed"       // content pushed via sync API
    EventProjectUpdated      EventType = "project.updated"      // project config changed (e.g., new locales)
)
```

### Event Bus Interface (Server-Side)

```go
type EventBus interface {
    Publish(ctx context.Context, event Event) error
    Subscribe(ctx context.Context, eventType EventType, handler EventHandler) (Subscription, error)
    Unsubscribe(sub Subscription) error
}

type EventHandler func(ctx context.Context, event Event) error
```

The local implementation uses a channel-based in-process event bus. For distributed deployments, the bus can be backed by Redis Streams or NATS without changing the interface.

### Automation Rules (Server-Side)

YAML-based trigger rules define reactive behavior. Rules live in Bowrain Server's project configuration and are managed through the admin UI or API:

```yaml
automations:
  - name: auto-translate-on-extract
    on: content.extracted
    conditions:
      project: "website-*"
    actions:
      - flow: ai-translate
        config:
          target_locales: [fr, de, ja]

  - name: qa-on-translation
    on: translation.updated
    conditions:
      origin: ["ai", "mt"]   # only for machine translations
    actions:
      - flow: qa-check
      - notify: slack
        config:
          channel: "#localization"
          on_failure: true

  - name: continuous-sync
    on: connector.synced
    conditions:
      connector: contentful-main
    actions:
      - flow: tm-leverage
      - flow: ai-translate
        config:
          skip_if: "translation-origin == 'human'"
```

Each automation rule specifies an event trigger (`on`), optional conditions that filter which events match, and a sequence of actions to execute. Actions are primarily **server-side flow executions** (not Kapi flows), but can also include notifications and webhook calls.

### Built-in Default Rules (Server-Side)

Three automation rules are built in and enabled by default on all projects:

| Rule | Trigger | Action |
|------|---------|--------|
| `auto-translate-on-push` | `push.completed` | Create translation jobs for each (item, locale) pair pushed |
| `auto-extract-on-push` | `push.completed` | Run entity/term extraction on pushed content |
| `auto-translate-new-locale` | `project.updated` | Translate all items when new target locales are added |

Built-in rules use the platform AI provider (Azure OpenAI with Managed
Identity) and link jobs to the originating `push_id` for traceability.
Projects can opt out via `auto_translate: false` in project properties.

### Automation Rule Persistence and Visual Editor (Server-Side)

Automation rules are persisted in the database via `RuleStore` (SQLite
or PostgreSQL) and managed through REST API endpoints:

- `GET/POST /api/v1/workspaces/:ws/automations` -- list and create rules
- `PUT/DELETE /api/v1/workspaces/:ws/automations/:id` -- update and delete
- `POST /api/v1/workspaces/:ws/automations/:id/toggle` -- enable/disable
- `GET /api/v1/workspaces/:ws/automations/events` -- available event types
- `GET /api/v1/workspaces/:ws/automations/history` -- execution records

The web UI provides a visual rule editor for creating and editing rules
via form-based interaction (event selection, condition filtering, action
configuration). Execution history tracks each rule invocation with
status, errors, and timestamps for debugging.

### Quality Gates (Server-Side)

Quality gates are blocking checks that prevent content from progressing through a workflow. They enforce standards before translations ship.

```go
type QualityGate struct {
    Name      string
    Type      GateType   // blocking or advisory
    Check     string     // flow name to run as check
    Threshold float64    // minimum pass rate (0.0-1.0)
    Scope     GateScope  // per-block, per-document, per-project
}

type GateType string

const (
    GateBlocking GateType = "blocking"  // prevents push/export until passed
    GateAdvisory GateType = "advisory"  // warns but allows progression
)
```

Gates are configured in server project settings:

```yaml
quality_gates:
  - name: terminology-compliance
    type: blocking
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

  - name: review-coverage
    type: blocking
    check: review-status
    threshold: 0.80
    scope: per-project
    applies_to: [export]
```

Example gates:
- **terminology-compliance**: Block `push` if more than 5% of blocks use forbidden terms (blocking)
- **qa-pass-rate**: Warn if QA check fails on more than 10% of blocks (advisory)
- **review-coverage**: Block export if fewer than 80% of blocks are reviewed (blocking)

### Continuous Sync (Server-Side)

Server-side connectors ([AD-005](./005-connector-system.md)) can be configured for continuous sync — periodically pulling content and pushing translations:

```yaml
sync:
  - connector: contentful-main
    interval: 15m
    pull:
      on_change: true      # only pull if content changed (ETag/modified check)
    push:
      auto: false          # require manual push (safety default)
      on_review: true      # auto-push when all blocks reviewed
```

Continuous sync bridges the gap between event-driven automation (reactive) and time-based polling (proactive). The pull side detects changes; once detected, it emits `content.extracted` events that feed into the automation rules above.

**This is server-side only.** Kapi does not run continuous sync — it syncs on demand via `kapi pull/push`.

### Bowrain CLI Local Automation (Client-Side)

Bowrain CLI provides local automation hooks in `.bowrain/config.yaml` that
coordinate client-side actions around sync operations:

```yaml
# .bowrain/config.yaml
automations:
  - trigger: post-push
    actions:
      - type: wait_translate
      - type: pull
  - trigger: pre-push
    actions:
      - type: run_flow
        flow: qa-check
```

**Six trigger types:** `pre-push`, `post-push`, `pre-pull`, `post-pull`,
`pre-flow`, `post-flow`

**Four action types:**
- `run_flow` -- execute a local flow (e.g., QA check)
- `wait_translate` -- poll server until translation jobs from the push
  are complete
- `pull` -- fetch translated content from server
- `push` -- send content to server

The `bowrain sync` command combines push + wait + pull into a single
operation, orchestrating the full round-trip via local automation.

**Differences from server automation:**
- **No event bus** — hooks are trigger-based command chains
- **Coordinates client+server** — can wait for server-side jobs, then pull
- **Synchronous** — hooks block the parent operation until complete
- **No conditions** — hooks always run (or can be skipped with `--no-hooks`)

### Bowrain CLI Flow Hooks (Client-Side)

Bowrain CLI supports flow hooks in `.bowrain/config.yaml`:

```yaml
# .bowrain/config.yaml
hooks:
  pre-push: [qa-check, term-enforce]
  post-pull: [update-stats]
```

**Hook execution:**
- `bowrain push` → runs `pre-push` flows → sends blocks to server
- `bowrain pull` → fetches blocks from server → runs `post-pull` flows

**Differences from Bowrain CLI automation rules:**
- **Local file processing only** — no `wait_translate` or server coordination
- **Simpler model** — two trigger points, flow names only
- **Synchronous** — hooks block the push/pull operation until complete

Note: Kapi is a standalone file-processing tool with no project model or push/pull commands, so it does not provide hooks.

### GitHub Action for CI/CD (Client-Side)

The `gokapi/bowrain-action` GitHub Action brings Bowrain sync into CI/CD
pipelines:

1. `gokapi/setup-bowrain@v1` installs the Bowrain CLI with platform
   detection, checksum verification, and GitHub Actions caching
2. `gokapi/bowrain-action@v1` runs `bowrain sync` (push → wait → pull),
   commits translated files, and pushes to the repository

This enables fully automated translation pipelines: a developer pushes
source content changes, the GitHub Action syncs with Bowrain Server,
waits for AI translation, pulls results, and commits them back — all
without manual intervention.

### Loop Prevention (Server-Side)

Automation rules can trigger events that trigger more rules. Without safeguards, this creates infinite cascading. Loop prevention works through three mechanisms:

1. **Causation chain**: Each event carries a `CausationID` tracking its lineage back to the originating event. If an event's lineage includes the same automation rule, the duplicate trigger is suppressed.
2. **Maximum chain depth**: A configurable depth limit (default: 5) prevents unbounded cascading regardless of causation tracking.
3. **Global pause**: Automations can be paused globally or individually for debugging, bulk imports, or emergency stops.

### Webhook Support (Server-Side)

External systems can receive event notifications via outbound webhooks, bridging Bowrain Server's internal event bus to external services:

```yaml
webhooks:
  - url: https://hooks.slack.com/services/...
    events: [quality.gate.failed, flow.completed]
    secret: ${WEBHOOK_SECRET}
```

Webhooks use HMAC-SHA256 signing so receivers can verify authenticity. Failed deliveries are retried with exponential backoff (3 attempts, then logged).

## Alternatives Considered

- **Cron-only scheduling**: Simple but lacks event reactivity. Content changes between polling intervals go unprocessed until the next tick. Real-time response to content changes is critical for continuous localization.

- **Full workflow engine (Temporal/Cadence)**: Powerful but over-engineered for localization triggers. Adds significant infrastructure. Bowrain's flows already provide execution; automation only needs to decide when to run them.

- **Plugin-based automation**: Harder to configure and audit. YAML rules are readable by non-developers and can be version-controlled alongside project configuration.

- **No quality gates**: Allows bad translations to ship. Gates are essential for professional workflows where terminology compliance and review coverage are non-negotiable.

- **Client-side automation in Kapi**: Would require Kapi to run as a daemon, manage event subscriptions, and coordinate complex workflows. This overcomplicates Kapi's role. Kapi is a CLI tool for local file work; the server orchestrates automation.

## Consequences

- **Server-side automation** is full-featured — events, rules, quality gates, continuous sync, webhooks. This enables complex multi-system workflows (CMS → TM → AI → QA → push).

- **Kapi hooks** are simple local tool chains — run flows before/after sync. No event bus, no complex orchestration.

- Content changes trigger automatic processing on the server, reducing manual overhead and eliminating the "forgot to re-translate" failure mode.

- Quality gates enforce standards before content ships, catching terminology violations and insufficient review coverage.

- YAML configuration is accessible to project managers and localization engineers, not just developers.

- The event bus is an internal server coordination mechanism; webhooks bridge to external systems (Slack, CI/CD, monitoring).

- Loop prevention avoids infinite automation cascades while allowing legitimate multi-step chains.

- Local event bus (channel-based) works for single-instance deployments; swappable to Redis Streams or NATS for distributed setups.

- Automation rules are project-scoped, enabling different workflows per project without cross-contamination.

- Continuous sync closes the loop between connectors and automation — pull changes, process automatically, push when ready.

- **Three-tier separation**: Server = orchestration platform with event bus and visual rule editor; Bowrain CLI = client-server coordination with `wait_translate` and `sync`; Kapi = local file tool with basic hooks.

- Bowrain CLI users can define local automation that coordinates with the server (push → wait for AI → pull) without needing to configure server-side rules.

- Kapi users can define simple pre/post sync hooks without needing to understand the server's automation system.

- The GitHub Action closes the CI/CD loop, enabling fully automated translation pipelines triggered by source content changes.

- Built-in default rules (auto-translate, auto-extract, auto-translate-new-locale) provide immediate value without configuration. Projects opt out rather than opt in.

- The automation model positions Bowrain Server as a **localization platform** that orchestrates complex workflows, Bowrain CLI as a **sync companion** that coordinates client-server operations, and Kapi as a focused **file processing tool**.
