package event

import (
	"sync/atomic"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRedisGroupSlowHandlerIsNotReclaimedFromItself: the reclaim sweep claims
// to the same consumer as the read loop, so an entry whose handler is merely
// slow looks exactly like one whose consumer died. Dispatching it again would
// run two copies of the same handler, in one process, at the same time — a
// second inbox row and a second email, from a batch that was only taking its
// time.
func TestRedisGroupSlowHandlerIsNotReclaimedFromItself(t *testing.T) {
	url := startRedis(t)
	publisher := newRedisBusAt(t, url)

	var dispatches atomic.Int32
	started := make(chan struct{}, 4)
	finish := make(chan struct{})

	// Reclaim settings far shorter than the handler takes, so several sweeps
	// see the entry as idle while it is still being worked on.
	consumer := newReclaimBusAt(t, url, 100*time.Millisecond, 100*time.Millisecond)
	consumer.SubscribeGroup("slow", func(platev.Event) error {
		dispatches.Add(1)
		started <- struct{}{}
		<-finish
		return nil
	})
	awaitGroup(t, publisher, "slow")

	publisher.Publish(platev.Event{Type: "test.slow"})
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("handler never started")
	}

	// Several sweep intervals pass while the handler holds the entry.
	time.Sleep(700 * time.Millisecond)
	assert.Equal(t, int32(1), dispatches.Load(), "a slow handler must not be reclaimed from itself")

	close(finish)
	require.Eventually(t, func() bool {
		return pendingCount(t, publisher, "slow") == 0
	}, 10*time.Second, 50*time.Millisecond, "the finished handler acknowledges its entry")
	assert.Equal(t, int32(1), dispatches.Load())
}
