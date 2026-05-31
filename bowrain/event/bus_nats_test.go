package event

import (
	"context"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// --- pure unit tests (no broker required) -----------------------------------

func TestSanitizeDurable(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "relay", "relay"},
		{"alnum-dash-underscore-kept", "Pod-1_abc", "Pod-1_abc"},
		{"dots-replaced", "block.updated", "block_updated"},
		{"nats-wildcards-replaced", "EVENTS.>", "EVENTS__"},
		{"star-replaced", "a*b", "a_b"},
		{"whitespace-replaced", "a b\tc", "a_b_c"},
		{"empty-gets-placeholder", "", "x"},
		{"only-illegal", "...", "___"},
		{"unicode-replaced", "café", "caf_"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeDurable(tt.in))
		})
	}
}

func TestSanitizeDurableIsDeterministic(t *testing.T) {
	in := "weird.name with*chars"
	assert.Equal(t, sanitizeDurable(in), sanitizeDurable(in),
		"sanitizeDurable must be a pure function so durable names are stable across restarts")
}

func TestResolveInstanceIDPrefersEnv(t *testing.T) {
	t.Setenv(instanceIDEnv, "bowrain-server-2")
	assert.Equal(t, "bowrain-server-2", resolveInstanceID())
}

func TestResolveInstanceIDSanitizesEnv(t *testing.T) {
	t.Setenv(instanceIDEnv, "pod.name.with.dots")
	assert.Equal(t, "pod_name_with_dots", resolveInstanceID())
}

func TestResolveInstanceIDFallsBackToHostname(t *testing.T) {
	t.Setenv(instanceIDEnv, "")
	// Hostname is almost always available in CI; the result must be non-empty,
	// sanitized, and stable across calls (so the durable name is stable).
	first := resolveInstanceID()
	require.NotEmpty(t, first)
	assert.Equal(t, first, resolveInstanceID())
	assert.Equal(t, sanitizeDurable(first), first, "instance ID must already be a valid durable name")
}

// --- integration tests (embedded NATS JetStream) ----------------------------

func startTestNATS(t *testing.T) string {
	t.Helper()
	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	srv, err := natsserver.NewServer(opts)
	require.NoError(t, err)

	srv.Start()
	t.Cleanup(srv.Shutdown)

	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready within 5s")
	}
	return srv.ClientURL()
}

func newTestBus(t *testing.T, url, instanceID string) *NATSEventBus {
	t.Helper()
	t.Setenv(instanceIDEnv, instanceID)
	bus, err := NewNATSEventBus(url)
	require.NoError(t, err)
	return bus
}

// crashBus simulates a hard process exit: it stops the consume-loop goroutines
// and closes the NATS connection WITHOUT calling Unsubscribe/Close, so the
// JetStream durables are left intact (and replay on the next boot). Stopping
// the loops keeps goleak happy without exercising the graceful delete path.
func crashBus(b *NATSEventBus) {
	b.mu.Lock()
	consumers := make([]*natsConsumer, 0, len(b.consumers))
	for _, nc := range b.consumers {
		consumers = append(consumers, nc)
	}
	b.mu.Unlock()
	for _, nc := range consumers {
		nc.cancel()
	}
	for _, nc := range consumers {
		<-nc.done
	}
	b.nc.Close()
}

// listConsumers returns the durable names currently defined on the EVENTS stream.
func listConsumers(t *testing.T, url string) []string {
	t.Helper()
	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := js.Stream(ctx, natsEventStream)
	require.NoError(t, err)

	var names []string
	for name := range stream.ConsumerNames(ctx).Name() {
		names = append(names, name)
	}
	return names
}

func TestNATSSubscribeAllUsesDeterministicDurableName(t *testing.T) {
	url := startTestNATS(t)
	bus := newTestBus(t, url, "instance-A")
	defer bus.Close()

	sub := bus.SubscribeAll(func(platev.Event) {})
	require.NotNil(t, sub)

	// The durable name is derived from the instance ID and the call ordinal —
	// no random component — so it recurs identically across restarts.
	assert.Equal(t, "all-instance-A-1", sub.Group)

	names := listConsumers(t, url)
	assert.Contains(t, names, "all-instance-A-1")
}

func TestNATSSubscribeAllOrdinalIsStableAcrossRestart(t *testing.T) {
	url := startTestNATS(t)

	// First "boot": two SubscribeAll callers (e.g. ChangeRelay then GraphSyncer).
	bus1 := newTestBus(t, url, "instance-A")
	s1 := bus1.SubscribeAll(func(platev.Event) {})
	s2 := bus1.SubscribeAll(func(platev.Event) {})
	assert.Equal(t, "all-instance-A-1", s1.Group)
	assert.Equal(t, "all-instance-A-2", s2.Group)
	// Simulate a hard crash: durables persist on the server (no graceful delete).
	crashBus(bus1)

	// Second "boot": same registration order yields the same durable names, so
	// they re-bind to the existing durables and replay pending messages.
	bus2 := newTestBus(t, url, "instance-A")
	defer bus2.Close()
	r1 := bus2.SubscribeAll(func(platev.Event) {})
	r2 := bus2.SubscribeAll(func(platev.Event) {})
	assert.Equal(t, "all-instance-A-1", r1.Group)
	assert.Equal(t, "all-instance-A-2", r2.Group)
}

func TestNATSDistinctInstancesGetDistinctConsumers(t *testing.T) {
	url := startTestNATS(t)

	busA := newTestBus(t, url, "instance-A")
	defer busA.Close()
	busB := newTestBus(t, url, "instance-B")
	defer busB.Close()

	subA := busA.SubscribeAll(func(platev.Event) {})
	subB := busB.SubscribeAll(func(platev.Event) {})

	require.NotEqual(t, subA.Group, subB.Group)
	names := listConsumers(t, url)
	assert.Contains(t, names, subA.Group)
	assert.Contains(t, names, subB.Group)
}

// TestNATSSubscribeAllReplaysPendingMessagesAfterRestart is the core regression
// for audit finding #46: a relay/graph subscriber that goes down before acking
// must receive the pending message after a restart, because its durable name is
// stable and JetStream retained the message.
func TestNATSSubscribeAllReplaysPendingMessagesAfterRestart(t *testing.T) {
	url := startTestNATS(t)

	publisher := newTestBus(t, url, "publisher")
	defer publisher.Close()

	// First boot: create the durable consumer, then simulate a crash before any
	// message is delivered. CreateOrUpdateConsumer with a stable name registers
	// interest so JetStream (InterestPolicy) retains messages published next.
	bus1 := newTestBus(t, url, "instance-A")
	sub1 := bus1.SubscribeAll(func(platev.Event) {})
	require.NotNil(t, sub1)
	require.Equal(t, "all-instance-A-1", sub1.Group)
	// Hard crash: tear down the consume loop + connection without deleting the
	// durable (no Unsubscribe/Close).
	crashBus(bus1)

	// Publish while instance-A is "down". The durable retains the message.
	publisher.Publish(platev.Event{Type: platev.EventBlockUpdated, ProjectID: "p1"})

	// Second boot: re-bind to the same durable name and confirm the pending
	// message is replayed.
	bus2 := newTestBus(t, url, "instance-A")
	defer bus2.Close()
	var got2 int
	var mu2 sync.Mutex
	done := make(chan struct{}, 1)
	sub2 := bus2.SubscribeAll(func(ev platev.Event) {
		mu2.Lock()
		got2++
		mu2.Unlock()
		assert.Equal(t, platev.EventBlockUpdated, ev.Type)
		select {
		case done <- struct{}{}:
		default:
		}
	})
	require.Equal(t, "all-instance-A-1", sub2.Group)

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("pending message was not replayed after restart")
	}
	mu2.Lock()
	assert.GreaterOrEqual(t, got2, 1)
	mu2.Unlock()
}

func TestNATSUnsubscribeDeletesFanoutConsumer(t *testing.T) {
	url := startTestNATS(t)
	bus := newTestBus(t, url, "instance-A")
	defer bus.Close()

	sub := bus.SubscribeAll(func(platev.Event) {})
	require.Contains(t, listConsumers(t, url), sub.Group)

	bus.Unsubscribe(sub)

	// Fan-out consumers are reclaimed on graceful Unsubscribe so they don't
	// accumulate; a crash (no Unsubscribe) leaves them for replay instead.
	assert.NotContains(t, listConsumers(t, url), sub.Group)
}

func TestNATSSubscribeTypedUsesInstanceScopedDurable(t *testing.T) {
	url := startTestNATS(t)
	bus := newTestBus(t, url, "instance-A")
	defer bus.Close()

	sub := bus.Subscribe(platev.EventBlockUpdated, func(platev.Event) {})
	require.NotNil(t, sub)
	assert.Equal(t, "type-block_updated-instance-A-1", sub.Group)

	bus.Unsubscribe(sub)
	assert.NotContains(t, listConsumers(t, url), sub.Group)
}

func TestNATSSubscribeGroupSharesDurableAndSurvivesUnsubscribe(t *testing.T) {
	url := startTestNATS(t)
	bus := newTestBus(t, url, "instance-A")
	defer bus.Close()

	sub := bus.SubscribeGroup("audit-logger", func(platev.Event) {})
	require.NotNil(t, sub)
	assert.Equal(t, "audit-logger", sub.Group)
	require.Contains(t, listConsumers(t, url), "audit-logger")

	// Group durables are shared across instances and must NOT be deleted when a
	// single subscriber unsubscribes.
	bus.Unsubscribe(sub)
	assert.Contains(t, listConsumers(t, url), "audit-logger")
}

func TestNATSFanoutDeliversToEveryInstance(t *testing.T) {
	url := startTestNATS(t)

	publisher := newTestBus(t, url, "publisher")
	defer publisher.Close()

	recv := func(bus *NATSEventBus) (<-chan struct{}, *platev.Subscription) {
		ch := make(chan struct{}, 4)
		sub := bus.SubscribeAll(func(platev.Event) {
			select {
			case ch <- struct{}{}:
			default:
			}
		})
		return ch, sub
	}

	busA := newTestBus(t, url, "instance-A")
	defer busA.Close()
	busB := newTestBus(t, url, "instance-B")
	defer busB.Close()

	chA, _ := recv(busA)
	chB, _ := recv(busB)

	// Give both consumers a moment to bind before publishing.
	time.Sleep(500 * time.Millisecond)
	publisher.Publish(platev.Event{Type: platev.EventBlockUpdated, ProjectID: "p1"})

	for _, ch := range []<-chan struct{}{chA, chB} {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatal("fan-out event not delivered to an instance (instances are competing instead of each receiving a copy)")
		}
	}
}
