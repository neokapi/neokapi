package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
	"github.com/redis/go-redis/v9"
)

// RedisEventBus implements EventBus on a single Redis stream. It is the AWS
// production backend (Redis = ElastiCache) and reuses the Redis dependency the
// server already runs for sessions, the sync-hash cache, and agent pub/sub — so
// no separate broker is needed.
//
// Two delivery modes map onto the two subscription shapes:
//
//   - SubscribeGroup → a Redis Streams consumer group (XREADGROUP): competing
//     consumers, so exactly one instance in the group handles each event, and
//     the group's position survives restarts. This is what the durable
//     domain-event consumers (audit, notifications, automations,
//     convergence-onpush, forge-delivery, …) need. A companion XAUTOCLAIM
//     sweep per group consumer reclaims entries stranded in a crashed
//     consumer's PEL (delivered but never acknowledged), so no event is lost
//     to a process dying mid-handler.
//   - Subscribe / SubscribeAll → an independent tail read (XREAD from the current
//     stream head): every such subscriber sees every event — correct fan-out for
//     the SSE/gRPC relays, even across instances.
//
// Publish is a single XADD (capped with MAXLEN so the stream stays bounded);
// both modes read the same stream.
type RedisEventBus struct {
	client     *redis.Client
	stream     string
	consumerID string

	// PEL reclaim tuning; production values by default, shortened in tests.
	reclaimInterval time.Duration
	reclaimMinIdle  time.Duration

	mu     sync.Mutex
	subs   map[string]*redisEventSub
	closed bool
}

type redisEventSub struct {
	cancel context.CancelFunc
	done   chan struct{}
}

const (
	defaultEventStream = "bowrain:events"
	eventStreamMaxLen  = 10000
	eventReadBlock     = 5 * time.Second

	// PEL reclaim: how often each group consumer sweeps for stranded pending
	// entries, and how long an entry must sit unacknowledged before it is
	// considered stranded. The min-idle threshold sits comfortably beyond any
	// synchronous handler duration, so a live consumer mid-dispatch is never
	// raced; anything older belongs to a consumer that died before acking.
	defaultReclaimInterval = 30 * time.Second
	defaultReclaimMinIdle  = time.Minute

	// reclaimBatch bounds one XAUTOCLAIM call; reclaimSweepMax bounds one
	// sweep (the next tick resumes — the PEL is capped by the stream MAXLEN
	// anyway, so a sweep can never be unbounded for long).
	reclaimBatch    = 64
	reclaimSweepMax = 1024
)

// NewRedisEventBus connects to Redis and verifies the connection.
func NewRedisEventBus(redisURL, password string) (*RedisEventBus, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis event bus: parse url: %w", err)
	}
	if password != "" {
		opts.Password = password
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis event bus: ping: %w", err)
	}
	return &RedisEventBus{
		client:          client,
		stream:          defaultEventStream,
		consumerID:      id.New(),
		reclaimInterval: defaultReclaimInterval,
		reclaimMinIdle:  defaultReclaimMinIdle,
		subs:            make(map[string]*redisEventSub),
	}, nil
}

// Publish appends the event to the stream. The MAXLEN cap keeps Redis bounded;
// events are transient signals, not a durable log.
func (b *RedisEventBus) Publish(ev platev.Event) {
	if ev.ID == "" {
		ev.ID = id.New()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	data, err := json.Marshal(ev)
	if err != nil {
		slog.Warn("redis-event-bus: marshal error", "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.client.XAdd(ctx, &redis.XAddArgs{
		Stream: b.stream,
		MaxLen: eventStreamMaxLen,
		Approx: true,
		Values: map[string]any{"data": data, "type": string(ev.Type)},
	}).Err(); err != nil {
		slog.Warn("redis-event-bus: publish error", "error", err)
	}
}

// Subscribe delivers only events of the given type (fan-out).
func (b *RedisEventBus) Subscribe(eventType platev.EventType, handler platev.EventHandler) *platev.Subscription {
	return b.addFanout(eventType, handler)
}

// SubscribeAll delivers every event (fan-out).
func (b *RedisEventBus) SubscribeAll(handler platev.EventHandler) *platev.Subscription {
	return b.addFanout("", handler)
}

// addFanout resolves the current stream head synchronously before starting the
// reader, so an event published the instant after this returns is not missed by
// the "$" race.
func (b *RedisEventBus) addFanout(eventType platev.EventType, handler platev.EventHandler) *platev.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	sub := &platev.Subscription{ID: id.New(), EventType: eventType, Handler: handler}

	lastID := "0"
	if info, err := b.client.XInfoStream(context.Background(), b.stream).Result(); err == nil {
		lastID = info.LastGeneratedID
	}

	ctx, cancel := context.WithCancel(context.Background())
	rs := &redisEventSub{cancel: cancel, done: make(chan struct{})}
	b.subs[sub.ID] = rs
	go b.runFanout(ctx, rs, lastID, eventType, handler)
	return sub
}

func (b *RedisEventBus) runFanout(ctx context.Context, rs *redisEventSub, lastID string, eventType platev.EventType, handler platev.EventHandler) {
	defer close(rs.done)
	for {
		if ctx.Err() != nil {
			return
		}
		res, err := b.client.XRead(ctx, &redis.XReadArgs{
			Streams: []string{b.stream, lastID},
			Count:   64,
			Block:   eventReadBlock,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue // block timed out with nothing new
			}
			if ctx.Err() != nil {
				return
			}
			slog.Warn("redis-event-bus: fanout read error", "error", err)
			sleepCtx(ctx, time.Second)
			continue
		}
		for _, st := range res {
			for _, msg := range st.Messages {
				lastID = msg.ID
				ev, ok := decodeEvent(msg)
				if !ok {
					continue
				}
				if eventType != "" && ev.Type != eventType {
					continue
				}
				dispatchEvent(handler, ev)
			}
		}
	}
}

// SubscribeGroup joins a Redis Streams consumer group: one member of the group
// handles each event (competing consumers), and the group resumes from its last
// acknowledged position after a restart.
func (b *RedisEventBus) SubscribeGroup(group string, handler platev.EventHandler) *platev.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	sub := &platev.Subscription{ID: id.New(), Group: group, Handler: handler}

	// Create the group at the current head (idempotent). BUSYGROUP means it
	// already exists — keep its persisted position rather than resetting it.
	createCtx, cancelCreate := context.WithTimeout(context.Background(), 5*time.Second)
	err := b.client.XGroupCreateMkStream(createCtx, b.stream, group, "$").Err()
	cancelCreate()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		slog.Warn("redis-event-bus: create group error", "group", group, "error", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rs := &redisEventSub{cancel: cancel, done: make(chan struct{})}
	b.subs[sub.ID] = rs
	go b.runGroup(ctx, rs, group, handler)
	return sub
}

func (b *RedisEventBus) runGroup(ctx context.Context, rs *redisEventSub, group string, handler platev.EventHandler) {
	defer close(rs.done)

	// Companion PEL-reclaim sweep: XREADGROUP with ">" only ever sees
	// never-delivered entries, so an event another consumer read but died
	// before acking would otherwise sit in that dead consumer's PEL forever.
	// The sweep shares this subscription's lifetime; done closes only after
	// both goroutines return, keeping shutdown (and goleak) clean.
	reclaimDone := make(chan struct{})
	go func() {
		defer close(reclaimDone)
		b.runGroupReclaim(ctx, group, handler)
	}()
	defer func() { <-reclaimDone }()

	for {
		if ctx.Err() != nil {
			return
		}
		res, err := b.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: b.consumerID,
			Streams:  []string{b.stream, ">"},
			Count:    64,
			Block:    eventReadBlock,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			slog.Warn("redis-event-bus: group read error", "group", group, "error", err)
			sleepCtx(ctx, time.Second)
			continue
		}
		for _, st := range res {
			for _, msg := range st.Messages {
				b.handleGroupMessage(ctx, group, handler, msg)
			}
		}
	}
}

// handleGroupMessage decodes, dispatches, and acknowledges one group-delivered
// entry — the shared path for fresh XREADGROUP reads and XAUTOCLAIM reclaims.
func (b *RedisEventBus) handleGroupMessage(ctx context.Context, group string, handler platev.EventHandler, msg redis.XMessage) {
	if ev, ok := decodeEvent(msg); ok {
		dispatchEvent(handler, ev)
	}
	// Ack unconditionally: a malformed entry must not be redelivered
	// forever. Detach from ctx so a shutdown mid-batch still acks.
	b.client.XAck(context.WithoutCancel(ctx), b.stream, group, msg.ID)
}

// runGroupReclaim periodically sweeps the group's pending-entries list for
// entries stranded by a crashed consumer and redispatches them here.
func (b *RedisEventBus) runGroupReclaim(ctx context.Context, group string, handler platev.EventHandler) {
	ticker := time.NewTicker(b.reclaimInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		b.reclaimStranded(ctx, group, handler)
		b.pruneDeadConsumers(ctx, group)
	}
}

// reclaimStranded runs XAUTOCLAIM over the group's PEL, claiming entries idle
// beyond the min-idle threshold to this consumer and dispatching them through
// the normal handler + ack path.
//
// Idempotency: group consumers are replay-safe by design (the same argument
// that makes the durable consumer groups safe across deploy rollovers — #28,
// #1364): audit appends are keyed, notifications/automations/convergence
// triggers re-derive state. A reclaimed entry whose original consumer did
// finish the handler but died before XACK is therefore at worst an extra
// idempotent pass, never corruption.
func (b *RedisEventBus) reclaimStranded(ctx context.Context, group string, handler platev.EventHandler) {
	start := "0-0"
	claimed := 0
	for claimed < reclaimSweepMax {
		msgs, next, err := b.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   b.stream,
			Group:    group,
			Consumer: b.consumerID,
			MinIdle:  b.reclaimMinIdle,
			Start:    start,
			Count:    reclaimBatch,
		}).Result()
		if err != nil {
			if ctx.Err() == nil {
				slog.Warn("redis-event-bus: reclaim error", "group", group, "error", err)
			}
			break
		}
		for _, msg := range msgs {
			b.handleGroupMessage(ctx, group, handler, msg)
		}
		claimed += len(msgs)
		// XAUTOCLAIM is an iterative scan: the returned cursor resumes where
		// this call stopped, and "0-0" means the whole PEL has been covered.
		if next == "" || next == "0-0" {
			break
		}
		start = next
	}
	if claimed > 0 {
		// Stranded entries mean a consumer died between delivery and ack —
		// worth a WARN, not routine operation.
		slog.Warn("redis-event-bus: reclaimed stranded pending entries from a dead consumer",
			"group", group, "count", claimed)
	}
}

// pruneDeadConsumers deletes group consumers that have nothing pending and
// have been idle past the reclaim threshold: leftover per-process consumer IDs
// from dead instances that would otherwise accumulate forever. Deleting a
// zero-pending consumer is loss-free — even in the pathological case of a live
// but long-stalled consumer, XREADGROUP transparently recreates it on its next
// read.
func (b *RedisEventBus) pruneDeadConsumers(ctx context.Context, group string) {
	consumers, err := b.client.XInfoConsumers(ctx, b.stream, group).Result()
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("redis-event-bus: list consumers error", "group", group, "error", err)
		}
		return
	}
	for _, c := range consumers {
		if c.Name == b.consumerID || c.Pending != 0 || c.Idle < b.reclaimMinIdle {
			continue
		}
		if err := b.client.XGroupDelConsumer(ctx, b.stream, group, c.Name).Err(); err != nil && ctx.Err() == nil {
			slog.Warn("redis-event-bus: delete dead consumer error", "group", group, "consumer", c.Name, "error", err)
		}
	}
}

// Unsubscribe stops one subscription's reader.
func (b *RedisEventBus) Unsubscribe(sub *platev.Subscription) {
	if sub == nil {
		return
	}
	b.mu.Lock()
	rs, ok := b.subs[sub.ID]
	if ok {
		delete(b.subs, sub.ID)
	}
	b.mu.Unlock()
	if ok {
		rs.cancel()
		<-rs.done
	}
}

// Close stops all readers and releases the connection.
func (b *RedisEventBus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	subs := make([]*redisEventSub, 0, len(b.subs))
	for _, rs := range b.subs {
		subs = append(subs, rs)
	}
	b.subs = make(map[string]*redisEventSub)
	b.mu.Unlock()

	for _, rs := range subs {
		rs.cancel()
		<-rs.done
	}
	_ = b.client.Close()
}

func decodeEvent(msg redis.XMessage) (platev.Event, bool) {
	raw, ok := msg.Values["data"].(string)
	if !ok {
		return platev.Event{}, false
	}
	var ev platev.Event
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		slog.Warn("redis-event-bus: decode error", "id", msg.ID, "error", err)
		return platev.Event{}, false
	}
	return ev, true
}

func dispatchEvent(handler platev.EventHandler, ev platev.Event) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("redis-event-bus: recovered panic in event handler", "event_type", ev.Type, "panic", r)
		}
	}()
	handler(ev)
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

var _ platev.EventBus = (*RedisEventBus)(nil)
