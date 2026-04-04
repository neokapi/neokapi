package event

import (
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishSubscribe(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var received []platev.Event
	var mu sync.Mutex

	bus.Subscribe(platev.EventBlockCreated, func(e platev.Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(platev.Event{Type: platev.EventBlockCreated, Source: "test"})
	bus.Publish(platev.Event{Type: platev.EventBlockUpdated, Source: "test"}) // Should not be received

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	assert.Equal(t, platev.EventBlockCreated, received[0].Type)
	mu.Unlock()
}

func TestSubscribeAll(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var received []platev.Event
	var mu sync.Mutex

	bus.SubscribeAll(func(e platev.Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(platev.Event{Type: platev.EventBlockCreated})
	bus.Publish(platev.Event{Type: platev.EventBlockUpdated})
	bus.Publish(platev.Event{Type: platev.EventProjectCreated})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 3
	}, 2*time.Second, 10*time.Millisecond)
}

func TestMultipleSubscribers(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	count := 0
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		bus.Subscribe(platev.EventBlockCreated, func(e platev.Event) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	bus.Publish(platev.Event{Type: platev.EventBlockCreated})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return count == 5
	}, 2*time.Second, 10*time.Millisecond)
}

func TestUnsubscribe(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	count := 0
	var mu sync.Mutex

	sub := bus.Subscribe(platev.EventBlockCreated, func(e platev.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(platev.Event{Type: platev.EventBlockCreated})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return count == 1
	}, 2*time.Second, 10*time.Millisecond)

	bus.Unsubscribe(sub)

	bus.Publish(platev.Event{Type: platev.EventBlockCreated})

	// After unsubscribe, count should remain 1. Give the bus time to
	// process the event (if it were delivered, count would increase).
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, count)
	mu.Unlock()
}

func TestContextCancellation(t *testing.T) {
	bus := NewChannelEventBus()
	bus.Close() // Close immediately

	// Should not panic.
	bus.Publish(platev.Event{Type: platev.EventBlockCreated})
}

func TestEventIDAndTimestamp(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var received platev.Event
	var mu sync.Mutex

	bus.Subscribe(platev.EventBlockCreated, func(e platev.Event) {
		mu.Lock()
		received = e
		mu.Unlock()
	})

	bus.Publish(platev.Event{Type: platev.EventBlockCreated})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received.ID != ""
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	assert.False(t, received.Timestamp.IsZero())
	mu.Unlock()
}
