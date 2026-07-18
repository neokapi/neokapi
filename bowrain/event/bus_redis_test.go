package event

import (
	"context"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
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
	if testing.Short() {
		t.Skip("skipping Redis container test in -short mode")
	}
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
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
	bus.SubscribeGroup("recorder", func(ev platev.Event) { got <- ev })

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
	gen1.SubscribeGroup("rollover", func(ev platev.Event) { got1 <- ev })
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
	gen2.SubscribeGroup("rollover", func(ev platev.Event) { got2 <- ev })
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
