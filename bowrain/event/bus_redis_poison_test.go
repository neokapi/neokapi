package event

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// A handler that fails the same entry on every delivery must not hold the
// group's reclaim sweep forever: 70k retries per half hour once ground the
// production server. Past poisonDeliveryCap the sweep acks the entry away and
// says so at ERROR.
func TestGroupReclaim_DropsPoisonEntryAfterCap(t *testing.T) {
	url := startRedis(t)
	bus := newReclaimBusAt(t, url, 30*time.Millisecond, 10*time.Millisecond)

	var calls atomic.Int64
	bus.SubscribeGroup("poison-test", func(_ platev.Event) error {
		calls.Add(1)
		return errors.New("always fails")
	})

	// The group reader starts asynchronously; an event published before the
	// group exists is never delivered to it. Publish until the handler has
	// seen at least one delivery, then watch that delivery get poisoned.
	require.Eventually(t, func() bool {
		bus.Publish(platev.Event{Type: "block.updated", WorkspaceID: "ws"})
		return calls.Load() > 0
	}, 10*time.Second, 200*time.Millisecond, "the group never received a delivery")

	// The entry is delivered, fails, and is reclaimed until the cap drops it:
	// the group's pending list must eventually go (and stay) empty.
	// The handler never succeeds, so the only way the pending list drains is
	// the sweep's poison drop. (The drop's ERROR line goes through the global
	// slog default, which other tests in this package swap — asserting on it
	// here would race them.)
	require.Eventually(t, func() bool {
		pending, perr := bus.client.XPending(context.Background(), bus.stream, "poison-test").Result()
		return perr == nil && pending.Count == 0
	}, 30*time.Second, 500*time.Millisecond, "the pending list never drained")
	// The cap is a maximum, not an overshoot: a single poisoned entry is
	// delivered exactly poisonDeliveryCap times before the sweep acks it away,
	// so equality is the deterministic single-entry outcome. Strictly-greater
	// only held when the publish-until loop happened to enqueue extra events.
	assert.GreaterOrEqual(t, calls.Load(), int64(poisonDeliveryCap),
		"the entry should have been retried up to the cap before the drop")
}
