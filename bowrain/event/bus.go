package event

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// ChannelEventBus is an in-process, channel-based EventBus implementation.
// Each subscriber gets its own goroutine and buffered channel.
type ChannelEventBus struct {
	mu          sync.RWMutex
	subscribers map[string]*subscriber
	closed      bool
}

type subscriber struct {
	sub  *Subscription
	ch   chan Event
	done chan struct{}
}

// NewChannelEventBus creates a new ChannelEventBus.
func NewChannelEventBus() *ChannelEventBus {
	return &ChannelEventBus{
		subscribers: make(map[string]*subscriber),
	}
}

// Publish sends an event to all matching subscribers.
func (b *ChannelEventBus) Publish(event Event) {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for _, s := range b.subscribers {
		if s.sub.EventType == "" || s.sub.EventType == event.Type {
			select {
			case s.ch <- event:
			default:
				// Drop event if subscriber is too slow.
			}
		}
	}
}

// Subscribe registers a handler for a specific event type.
func (b *ChannelEventBus) Subscribe(eventType EventType, handler EventHandler) *Subscription {
	sub := &Subscription{
		ID:        uuid.NewString(),
		EventType: eventType,
		Handler:   handler,
	}
	b.addSubscriber(sub)
	return sub
}

// SubscribeAll registers a handler for all events.
func (b *ChannelEventBus) SubscribeAll(handler EventHandler) *Subscription {
	sub := &Subscription{
		ID:      uuid.NewString(),
		Handler: handler,
	}
	b.addSubscriber(sub)
	return sub
}

func (b *ChannelEventBus) addSubscriber(sub *Subscription) {
	s := &subscriber{
		sub:  sub,
		ch:   make(chan Event, 64),
		done: make(chan struct{}),
	}

	go func() {
		defer close(s.done)
		for event := range s.ch {
			sub.Handler(event)
		}
	}()

	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[sub.ID] = s
}

// Unsubscribe removes a subscription and stops its goroutine.
func (b *ChannelEventBus) Unsubscribe(sub *Subscription) {
	b.mu.Lock()
	s, ok := b.subscribers[sub.ID]
	if ok {
		delete(b.subscribers, sub.ID)
	}
	b.mu.Unlock()

	if ok {
		close(s.ch)
		<-s.done // Wait for goroutine to finish.
	}
}

// Close shuts down all subscribers.
func (b *ChannelEventBus) Close() {
	b.mu.Lock()
	b.closed = true
	subs := make([]*subscriber, 0, len(b.subscribers))
	for _, s := range b.subscribers {
		subs = append(subs, s)
	}
	b.subscribers = make(map[string]*subscriber)
	b.mu.Unlock()

	for _, s := range subs {
		close(s.ch)
		<-s.done
	}
}
