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
//     convergence-onpush, forge-delivery, …) need.
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
		client:     client,
		stream:     defaultEventStream,
		consumerID: id.New(),
		subs:       make(map[string]*redisEventSub),
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
				if ev, ok := decodeEvent(msg); ok {
					dispatchEvent(handler, ev)
				}
				// Ack unconditionally: a malformed entry must not be redelivered
				// forever. Detach from ctx so a shutdown mid-batch still acks.
				b.client.XAck(context.WithoutCancel(ctx), b.stream, group, msg.ID)
			}
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
