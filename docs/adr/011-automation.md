---
id: 011-automation
sidebar_position: 11
title: "ADR-011: Automation and Event System"
---
# ADR-011: Automation and Event System

## Context

Manual localization workflows are error-prone and brittle. Content changes in a
CMS but translations are not updated. Quality checks are forgotten. Terminology
violations accumulate silently until they reach production. A translator finishes
work but nobody pushes it back to the source system. These failures are not
caused by bad tools — they are caused by the absence of reactive behavior.

The key insight is that automation should be event-driven rather than
scheduled — reacting to what actually happened rather than polling on a timer.
Quality checks should run when content changes. Compliance rules should be
enforced before data ships.

Gokapi needs the same pattern. When source content is extracted from a connector,
translation should start automatically. When a machine translation is inserted,
quality checks should run. When terminology changes, affected translations should
be flagged for review. This reactive automation complements the bidirectional
connector system ([ADR-005](./005-connector-system.md)) by adding behavior on
top of integration — connectors move content, automation decides what to do when
content moves.

The streaming pipeline ([ADR-004](./004-processing-engine.md))
already provides the execution substrate: flows process content through tool
chains. Automation rules simply trigger those flows in response to events, adding
an orchestration layer above the existing pipeline.

## Decision

### Event Types

The platform emits events when significant state changes occur. Events are the
atoms of automation — each one represents something that happened, not something
that should happen.

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

### Event Bus Interface

```go
type EventBus interface {
    Publish(ctx context.Context, event Event) error
    Subscribe(ctx context.Context, eventType EventType, handler EventHandler) (Subscription, error)
    Unsubscribe(sub Subscription) error
}

type EventHandler func(ctx context.Context, event Event) error
```

The local implementation uses a channel-based in-process event bus — consistent
with gokapi's local-first design ([ADR-004](./004-processing-engine.md)).
For distributed deployments, the bus can be backed by Redis Streams or NATS
without changing the interface.

### Automation Triggers

YAML-based trigger rules define reactive behavior. Rules live in
`gokapi.yaml` or `.gokapi/automations.yaml` and are scoped per project:

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

Each automation rule specifies an event trigger (`on`), optional conditions that
filter which events match, and a sequence of actions to execute. Actions are
primarily flow executions — the same flows that run from the CLI or Bowrain —
but can also include notifications and webhook calls.

### Quality Gates

Quality gates are blocking checks that prevent content from progressing through
a workflow. They enforce standards before translations ship.

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

Gates are configured in YAML alongside automations:

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
- **terminology-compliance**: Block `push` if more than 5% of blocks use
  forbidden terms (blocking)
- **qa-pass-rate**: Warn if QA check fails on more than 10% of blocks (advisory)
- **review-coverage**: Block export if fewer than 80% of blocks are reviewed
  (blocking)

### Continuous Sync

Connectors ([ADR-005](./005-connector-system.md)) can be configured for
continuous sync — periodically pulling content and pushing translations:

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

Continuous sync bridges the gap between event-driven automation (reactive) and
time-based polling (proactive). The pull side detects changes; once detected, it
emits `content.extracted` events that feed into the automation rules above.

### Loop Prevention

Automation rules can trigger events that trigger more rules. Without safeguards,
this creates infinite cascading. Loop prevention works through three mechanisms:

1. **Causation chain**: Each event carries a `CausationID` tracking its lineage
   back to the originating event. If an event's lineage includes the same
   automation rule, the duplicate trigger is suppressed.
2. **Maximum chain depth**: A configurable depth limit (default: 5) prevents
   unbounded cascading regardless of causation tracking.
3. **Global pause**: Automations can be paused globally or individually for
   debugging, bulk imports, or emergency stops.

### Webhook Support

External systems can receive event notifications via outbound webhooks,
bridging gokapi's internal event bus to external services:

```yaml
webhooks:
  - url: https://hooks.slack.com/services/...
    events: [quality.gate.failed, flow.completed]
    secret: ${WEBHOOK_SECRET}
```

Webhooks use HMAC-SHA256 signing so receivers can verify authenticity. Failed
deliveries are retried with exponential backoff (3 attempts, then logged).

## Alternatives Considered

- **Cron-only scheduling**: Simple but lacks event reactivity. Content changes
  between polling intervals go unprocessed until the next tick. Real-time
  response to content changes is critical for continuous localization.
- **Full workflow engine (Temporal/Cadence)**: Powerful but over-engineered for
  localization triggers. Adds significant infrastructure. Gokapi's flows already
  provide execution; automation only needs to decide when to run them.
- **Plugin-based automation**: Harder to configure and audit. YAML rules are
  readable by non-developers and can be version-controlled alongside project
  configuration.
- **No quality gates**: Allows bad translations to ship. Gates are essential for
  professional workflows where terminology compliance and review coverage are
  non-negotiable.

## Consequences

- Content changes trigger automatic processing, reducing manual overhead and
  eliminating the "forgot to re-translate" failure mode
- Quality gates enforce standards before content ships, catching terminology
  violations and insufficient review coverage
- YAML configuration is accessible to project managers and localization
  engineers, not just developers
- The event bus is an internal coordination mechanism; webhooks bridge to
  external systems (Slack, CI/CD, monitoring)
- Loop prevention avoids infinite automation cascades while allowing legitimate
  multi-step chains
- Local event bus (channel-based) works for single-machine deployments;
  swappable to Redis Streams or NATS for distributed setups
  ([ADR-004](./004-processing-engine.md))
- Automation rules are project-scoped, enabling different workflows per project
  without cross-contamination
- Continuous sync closes the loop between connectors and automation — pull
  changes, process automatically, push when ready
