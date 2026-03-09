package event

import (
	"sync"
	"time"

	"github.com/gokapi/gokapi/core/id"
	platev "github.com/gokapi/gokapi/platform/event"
)

// ChannelEventBus is an in-process, channel-based EventBus implementation.
// Each subscriber gets its own goroutine and buffered channel.
type ChannelEventBus struct {
	mu          sync.RWMutex
	subscribers map[string]*subscriber
	closed      bool
}

type subscriber struct {
	sub  *platev.Subscription
	ch   chan platev.Event
	done chan struct{}
}

// NewChannelEventBus creates a new ChannelEventBus.
func NewChannelEventBus() *ChannelEventBus {
	return &ChannelEventBus{
		subscribers: make(map[string]*subscriber),
	}
}

// Publish sends an event to all matching subscribers.
func (b *ChannelEventBus) Publish(ev platev.Event) {
	if ev.ID == "" {
		ev.ID = id.New()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for _, s := range b.subscribers {
		if s.sub.EventType == "" || s.sub.EventType == ev.Type {
			select {
			case s.ch <- ev:
			default:
				// Drop event if subscriber is too slow.
			}
		}
	}
}

// Subscribe registers a handler for a specific event type.
func (b *ChannelEventBus) Subscribe(eventType platev.EventType, handler platev.EventHandler) *platev.Subscription {
	sub := &platev.Subscription{
		ID:        id.New(),
		EventType: eventType,
		Handler:   handler,
	}
	b.addSubscriber(sub)
	return sub
}

// SubscribeAll registers a handler for all events.
func (b *ChannelEventBus) SubscribeAll(handler platev.EventHandler) *platev.Subscription {
	sub := &platev.Subscription{
		ID:      id.New(),
		Handler: handler,
	}
	b.addSubscriber(sub)
	return sub
}

func (b *ChannelEventBus) addSubscriber(sub *platev.Subscription) {
	s := &subscriber{
		sub:  sub,
		ch:   make(chan platev.Event, 64),
		done: make(chan struct{}),
	}

	go func() {
		defer close(s.done)
		for ev := range s.ch {
			sub.Handler(ev)
		}
	}()

	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[sub.ID] = s
}

// Unsubscribe removes a subscription and stops its goroutine.
func (b *ChannelEventBus) Unsubscribe(sub *platev.Subscription) {
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
