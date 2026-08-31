---
title: Event System
sidebar_position: 13
---

# Event System and Automation

The event system provides pub/sub for reacting to content changes, triggering
automation rules, and delivering webhooks. In a deployment the bus runs on
Redis Streams (`BOWRAIN_EVENT_BACKEND=redis`), shared by the server and the
worker; the in-process `ChannelEventBus` below backs single-process
development and tests.

## EventBus

The `ChannelEventBus` is a channel-based pub/sub implementation with per-subscriber goroutines:

```go
bus := event.NewChannelEventBus()

// Subscribe to specific event types
sub := bus.Subscribe(platev.EventBlockUpdated, func(e platev.Event) {
    fmt.Printf("Block %s updated in project %s\n", e.Data["block_id"], e.ProjectID)
})

// Subscribe to all events
allSub := bus.SubscribeAll(func(e platev.Event) {
    fmt.Printf("Event: %s\n", e.Type)
})

// Unsubscribe
bus.Unsubscribe(sub)
```

The event types are declared in `bowrain/core/event` (imported as `platev`
above); the bus, the emitting store decorator, the automation engine and
webhook delivery live in `bowrain/event`.

## Event Types

| Event                        | Emitted when                                   |
| ---------------------------- | ---------------------------------------------- |
| `block.created`              | A block is stored for the first time           |
| `block.updated`              | A block is updated                             |
| `block.deleted`              | A block is deleted                             |
| `project.created`            | A project is created                           |
| `project.updated`            | A project is updated                           |
| `project.deleted`            | A project is deleted                           |
| `version.created`            | A version snapshot is created                  |
| `collection.created` / `collection.updated` / `collection.deleted` | A collection changes |
| `item.created` / `item.deleted` | An item is added or removed                 |
| `connector.pull.completed`   | A pull from a connector completes              |
| `connector.push.completed`   | A push completes                               |
| `connector.sync.completed`   | A connector sync completes                     |
| `push.automations.completed` | Every automation for a push has completed      |
| `convergence.run.completed`  | A run finishes                                 |
| `flow.started`               | A flow begins execution                        |
| `flow.completed` / `flow.failed` | Declared; no execution path emits them, so no automation trigger fires on a flow finishing |
| `extraction.completed`       | Term extraction completes                      |
| `quality.gate.pass` / `quality.gate.fail` | A quality gate is evaluated         |
| `source.review.completed`    | A source review task is completed              |
| `review.completed`           | A project's review queue is emptied            |
| `voice.check.started` / `voice.check.completed` | A voice check runs            |
| `voice.gate.passed` / `voice.gate.failed` | A voice gate is evaluated           |
| `voice.drift` / `voice.corrected` / `voice.profile.updated` | The voice loop moves |
| `stream.created` / `stream.merged` / `stream.deleted` / `stream.locked` / `stream.unlocked` / `stream.tagged` | A stream changes |
| `member.*`, `role.template.*`, `invite.*`, `token.*`, `auth.*`, `session.grant.created`, `authz.denied` | Membership and access |
| `rollback.performed`, `platform_config.changed` | Administration               |
| `agent.*`                    | The in-product agent (dark by default)         |

The canonical list is the `EventType` constants in
`bowrain/core/event/event.go`. The automation editor offers only the types an
execution path emits.

## EventEmittingStore

The `EventEmittingStore` decorator wraps a `ContentStore` and emits events on all mutations:

```go
cs, err := sqlitestore.NewSQLiteStore("working-copy.db")
if err != nil {
    log.Fatal(err)
}
bus := event.NewChannelEventBus()
emittingStore := event.NewEventEmittingStore(cs, bus)
```

## Automation Rules

The automation engine evaluates rules triggered by events. It takes the bus and
an `ActionExecutor`, which runs a rule's actions (`run_flow`, `notify`, and
the rest) when a rule matches:

```go
engine := event.NewAutomationEngine(bus, executor)
engine.AddRule(event.AutomationRule{
    Name:      "auto-draft-on-push",
    EventType: platev.EventPushCompleted,
    Conditions: []event.Condition{
        {Field: "project_id", Operator: "equals", Value: "proj-1"},
    },
    // Actions are executed by the ActionExecutor.
})
engine.Start(ctx)
```

### Loop Prevention

Automation chains are tracked via `CausationID`. If a chain exceeds the maximum depth (default 5), it is automatically broken to prevent infinite loops.

## Quality Gates

Quality gates evaluate content quality and can block or advise:

```go
gates := []event.QualityGate{
    {
        Name:      "min-coverage",
        Type:      event.GateBlocking,
        Threshold: 0.9,
        Evaluate: func(projectID string) (float64, error) {
            // Return coverage score
            return 0.95, nil
        },
    },
}

results, err := event.EvaluateGates(gates, projectID)
```

## Webhooks

Webhook delivery with HMAC-SHA256 signing and retry is a library primitive in
`bowrain/event`; no workspace-facing surface configures outbound webhooks.

```go
wh := event.WebhookDelivery{
    URL:    "https://example.com/webhook",
    Secret: "shared-secret",
}
err := wh.Deliver(ctx, eventData)
```

Signature verification on the receiving end:

```go
valid := event.VerifySignature(payload, signature, secret)
```
