package event

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startRedis starts a throwaway Redis and returns its URL. Each test gets its
// own container so streams and consumer groups stay isolated. Exercises real
// Redis Streams (XADD/XREAD/XREADGROUP) — the same code path as ElastiCache in
// production, per the no-mocks rule.
func startRedis(t *testing.T) string {
	t.Helper()
	// An already-running Redis opts in, the way pgtest's
	// BOWRAIN_TEST_POSTGRES_URL does, so these run on a machine with no
	// container runtime. Each test flushes it, since they share one instance
	// where a container per test would isolate them.
	if url := os.Getenv("BOWRAIN_TEST_REDIS_URL"); url != "" {
		opts, err := redis.ParseURL(url)
		require.NoError(t, err)
		client := redis.NewClient(opts)
		require.NoError(t, client.FlushAll(context.Background()).Err())
		require.NoError(t, client.Close())
		return url
	}
	if testing.Short() {
		t.Skip("skipping Redis container test in -short mode")
	}
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
		Started:      true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	return "redis://" + host + ":" + port.Port()
}

// newRedisBus starts a throwaway Redis and returns an event bus on it.
func newRedisBus(t *testing.T) *RedisEventBus {
	t.Helper()
	return newRedisBusAt(t, startRedis(t))
}

// newRedisBusAt returns a bus on an already-running Redis, so a test can model
// separate server instances (distinct consumer IDs) sharing one stream.
func newRedisBusAt(t *testing.T, url string) *RedisEventBus {
	t.Helper()
	bus, err := NewRedisEventBus(url, "")
	require.NoError(t, err)
	t.Cleanup(bus.Close)
	return bus
}

func waitEvent(t *testing.T, ch <-chan platev.Event, timeout time.Duration) platev.Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for event")
		return platev.Event{}
	}
}

// SubscribeAll delivers every published event.
func TestRedisFanoutDelivers(t *testing.T) {
	bus := newRedisBus(t)
	got := make(chan platev.Event, 4)
	bus.SubscribeAll(func(ev platev.Event) { got <- ev })

	bus.Publish(platev.Event{Type: "test.a"})
	ev := waitEvent(t, got, 5*time.Second)
	assert.Equal(t, platev.EventType("test.a"), ev.Type)
	assert.NotEmpty(t, ev.ID, "Publish must stamp an ID")
}

// Subscribe(type) filters to that type; other types never reach the handler.
func TestRedisTypeFilter(t *testing.T) {
	bus := newRedisBus(t)
	got := make(chan platev.Event, 4)
	bus.Subscribe("test.a", func(ev platev.Event) { got <- ev })

	bus.Publish(platev.Event{Type: "test.b"})
	bus.Publish(platev.Event{Type: "test.a"})

	ev := waitEvent(t, got, 5*time.Second)
	assert.Equal(t, platev.EventType("test.a"), ev.Type)

	select {
	case extra := <-got:
		t.Fatalf("received an event of a filtered-out type: %s", extra.Type)
	case <-time.After(500 * time.Millisecond):
	}
}

// Two independent fan-out subscribers each receive every event — this is the
// SSE-relay semantics, distinct from a consumer group.
func TestRedisTwoFanoutSubscribersBothReceive(t *testing.T) {
	bus := newRedisBus(t)
	a := make(chan platev.Event, 2)
	b := make(chan platev.Event, 2)
	bus.SubscribeAll(func(ev platev.Event) { a <- ev })
	bus.SubscribeAll(func(ev platev.Event) { b <- ev })

	bus.Publish(platev.Event{Type: "test.a"})
	assert.Equal(t, platev.EventType("test.a"), waitEvent(t, a, 5*time.Second).Type)
	assert.Equal(t, platev.EventType("test.a"), waitEvent(t, b, 5*time.Second).Type)
}

// A consumer group receives every event published after it joins, in order,
// via XREADGROUP.
func TestRedisGroupDelivers(t *testing.T) {
	bus := newRedisBus(t)
	got := make(chan platev.Event, 8)
	bus.SubscribeGroup("recorder", func(ev platev.Event) error { got <- ev; return nil })

	for range 3 {
		bus.Publish(platev.Event{Type: "test.g"})
	}
	for i := range 3 {
		ev := waitEvent(t, got, 5*time.Second)
		assert.Equal(t, platev.EventType("test.g"), ev.Type, "event %d", i)
	}
}

// The deploy-rollover scenario that motivated the durable on-push trigger: an
// event published while NO consumer of the group is up (old instance gone, new
// one not yet subscribed) is delivered once any instance rejoins the group —
// the group's acknowledged position survives subscriber downtime. Modeled with
// two separate bus instances (distinct consumer IDs) on one Redis, like two
// server generations sharing the stream, plus a publisher standing in for
// whatever emits the event mid-rollover.
func TestRedisGroupResumesAfterDowntime(t *testing.T) {
	url := startRedis(t)
	publisher := newRedisBusAt(t, url)

	// Generation 1 joins the group, handles one event, then goes away entirely
	// (deploy rollover: the old process exits).
	gen1 := newRedisBusAt(t, url)
	got1 := make(chan platev.Event, 8)
	gen1.SubscribeGroup("rollover", func(ev platev.Event) error { got1 <- ev; return nil })
	publisher.Publish(platev.Event{Type: "test.before"})
	assert.Equal(t, platev.EventType("test.before"), waitEvent(t, got1, 5*time.Second).Type)
	gen1.Close()

	// Published during the downtime window — the event a plain Subscribe tail
	// would lose forever.
	publisher.Publish(platev.Event{Type: "test.during"})

	// Generation 2 rejoins the same group: the missed event is delivered, and
	// the already-acknowledged one is NOT redelivered.
	gen2 := newRedisBusAt(t, url)
	got2 := make(chan platev.Event, 8)
	gen2.SubscribeGroup("rollover", func(ev platev.Event) error { got2 <- ev; return nil })
	ev := waitEvent(t, got2, 5*time.Second)
	assert.Equal(t, platev.EventType("test.during"), ev.Type,
		"the event published during subscriber downtime must be delivered on rejoin")

	select {
	case extra := <-got2:
		t.Fatalf("acknowledged event redelivered: %s", extra.Type)
	case <-time.After(500 * time.Millisecond):
	}
}

// The contrast that makes SubscribeGroup load-bearing for state-advancing
// consumers: a fan-out Subscribe tails from the CURRENT stream position, so an
// event published before the subscriber (re)attaches never reaches it. This
// pins the semantics — do not "fix" it; fan-out is correct for the SSE/gRPC
// relays, and durable consumers must use SubscribeGroup instead.
func TestRedisFanoutMissesEventsBeforeSubscribe(t *testing.T) {
	url := startRedis(t)
	publisher := newRedisBusAt(t, url)

	publisher.Publish(platev.Event{Type: "test.before"})
	// Give the XADD time to land so the subscriber's head resolution follows it.
	time.Sleep(200 * time.Millisecond)

	sub := newRedisBusAt(t, url)
	got := make(chan platev.Event, 8)
	sub.SubscribeAll(func(ev platev.Event) { got <- ev })

	publisher.Publish(platev.Event{Type: "test.after"})
	assert.Equal(t, platev.EventType("test.after"), waitEvent(t, got, 5*time.Second).Type,
		"a fan-out subscriber sees only events published after it attached")

	select {
	case extra := <-got:
		t.Fatalf("fan-out unexpectedly delivered a pre-subscription event: %s", extra.Type)
	case <-time.After(500 * time.Millisecond):
	}
}

// newReclaimBusAt returns a bus whose PEL-reclaim sweep runs on a short
// interval/threshold so a test can observe a reclaim within seconds. The
// fields are set before any SubscribeGroup starts a sweep goroutine.
func newReclaimBusAt(t *testing.T, url string, interval, minIdle time.Duration) *RedisEventBus {
	t.Helper()
	bus := newRedisBusAt(t, url)
	bus.reclaimInterval = interval
	bus.reclaimMinIdle = minIdle
	return bus
}

// syncBuffer is a goroutine-safe bytes.Buffer for capturing slog output
// written from the bus's background goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// The crashed-consumer scenario the XAUTOCLAIM sweep exists for: consumer A
// reads an event via XREADGROUP but dies before acknowledging it, stranding
// the entry in A's pending-entries list — invisible to every other consumer's
// ">" read. Consumer B (same group, new per-process consumer ID) must reclaim
// and handle the stranded event exactly once, must NOT re-dispatch the event A
// already acked, must WARN about the reclaim, and should prune A's leftover
// consumer identity once its PEL is empty.
func TestRedisGroupReclaimsStrandedPendingAfterCrash(t *testing.T) {
	url := startRedis(t)
	publisher := newRedisBusAt(t, url)

	// Capture the bus's slog output to assert the reclaim WARN.
	var logs syncBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	// Consumer A: a raw XREADGROUP client, modeling the process that dies
	// mid-handler. It reads both events, acks the first, and goes away with
	// the second still pending.
	ctx := context.Background()
	opts, err := redis.ParseURL(url)
	require.NoError(t, err)
	dead := redis.NewClient(opts)
	t.Cleanup(func() { _ = dead.Close() })
	require.NoError(t, dead.XGroupCreateMkStream(ctx, defaultEventStream, "crash", "$").Err())

	publisher.Publish(platev.Event{Type: "test.acked"})
	publisher.Publish(platev.Event{Type: "test.stranded"})

	res, err := dead.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    "crash",
		Consumer: "dead-consumer",
		Streams:  []string{defaultEventStream, ">"},
		Count:    10,
		Block:    5 * time.Second,
	}).Result()
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.Len(t, res[0].Messages, 2, "consumer A must have both events delivered before crashing")
	require.NoError(t, dead.XAck(ctx, defaultEventStream, "crash", res[0].Messages[0].ID).Err())
	require.NoError(t, dead.Close()) // the crash: test.stranded stays delivered-but-unacked

	// Consumer B: same group, fresh consumer ID, short reclaim settings.
	sweeper := newReclaimBusAt(t, url, 150*time.Millisecond, 250*time.Millisecond)
	got := make(chan platev.Event, 8)
	sweeper.SubscribeGroup("crash", func(ev platev.Event) error { got <- ev; return nil })

	ev := waitEvent(t, got, 10*time.Second)
	assert.Equal(t, platev.EventType("test.stranded"), ev.Type,
		"the un-acked event must be reclaimed from the dead consumer's PEL")

	// Exactly once: several further sweep intervals pass without the reclaimed
	// event being redelivered, and the already-acked event never reappears.
	select {
	case extra := <-got:
		t.Fatalf("event redelivered after reclaim: %s", extra.Type)
	case <-time.After(time.Second):
	}

	assert.Contains(t, logs.String(), "reclaimed stranded pending entries",
		"the sweep must WARN when it reclaims from a dead consumer")
	assert.Contains(t, logs.String(), "group=crash")

	// Dead-consumer hygiene: once its PEL is empty and it has been idle past
	// the threshold, the sweep deletes the leftover consumer identity.
	require.Eventually(t, func() bool {
		consumers, err := publisher.client.XInfoConsumers(ctx, defaultEventStream, "crash").Result()
		if err != nil {
			return false
		}
		for _, c := range consumers {
			if c.Name == "dead-consumer" {
				return false
			}
		}
		return true
	}, 5*time.Second, 100*time.Millisecond, "dead consumer must be pruned once nothing is pending")
}
