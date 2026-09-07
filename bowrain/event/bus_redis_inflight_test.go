package event

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reclaim sweep claims to the same consumer as the read loop, so an entry
// whose handler is merely slow looks exactly like one whose consumer died.
// Dispatching it again would run two copies of the same handler, in one
// process, at the same time: a second inbox row and a second email, from a
// batch that was only taking its time.
//
// Both tests below drive the sweep by hand. A test that publishes, sleeps and
// hopes a sweep landed inside the window reports nothing when the runner is
// loaded and no sweep ran, and it flakes when one runs at the wrong moment.
// Calling reclaimStranded is the same code path with the timing decided by the
// test.

// awaitStarted waits for the handler to report that it has the entry.
func awaitStarted(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("handler never started")
	}
}

// retryCount reports how many times the group has delivered one entry, which is
// how a test shows that a sweep really claimed it rather than passing because
// nothing happened.
func retryCount(t *testing.T, bus *RedisEventBus, group, msgID string) int64 {
	t.Helper()
	pend, err := bus.client.XPendingExt(context.Background(), &redis.XPendingExtArgs{
		Stream: bus.stream, Group: group, Start: msgID, End: msgID, Count: 1,
	}).Result()
	require.NoError(t, err)
	if len(pend) == 0 {
		return 0
	}
	return pend[0].RetryCount
}

// onlyPendingID returns the id of the group's single pending entry.
func onlyPendingID(t *testing.T, bus *RedisEventBus, group string) string {
	t.Helper()
	var id string
	require.Eventually(t, func() bool {
		pend, err := bus.client.XPendingExt(context.Background(), &redis.XPendingExtArgs{
			Stream: bus.stream, Group: group, Start: "-", End: "+", Count: 8,
		}).Result()
		if err != nil || len(pend) != 1 {
			return false
		}
		id = pend[0].ID
		return true
	}, 10*time.Second, 25*time.Millisecond, "the group should hold exactly one pending entry")
	return id
}

// TestRedisGroupSlowHandlerIsNotReclaimedFromItself: a sweep that runs while a
// handler still holds its entry must leave it alone.
func TestRedisGroupSlowHandlerIsNotReclaimedFromItself(t *testing.T) {
	url := startRedis(t)
	publisher := newRedisBusAt(t, url)

	var dispatches atomic.Int32
	started := make(chan struct{}, 4)
	finish := make(chan struct{})

	handler := func(platev.Event) error {
		dispatches.Add(1)
		started <- struct{}{}
		<-finish
		return nil
	}

	// Nothing is idle-gated and nothing sweeps on its own: every entry
	// qualifies the moment it is pending, and the sweeps below are the test's.
	consumer := newReclaimBusAt(t, url, time.Hour, 0)
	consumer.SubscribeGroup("slow", handler)
	awaitGroup(t, publisher, "slow")

	publisher.Publish(platev.Event{Type: "test.slow"})
	awaitStarted(t, started)
	msgID := onlyPendingID(t, publisher, "slow")

	for range 3 {
		consumer.reclaimStranded(t.Context(), "slow", handler)
	}
	assert.Equal(t, int32(1), dispatches.Load(), "a slow handler must not be reclaimed from itself")
	assert.Greater(t, retryCount(t, publisher, "slow", msgID), int64(1),
		"the sweeps must have claimed the entry, or the assertion above proves nothing")

	close(finish)
	require.Eventually(t, func() bool {
		return pendingCount(t, publisher, "slow") == 0
	}, 10*time.Second, 50*time.Millisecond, "the finished handler acknowledges its entry")
	assert.Equal(t, int32(1), dispatches.Load())
}

// TestRedisGroupReclaimSkipsAnEntryAcknowledgedFirst covers the window between
// the two halves of a sweep. XAUTOCLAIM answers for the moment the server ran
// it; a handler that finishes before the sweep gets to the dispatch has already
// acknowledged the entry, and the claim on the in-flight map is gone with it.
// Dispatching then runs a second copy of work that succeeded.
//
// This is what made the slow-handler test flake on CI: the run for #2511 saw
// two dispatches at the final assertion, after the handler had finished, rather
// than at the one guarding the sweep.
func TestRedisGroupReclaimSkipsAnEntryAcknowledgedFirst(t *testing.T) {
	url := startRedis(t)
	publisher := newRedisBusAt(t, url)

	var dispatches atomic.Int32
	started := make(chan struct{}, 4)
	finish := make(chan struct{})

	handler := func(platev.Event) error {
		dispatches.Add(1)
		started <- struct{}{}
		<-finish
		return nil
	}

	consumer := newReclaimBusAt(t, url, time.Hour, 0)
	consumer.SubscribeGroup("acked", handler)
	awaitGroup(t, publisher, "acked")

	publisher.Publish(platev.Event{Type: "test.acked"})
	awaitStarted(t, started)

	// The sweep's own fetch, taken while the handler still holds the entry.
	msgs, _, err := consumer.client.XAutoClaim(t.Context(), &redis.XAutoClaimArgs{
		Stream:   consumer.stream,
		Group:    "acked",
		Consumer: consumer.consumerID,
		MinIdle:  0,
		Start:    "0-0",
		Count:    8,
	}).Result()
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	// The handler finishes and acknowledges before the sweep reaches its
	// dispatch.
	close(finish)
	require.Eventually(t, func() bool {
		return pendingCount(t, publisher, "acked") == 0
	}, 10*time.Second, 50*time.Millisecond, "the finished handler acknowledges its entry")

	consumer.handleGroupMessage(t.Context(), "acked", handler, msgs[0], true)
	assert.Equal(t, int32(1), dispatches.Load(),
		"an entry acknowledged while the sweep held it must not be dispatched again")
}
