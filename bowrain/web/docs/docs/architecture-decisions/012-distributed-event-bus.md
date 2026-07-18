---
id: 012-distributed-event-bus
sidebar_position: 12
title: "AD-012: Distributed Event Bus"
---

# AD-012: Distributed Event Bus

## Summary

Bowrain-server and bowrain-worker replicas coordinate through a shared
event broker. The `EventBus` interface has three production backends —
**Redis Streams** (the AWS default, on ElastiCache), Azure Service Bus,
and NATS JetStream (self-hosted) — selected at runtime from
configuration. Every `SubscribeGroup` component becomes a consumer group
(a Redis Streams group, an ASB subscription, or a JetStream durable
consumer), so any replica can consume events with no leader election;
`Subscribe`/`SubscribeAll` are fan-out (every replica sees every event),
which is what the SSE/gRPC relays need.

Redis is the default on AWS because it is already a platform dependency
(sessions, the sync-hash cache, agent pub/sub) and is managed there as
ElastiCache — so the event bus needs no separate broker, and removing the
standalone NATS/JetStream container is what lets the compute node hold no
queue state.

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

Four implementations:

| Implementation        | Backend                   | Purpose                          |
| --------------------- | ------------------------- | -------------------------------- |
| `ChannelEventBus`     | Go channels (in-process)  | Unit tests only                  |
| `RedisEventBus`       | Redis Streams             | AWS production (ElastiCache), local dev |
| `NATSEventBus`        | NATS JetStream            | Self-hosted                      |
| `ServiceBusEventBus`  | Azure Service Bus         | Legacy Azure deployment          |

`RedisEventBus` publishes with a single `XADD` (capped by `MAXLEN` so the
stream stays bounded). `SubscribeGroup` reads via `XREADGROUP` on a Redis
Streams consumer group — competing consumers, position preserved across
restarts. `Subscribe`/`SubscribeAll` read the stream tail with `XREAD`
independently, so every such subscriber sees every event (fan-out). The
starting position is resolved synchronously at subscribe time, so an event
published immediately afterward is not lost to a `$`-resolution race.

### Runtime selection

The server picks a backend from configuration, Redis first when opted in:

```go
switch {
case os.Getenv("BOWRAIN_EVENT_BACKEND") == "redis" && cfg.RedisURL != "":
    bus = event.NewRedisEventBus(cfg.RedisURL, cfg.RedisPassword)
case cfg.ServiceBusConnection != "":
    bus = event.NewServiceBusEventBus(cfg.ServiceBusConnection)
case cfg.NATSURL != "":
    bus = event.NewNATSEventBus(cfg.NATSURL)
default:
    bus = event.NewChannelEventBus()
}
```

A test binary uses the channel bus. Docker Compose and the AWS deployment
set `BOWRAIN_EVENT_BACKEND=redis` and point `BOWRAIN_REDIS_URL` at the
shared Redis/ElastiCache — the same instance the session store and
sync-hash cache already use. The worker selects the backend the same way,
so server and worker share one broker.

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
| `flow.completed` / `flow.failed`| Flow executor (type defined; not yet emitted)              |
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
- [AD-framework-004: Processing Engine](https://neokapi.github.io/web/neokapi/contribute/architecture/004-processing-engine) — in-process pipeline model
