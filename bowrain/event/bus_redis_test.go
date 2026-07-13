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

// newRedisBus starts a throwaway Redis and returns an event bus on it. Each test
// gets its own container so streams and consumer groups stay isolated. Exercises
// real Redis Streams (XADD/XREAD/XREADGROUP) — the same code path as ElastiCache
// in production, per the no-mocks rule.
func newRedisBus(t *testing.T) *RedisEventBus {
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

	bus, err := NewRedisEventBus("redis://"+host+":"+port.Port(), "")
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
