package event

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// awaitGroup waits until a consumer group exists on the stream. A group is
// created at the current head, so anything published before it exists is not
// its to read.
func awaitGroup(t *testing.T, bus *RedisEventBus, name string) {
	t.Helper()
	require.Eventually(t, func() bool {
		groups, err := bus.client.XInfoGroups(context.Background(), defaultEventStream).Result()
		if err != nil {
			return false
		}
		for _, g := range groups {
			if g.Name == name {
				return true
			}
		}
		return false
	}, 5*time.Second, 25*time.Millisecond, "consumer group %q never appeared", name)
}

func pendingCount(t *testing.T, bus *RedisEventBus, group string) int64 {
	t.Helper()
	p, err := bus.client.XPending(context.Background(), defaultEventStream, group).Result()
	if err != nil {
		return -1
	}
	return p.Count
}

// TestRedisGroupHandlerFailureStaysPending is the durability property the
// error-returning group contract exists for: a handler that could not do its
// work must not have its event acknowledged.
//
// The scenario is a database blip during an audit append. With an
// unconditional ack the record was gone for good, and — because the hash chain
// links each row to the previous SURVIVING row — verification still passed over
// the gap. Now the entry stays in the pending list until the reclaim sweep
// hands it back, and it lands exactly once.
func TestRedisGroupHandlerFailureStaysPending(t *testing.T) {
	url := startRedis(t)
	publisher := newRedisBusAt(t, url)

	var down atomic.Bool
	down.Store(true)
	handled := make(chan platev.Event, 8)

	consumer := newReclaimBusAt(t, url, 150*time.Millisecond, 250*time.Millisecond)
	consumer.SubscribeGroup("durable", func(ev platev.Event) error {
		if down.Load() {
			return errors.New("database is down")
		}
		handled <- ev
		return nil
	})
	awaitGroup(t, publisher, "durable")

	publisher.Publish(platev.Event{Type: "member.role_changed"})

	// While the store is down the entry is delivered and not acknowledged, so
	// it sits in the consumer's pending list rather than vanishing.
	require.Eventually(t, func() bool {
		return pendingCount(t, publisher, "durable") == 1
	}, 5*time.Second, 50*time.Millisecond, "a failed handler must leave its event pending")

	select {
	case ev := <-handled:
		t.Fatalf("handler reported failure yet the event was treated as handled: %s", ev.Type)
	case <-time.After(300 * time.Millisecond):
	}

	// The store comes back. The reclaim sweep finds the entry idle past the
	// threshold and redelivers it to this same consumer.
	down.Store(false)
	ev := waitEvent(t, handled, 10*time.Second)
	assert.Equal(t, platev.EventType("member.role_changed"), ev.Type)

	// Exactly once: no further sweep interval produces a second delivery, and
	// nothing is left pending.
	select {
	case extra := <-handled:
		t.Fatalf("event handled twice: %s", extra.Type)
	case <-time.After(time.Second):
	}
	require.Eventually(t, func() bool {
		return pendingCount(t, publisher, "durable") == 0
	}, 5*time.Second, 50*time.Millisecond, "the recovered handler acknowledges its event")
}

// TestRedisGroupPanicStaysPending covers the same property for a handler that
// did not return at all. A panicking handler has finished no work, so treating
// it as delivered would lose the event just as silently.
func TestRedisGroupPanicStaysPending(t *testing.T) {
	url := startRedis(t)
	publisher := newRedisBusAt(t, url)

	var boom atomic.Bool
	boom.Store(true)
	handled := make(chan platev.Event, 4)

	consumer := newReclaimBusAt(t, url, 150*time.Millisecond, 250*time.Millisecond)
	consumer.SubscribeGroup("panicky", func(ev platev.Event) error {
		if boom.Load() {
			panic("nil map write")
		}
		handled <- ev
		return nil
	})
	awaitGroup(t, publisher, "panicky")

	publisher.Publish(platev.Event{Type: "test.panic"})
	require.Eventually(t, func() bool {
		return pendingCount(t, publisher, "panicky") == 1
	}, 5*time.Second, 50*time.Millisecond, "a panicking handler must leave its event pending")

	boom.Store(false)
	assert.Equal(t, platev.EventType("test.panic"), waitEvent(t, handled, 10*time.Second).Type)
}

// TestRedisGroupPoisonPayloadIsAcked keeps the one exception in place: an entry
// that will never decode must not be redelivered forever.
func TestRedisGroupPoisonPayloadIsAcked(t *testing.T) {
	url := startRedis(t)
	publisher := newRedisBusAt(t, url)
	ctx := context.Background()

	consumer := newReclaimBusAt(t, url, 150*time.Millisecond, 250*time.Millisecond)
	consumer.SubscribeGroup("poison", func(platev.Event) error {
		return errors.New("must never be called for an undecodable entry")
	})
	awaitGroup(t, publisher, "poison")

	require.NoError(t, publisher.client.XAdd(ctx, &redis.XAddArgs{
		Stream: defaultEventStream,
		Values: map[string]any{"data": "{not json", "type": "test.poison"},
	}).Err())

	require.Eventually(t, func() bool {
		return pendingCount(t, publisher, "poison") == 0
	}, 5*time.Second, 50*time.Millisecond, "a malformed entry must be acknowledged, not retried forever")
}
