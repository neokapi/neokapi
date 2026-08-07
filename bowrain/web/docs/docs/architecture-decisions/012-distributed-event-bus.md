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
holds no queue state. Self-hosted stacks run the same pair the cloud does: one
broker story everywhere, one fewer container to operate.

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

### The group handler's verdict

A group handler is `func(Event) error`, and the acknowledgement follows its
return. Nil acknowledges; an error leaves the entry pending, where the
companion `XAUTOCLAIM` sweep finds it once the consumer has been idle past
the threshold. That is the only reason a group differs from a tail read:
without it, a handler that could not do its work had its event acknowledged
anyway, and the event was gone.

Two rules follow, and each consumer states which side of them it is on:

- **Return an error only when the side effect must happen and did not.** A
  handler that reports failure for work it merely chose not to do turns its
  group into a redelivery loop. The audit logger, the activity recorder, the
  notification dispatcher, the graph syncer and the review re-check report
  theirs. The convergence, forge-delivery, review-completion, automation,
  failure-summons, push-tracking and progress consumers acknowledge: their
  work is re-derived on the next event, collapsed by a run guard, or is a
  notice nobody should receive twice.
- **Every side effect behind an error return needs an idempotency key.**
  Redelivery is not hypothetical — the reclaim sweep re-runs handlers whose
  original consumer died *after* finishing — so the durable writes are keyed
  on the event that produced them: `audit_log.event_id`,
  `activities.event_id`, and `notifications (user_id, source_event_id)`,
  each behind a unique index and an `ON CONFLICT DO NOTHING`.

Two exceptions are deliberate. An entry that does not decode is acknowledged,
because it never will and would otherwise arrive forever. And a handler still
running is not redelivered to itself: the sweep claims to the same consumer id
as the read loop, so a handler slower than the min-idle threshold is
indistinguishable from a dead one — an in-flight set makes the reclaim a no-op
rather than a second concurrent dispatch.

`ChannelEventBus` has no pending list, so a handler error there is logged and
nothing more, and a subscriber that falls behind its buffer loses events
outright (counted as `bowrain_events_dropped_total`). That is the durability
difference between the in-process default and Redis, and the reason a
deployment that must not lose events runs Redis.

### No leader election

Any bowrain-server or bowrain-worker replica can consume from any
subscription. The broker, not the application, owns delivery semantics.
This eliminates custom coordination — there is no `LeaderElector`, no
lease table, no IsLeader gating, no polling to discover work.

### Test backend

`ChannelEventBus` remains available for unit tests that exercise the
event flow without external infrastructure. Integration tests against
real behavior use Redis via Docker Compose.

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
