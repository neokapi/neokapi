---
title: Event System
sidebar_position: 13
---

# Event System and Automation

The event system provides in-process pub/sub for reacting to content changes, triggering automation rules, and delivering webhooks.

## EventBus

The `ChannelEventBus` is a channel-based pub/sub implementation with per-subscriber goroutines:

```go
bus := event.NewChannelEventBus()

// Subscribe to specific event types
sub := bus.Subscribe(event.EventBlockStored, func(e event.Event) {
    fmt.Printf("Block %s stored in project %s\n", e.Data["block_id"], e.ProjectID)
})

// Subscribe to all events
allSub := bus.SubscribeAll(func(e event.Event) {
    fmt.Printf("Event: %s\n", e.Type)
})

// Unsubscribe
bus.Unsubscribe(sub)
```

## Event Types

| Event | Emitted When |
|-------|-------------|
| `block.stored` | Blocks are stored or updated |
| `block.deleted` | A block is deleted |
| `project.created` | A project is created |
| `project.updated` | A project is updated |
| `project.deleted` | A project is deleted |
| `version.created` | A version snapshot is created |
| `connector.pulled` | Content is pulled from a connector |
| `connector.pushed` | Content is pushed to a connector |
| `flow.started` | A flow begins execution |
| `flow.completed` | A flow completes successfully |
| `flow.failed` | A flow fails |
| `quality.passed` | Quality gate passes |
| `quality.failed` | Quality gate fails |
| `quality.warning` | Quality gate issues advisory warning |

## EventEmittingStore

The `EventEmittingStore` decorator wraps a `ContentStore` and emits events on all mutations:

```go
store := store.NewSQLiteStore("project.db")
bus := event.NewChannelEventBus()
emittingStore := event.NewEventEmittingStore(store, bus)
```

## Automation Rules

The automation engine evaluates rules triggered by events:

```go
engine := event.NewAutomationEngine(bus)
engine.AddRule(event.AutomationRule{
    Name:      "auto-translate-on-pull",
    EventType: event.EventConnectorPulled,
    Conditions: []event.Condition{
        {Field: "project_id", Operator: "equals", Value: "proj-1"},
    },
    Action: func(e event.Event) {
        // Trigger translation flow
    },
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
        Name:      "min-translation-coverage",
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

Webhook delivery with HMAC-SHA256 signing and retry:

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
