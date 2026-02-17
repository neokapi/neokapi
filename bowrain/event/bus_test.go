package event

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishSubscribe(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var received []Event
	var mu sync.Mutex

	bus.Subscribe(EventBlockCreated, func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventBlockCreated, Source: "test"})
	bus.Publish(Event{Type: EventBlockUpdated, Source: "test"}) // Should not be received

	time.Sleep(50 * time.Millisecond) // Wait for async delivery

	mu.Lock()
	assert.Len(t, received, 1)
	assert.Equal(t, EventBlockCreated, received[0].Type)
	mu.Unlock()
}

func TestSubscribeAll(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var received []Event
	var mu sync.Mutex

	bus.SubscribeAll(func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventBlockCreated})
	bus.Publish(Event{Type: EventBlockUpdated})
	bus.Publish(Event{Type: EventProjectCreated})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Len(t, received, 3)
	mu.Unlock()
}

func TestMultipleSubscribers(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	count := 0
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		bus.Subscribe(EventBlockCreated, func(e Event) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	bus.Publish(Event{Type: EventBlockCreated})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 5, count)
	mu.Unlock()
}

func TestUnsubscribe(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	count := 0
	var mu sync.Mutex

	sub := bus.Subscribe(EventBlockCreated, func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventBlockCreated})
	time.Sleep(50 * time.Millisecond)

	bus.Unsubscribe(sub)

	bus.Publish(Event{Type: EventBlockCreated})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, count)
	mu.Unlock()
}

func TestContextCancellation(t *testing.T) {
	bus := NewChannelEventBus()
	bus.Close() // Close immediately

	// Should not panic.
	bus.Publish(Event{Type: EventBlockCreated})
}

func TestEventIDAndTimestamp(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var received Event
	var mu sync.Mutex

	bus.Subscribe(EventBlockCreated, func(e Event) {
		mu.Lock()
		received = e
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventBlockCreated})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	require.NotEmpty(t, received.ID)
	assert.False(t, received.Timestamp.IsZero())
	mu.Unlock()
}
