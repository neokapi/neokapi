package event

import (
	"sync"
	"testing"
	"time"

	platev "github.com/gokapi/gokapi/platform/event"
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

	time.Sleep(50 * time.Millisecond) // Wait for async delivery

	mu.Lock()
	assert.Len(t, received, 1)
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
		bus.Subscribe(platev.EventBlockCreated, func(e platev.Event) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	bus.Publish(platev.Event{Type: platev.EventBlockCreated})

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

	sub := bus.Subscribe(platev.EventBlockCreated, func(e platev.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(platev.Event{Type: platev.EventBlockCreated})
	time.Sleep(50 * time.Millisecond)

	bus.Unsubscribe(sub)

	bus.Publish(platev.Event{Type: platev.EventBlockCreated})
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
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	require.NotEmpty(t, received.ID)
	assert.False(t, received.Timestamp.IsZero())
	mu.Unlock()
}
