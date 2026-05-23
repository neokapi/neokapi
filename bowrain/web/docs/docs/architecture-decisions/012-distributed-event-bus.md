---
id: 012-distributed-event-bus
sidebar_position: 12
title: "AD-012: Distributed Event Bus"
---

# AD-012: Distributed Event Bus

## Summary

Bowrain-server and bowrain-worker replicas coordinate through a shared
event broker. The `EventBus` interface has two production backends —
Azure Service Bus (managed) and NATS JetStream (self-hosted and local
development) — selected at runtime from configuration. Every subscriber
component becomes a consumer group (an ASB subscription or a JetStream
durable consumer), so any replica can consume events with no leader
election.

## Context

Bowrain's automation engine ([AD-013](013-automation-engine.md)),
activity recorder, notification dispatcher, push-completion tracker,
and audit logger all react to platform events. Running
multiple server replicas — behind a load balancer, across zones, or in
a worker pool — requires a broker that delivers each event to exactly
one member of each consumer group and retains messages across replica
restarts. An in-process channel bus cannot meet either requirement.

## Decision

### EventBus interface

```go
type EventBus interface {
    Publish(ctx context.Context, event Event) error
    Subscribe(ctx context.Context, eventType EventType, h EventHandler) (Subscription, error)
    SubscribeAll(ctx context.Context, h EventHandler) (Subscription, error)
    Unsubscribe(sub Subscription) error
}

type EventHandler func(ctx context.Context, event Event) error
```

Three implementations:

| Implementation        | Backend                   | Purpose                    |
| --------------------- | ------------------------- | -------------------------- |
| `ChannelEventBus`     | Go channels (in-process)  | Unit tests only            |
| `NATSEventBus`        | NATS JetStream            | Local dev, self-hosted     |
| `ServiceBusEventBus`  | Azure Service Bus         | Managed production         |

### Runtime selection

The server picks a backend from configuration, in order:

```go
switch {
case cfg.ServiceBusConnection != "":
    bus = event.NewServiceBusEventBus(cfg.ServiceBusConnection)
case cfg.NATSUrl != "":
    bus = event.NewNATSEventBus(cfg.NATSUrl)
default:
    bus = event.NewChannelEventBus()
}
```

A test binary uses the channel bus. Docker Compose sets `NATS_URL` so
local development hits JetStream. Production sets
`SERVICE_BUS_CONNECTION` and routes through the managed namespace.

### Event model

```go
type Event struct {
    ID          string
    Type        EventType
    Source      string             // connector ID, tool name, or "user"
    ProjectID   string
    CausationID string             // ID of the event that caused this one (loop prevention)
    Payload     any
    Timestamp   time.Time
}
```

Events serialize as JSON. Every event has a type drawn from a
registered schema. The `Type` field maps to the Service Bus message
`Subject` or the NATS subject suffix (`EVENTS.<type>`), so subscriptions
can filter at the broker.

Registered event types include:

| Event type                      | Emitter                                                    |
| ------------------------------- | ---------------------------------------------------------- |
| `content.changed`               | EventEmittingStore on mutations                            |
| `content.extracted`             | Connector pull                                             |
| `translation.updated`           | Block target updates                                       |
| `translation.reviewed`          | Review decisions                                           |
| `connector.synced`              | Connector completion                                       |
| `flow.completed` / `flow.failed`| Flow executor                                              |
| `quality.gate.failed`           | Quality gate evaluation                                    |
| `terminology.changed`           | Termbase mutations                                         |
| `push.completed`                | Sync push commit                                           |
| `push.automations.completed`    | PushCompletionTracker (AD-014)                             |
| `project.updated`               | Project settings / locale additions                        |
| `run.started` / `run.completed` | AutomationRunManager (AD-013)                              |
| `agent.*`                       | Bravo agent (AD-016)                                       |

### Azure Service Bus

A single topic (`bowrain-events`) fans out to one subscription per
consumer component. Each subscription is a competing-consumer group;
Service Bus guarantees exactly-once delivery within a subscription and
redelivers unacked messages on consumer failure.

```
Topic: bowrain-events
  ├── Subscription: automations        → AutomationEngine
  ├── Subscription: activity-recorder  → ActivityRecorder
  ├── Subscription: notifications      → NotificationDispatcher
  ├── Subscription: push-tracker       → PushCompletionTracker
  ├── Subscription: progress-tracker   → ProgressTracker
  ├── Subscription: audit-logger       → AuditLogger
  └── Subscription: graph-syncer       → GraphSyncer
```

Subscription settings: max delivery 5, lock duration 30s, dead-letter
after 7 days. `Publish` sets the message `Subject` to the event type so
subscriptions with filter rules receive only the relevant types.

### NATS JetStream

The self-hosted backend uses a single stream (`EVENTS`) with subjects
`EVENTS.>` and one durable consumer per subscriber component. Consumers
use WorkQueue retention (messages delete after ack).

```
Stream: EVENTS (subjects: EVENTS.>)
  ├── Consumer: automations
  ├── Consumer: activity-recorder
  ├── Consumer: notifications
  ├── Consumer: push-tracker
  ├── Consumer: progress-tracker
  ├── Consumer: audit-logger
  └── Consumer: graph-syncer
```

`Publish` writes to `EVENTS.<event_type>`. `Subscribe` creates a
durable consumer with the matching subject filter; `SubscribeAll`
filters on `EVENTS.>`.

### No leader election

Any bowrain-server or bowrain-worker replica can consume from any
subscription. The broker, not the application, owns delivery semantics.
This eliminates custom coordination — there is no `LeaderElector`, no
lease table, no IsLeader gating, no polling to discover work.

### Test backend

`ChannelEventBus` remains available for unit tests that exercise the
event flow without external infrastructure. Integration tests against
real behavior use NATS via Docker Compose.

## Consequences

- Horizontal scaling is a deployment concern, not an application
  concern. Adding replicas does not require code changes.
- Events published on any replica reach exactly one consumer per
  group.
- Failover is automatic: unacked messages redeliver when a replica
  crashes or is drained.
- Zero-delay event flow — no polling intervals, no 5-second sleep in
  trackers.
- Local development stays fast because NATS is already part of the
  Docker Compose topology; no extra service is required.
- Tests remain deterministic with the in-process bus.

## Related

- [AD-013: Automation Engine](013-automation-engine.md) — primary event consumer
- [AD-014: Translator Workflow](014-translator-workflow.md) — activities, tasks, notifications
- [AD-015: Server-Side AI Operations](015-server-ai-operations.md) — translation job queue sibling
- [AD-framework-004: Processing Engine](https://neokapi.github.io/web/neokapi/docs/architecture/004-processing-engine) — in-process pipeline model
