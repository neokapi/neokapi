---
id: 012-distributed-event-bus
sidebar_position: 12
title: "AD-012: Distributed Event Bus"
---

# AD-012: Distributed Event Bus

## Summary

Bowrain-server and bowrain-worker replicas coordinate through a shared
event broker. The `EventBus` interface has one production backend —
**Redis Streams** (ElastiCache on AWS; a stock `redis` container for
local and self-hosted stacks) — opted into with
`BOWRAIN_EVENT_BACKEND=redis`. Every `SubscribeGroup` component becomes a
Redis Streams consumer group, so any replica can consume events with no
leader election; `Subscribe`/`SubscribeAll` are fan-out (every replica
sees every event), which is what the SSE/gRPC relays need.

Redis carries the bus because it is already a platform dependency
(sessions, the sync-hash cache, agent pub/sub) and is managed on AWS as
ElastiCache — the event bus needs no separate broker, so the compute node
holds no queue state. (Historical backends on Azure Service Bus and NATS
JetStream were removed with the managed-services redesign and the
follow-up backend cut; see [History](#history-removed-backends).)

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

Two implementations:

| Implementation    | Backend                  | Purpose                                        |
| ----------------- | ------------------------ | ---------------------------------------------- |
| `ChannelEventBus` | Go channels (in-process) | Unit tests and single-process development      |
| `RedisEventBus`   | Redis Streams            | Production (ElastiCache), local + self-hosted  |

`RedisEventBus` publishes with a single `XADD` (capped by `MAXLEN` so the
stream stays bounded). `SubscribeGroup` reads via `XREADGROUP` on a Redis
Streams consumer group — competing consumers, position preserved across
restarts. `Subscribe`/`SubscribeAll` read the stream tail with `XREAD`
independently, so every such subscriber sees every event (fan-out). The
starting position is resolved synchronously at subscribe time, so an event
published immediately afterward is not lost to a `$`-resolution race.

### Runtime selection

The server picks a backend from configuration:

```go
if os.Getenv("BOWRAIN_EVENT_BACKEND") == "redis" && cfg.RedisURL != "" {
    bus = event.NewRedisEventBus(cfg.RedisURL, cfg.RedisPassword)
} else {
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
registered schema; subscribers filter on the `Type` field.

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
| `terminology.changed`           | Terms store mutations                                      |
| `push.completed`                | Sync push commit                                           |
| `push.automations.completed`    | PushCompletionTracker (AD-014)                             |
| `project.updated`               | Project settings / locale additions                        |
| `run.started` / `run.completed` | AutomationRunManager (AD-013)                              |
| `agent.*`                       | Bravo agent (AD-016)                                       |

### Consumer groups

One Redis Streams consumer group per subscriber component:

```
Stream: bowrain:events
  ├── Group: automations         → AutomationEngine
  ├── Group: activity-recorder   → ActivityRecorder
  ├── Group: notifications       → NotificationDispatcher
  ├── Group: push-tracker        → PushCompletionTracker
  ├── Group: progress-tracker    → ProgressTracker
  ├── Group: audit-logger        → AuditLogger
  ├── Group: siem-exporter       → SIEMExporter
  ├── Group: graph-syncer        → GraphSyncer
  ├── Group: convergence-onpush  → on-push convergence trigger (push / new locale → run)
  └── Group: forge-delivery      → forge delivery tier (run completed → pull request)
```

Consumer groups are for **state-advancing** subscribers: a missed event
would silently strand work (a push that never converges, a converged run
that never reaches the forge), so the group's persisted position must
survive replica restarts and deploy rollovers. Pure freshness
subscribers — the SSE/gRPC change relays and the platform-config cache
refresh — stay on fan-out `Subscribe`/`SubscribeAll`: every instance must
react, and a missed event is healed by the next read or reconnect.

### No leader election

Any bowrain-server or bowrain-worker replica can consume from any
subscription. The broker, not the application, owns delivery semantics.
This eliminates custom coordination — there is no `LeaderElector`, no
lease table, no IsLeader gating, no polling to discover work.

### Test backend

`ChannelEventBus` remains available for unit tests that exercise the
event flow without external infrastructure. Integration tests against
real behavior use Redis via Docker Compose.

### History: removed backends

Two further backends existed and were removed. `ServiceBusEventBus`
(Azure Service Bus, a topic with one subscription per consumer) went with
the Azure-to-AWS managed-services redesign. `NATSEventBus` (NATS
JetStream, one durable consumer per component on an `EVENTS.>` stream)
served self-hosted stacks until the backend cut consolidated self-hosting
onto the same SQS-compatible queue + Redis Streams pair the cloud uses —
one broker story everywhere, one fewer container to operate. Both
implementations live in git history behind the unchanged `EventBus`
interface.

## Consequences

- Horizontal scaling is a deployment concern, not an application
  concern. Adding replicas does not require code changes.
- Events published on any replica reach exactly one consumer per
  group.
- Failover is automatic: unacked messages redeliver when a replica
  crashes or is drained.
- Zero-delay event flow — no polling intervals, no 5-second sleep in
  trackers.
- Local development stays fast because Redis is already part of the
  Docker Compose topology; no extra service is required.
- Tests remain deterministic with the in-process bus.

## Related

- [AD-013: Automation Engine](013-automation-engine.md) — primary event consumer
- [AD-014: Translator Workflow](014-translator-workflow.md) — activities, tasks, notifications
- [AD-015: Server-Side AI Operations](015-server-ai-operations.md) — translation job queue sibling
- [AD-framework-004: Processing Engine](https://neokapi.github.io/contribute/architecture/004-processing-engine) — in-process pipeline model
