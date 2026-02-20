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
- YAML-based automation rules
- Quality gates (blocking checks)
- Continuous sync with connectors
- Webhook notifications

**Kapi** provides simple flow hooks:
- Pre-push hooks (run before `kapi push`)
- Post-pull hooks (run after `kapi pull`)
- Defined in `.kapi/config.yaml`
- Execute local flows (e.g., QA check, term enforce)

**Clear separation:**
- **Server automation** = orchestrate complex multi-system workflows
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

### Kapi Flow Hooks (Client-Side)

Kapi provides simple flow hooks in `.kapi/config.yaml` ([AD-016](./016-kapi-project-model.md)):

```yaml
# .kapi/config.yaml
hooks:
  pre-push: [qa-check, term-enforce]
  post-pull: [update-stats]
```

**Hook execution:**
- `kapi push` → runs `pre-push` flows → sends blocks to server
- `kapi pull` → fetches blocks from server → runs `post-pull` flows

**Differences from server automation:**
- **No event bus** — hooks are simple command chains
- **Local file processing** — flows run on local files, not server-side
- **Synchronous** — hooks block the push/pull operation until complete
- **No conditions** — hooks always run (or can be skipped with `--no-hooks`)

Example use cases:
- `pre-push`: Run QA checks, enforce terminology before sending to server
- `post-pull`: Update local statistics, generate reports after fetching translations

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

- **Clear separation**: Server = orchestration platform; Kapi = local file tool with basic hooks.

- Kapi users can define simple pre/post sync hooks without needing to understand the server's automation system.

- The automation model positions Bowrain Server as a **localization platform** that orchestrates complex workflows, while Kapi remains a focused **file processing tool**.
