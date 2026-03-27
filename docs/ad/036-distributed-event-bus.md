---
id: 036-distributed-event-bus
sidebar_position: 36
title: "AD-036: Distributed Event Bus"
---
# AD-036: Distributed Event Bus

## Context

The automation system ([AD-011](./011-automation.md)) uses an in-process `ChannelEventBus` where all subscribers live in one process. This prevents horizontal scaling: events published on one server instance are invisible to others. The leader election pattern ([#169](https://github.com/neokapi/neokapi/issues/169)) was a workaround — the correct solution is a shared message broker.

**Production infrastructure already includes Azure Service Bus** (Standard tier, deployed via Bicep). Local development uses NATS with JetStream in Docker Compose. Both support the pub/sub pattern with consumer groups needed for exactly-once event delivery.

## Decision

### Replace ChannelEventBus with broker-backed implementations

The `EventBus` interface stays unchanged. Two new implementations replace the in-memory bus:

```
EventBus interface (unchanged)
  ├── ChannelEventBus    (test only — single process)
  ├── NATSEventBus       (local dev + self-hosted)
  └── ServiceBusEventBus (Azure production)
```

### Azure Service Bus: Topics + Subscriptions

A Service Bus **topic** replaces the event bus. Each subscriber component becomes a **subscription** (consumer group) on the topic. Service Bus guarantees exactly-once delivery per subscription.

```
Topic: bowrain-events
  ├── Subscription: automations        → AutomationEngine
  ├── Subscription: activity-recorder  → ActivityRecorder
  ├── Subscription: notifications      → NotificationDispatcher
  ├── Subscription: push-tracker       → PushCompletionTracker
  ├── Subscription: progress-tracker   → ProgressTracker
  ├── Subscription: audit-logger       → AuditLogger
  ├── Subscription: graph-syncer       → GraphSyncer
  └── Subscription: queue-sink         → QueueSink
```

Each server instance creates receivers for all subscriptions. Service Bus distributes messages across competing consumers within each subscription — no leader election needed.

### NATS JetStream: Stream + Consumers

For local dev and self-hosted deployments, a JetStream **stream** with **durable consumers** per component:

```
Stream: EVENTS (subjects: EVENTS.>)
  ├── Consumer: automations        (deliver group)
  ├── Consumer: activity-recorder  (deliver group)
  ├── Consumer: notifications      (deliver group)
  ├── Consumer: push-tracker       (deliver group)
  ├── Consumer: progress-tracker   (deliver group)
  ├── Consumer: audit-logger       (deliver group)
  ├── Consumer: graph-syncer       (deliver group)
  └── Consumer: queue-sink         (deliver group)
```

### Event serialization

Events are serialized as JSON. The existing `Event` struct with `map[string]string` data is compact enough. The `Type` field maps to the Service Bus message `Subject` / NATS subject suffix for filtering.

### What gets removed

- **Leader election** (`LeaderElector`, `leader_leases` table) — no longer needed
- **IsLeader gating** on AutomationEngine, trackers — no longer needed
- **pending_pushes table** and DB polling — no longer needed; events flow through the broker
- **ChannelEventBus** remains for unit tests only

### Backend selection

Same pattern as job queues:

```go
var bus platev.EventBus
switch {
case cfg.ServiceBusConnection != "":
    bus = event.NewServiceBusEventBus(cfg.ServiceBusConnection)
case cfg.NATSUrl != "":
    bus = event.NewNATSEventBus(cfg.NATSUrl)
default:
    bus = event.NewChannelEventBus() // single-instance fallback
}
```

### Infrastructure changes

**Azure (bowrain-infra):**
- Add `bowrain-events` topic to the existing Service Bus namespace
- Add 8 subscriptions (one per consumer component)
- Topic auto-forwarded to dead-letter after 7 days
- Subscriptions: max delivery 5, lock duration 30s

**Local dev (compose.yaml):**
- NATS already running with JetStream enabled — no change needed

## Implementation

### ServiceBusEventBus

Uses the same Azure SDK as the existing queue (`azservicebus`):

```go
type ServiceBusEventBus struct {
    client *azservicebus.Client
    sender *azservicebus.Sender // publishes to topic
    subs   map[string]*busSubscriber // subscription name → receiver + handler
}
```

- `Publish`: Serializes `Event` to JSON, sets `Subject` to event type, sends to topic
- `Subscribe`: Creates a receiver for the named subscription, runs handler goroutine
- `SubscribeAll`: Same as Subscribe but with subscription that has no filter rule
- Events include correlation ID for tracing

### NATSEventBus

Uses the existing NATS JetStream SDK:

```go
type NATSEventBus struct {
    js        jetstream.JetStream
    nc        *nats.Conn
    consumers map[string]jetstream.Consumer
}
```

- `Publish`: Publishes to `EVENTS.<event_type>` subject
- `Subscribe`: Creates durable consumer with subject filter `EVENTS.<type>`
- `SubscribeAll`: Consumer with subject filter `EVENTS.>`
- WorkQueue retention (delete after ack)

### Migration from leader election

1. Deploy new event bus implementations
2. Wire `ServiceBusEventBus` / `NATSEventBus` in server init based on config
3. Remove `IsLeader` checks from AutomationEngine, trackers
4. Remove `LeaderElector`, `leader_leases` table, `pending_pushes` table
5. PushCompletionTracker reverts to event-based discovery (no DB polling)

### Files to create

| File | Purpose |
|---|---|
| `platform/event/bus_servicebus.go` | Azure Service Bus EventBus implementation |
| `platform/event/bus_nats.go` | NATS JetStream EventBus implementation |
| `bowrain-infra/modules/servicebus-events.bicep` | Topic + subscriptions Bicep module |

### Files to modify

| File | Change |
|---|---|
| `platform/server/server.go` | Select bus backend based on config, remove leader election |
| `platform/server/config.go` | Already has ServiceBusConnection + NATSUrl |
| `platform/event/automation.go` | Remove IsLeader field |
| `platform/event/push_completion_tracker.go` | Remove IsLeader + DB polling, revert to event-only |
| `platform/event/step_completion_tracker.go` | Remove IsLeader |
| `platform/compose.yaml` | No change (NATS already present) |
| `bowrain-infra/modules/servicebus.bicep` | Import events module |

## Alternatives Considered

- **Keep leader election**: Works but is a custom solution for a standard problem. Adds complexity (lease TTL tuning, failover delay, DB polling) that a message broker eliminates.

- **Redis Streams**: Already deployed but no persistence guarantees for consumer groups. Service Bus is more reliable for exactly-once delivery.

- **PostgreSQL LISTEN/NOTIFY**: No persistence, no consumer groups, limited throughput. Not suitable for production event delivery.

- **Self-host NATS in Azure**: Additional infrastructure to manage when Service Bus already exists and is managed.

## Consequences

- Horizontal scaling works correctly without custom coordination
- Events published on any instance are delivered to exactly one consumer per group
- Zero-delay event delivery (no 5-second polling)
- Automatic failover when instances crash (broker redelivers unacked messages)
- Leader election code can be removed — simpler system
- Local dev continues to work with NATS (already in Docker Compose)
- Unit tests continue to use ChannelEventBus (no infrastructure needed)
