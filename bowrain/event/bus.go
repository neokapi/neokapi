package event

import (
	"log"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
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
				log.Printf("WARNING: event bus dropping event %s (type=%s) for subscriber %s: channel full", ev.ID, ev.Type, s.sub.ID)
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

// SubscribeGroup registers a handler with a named consumer group.
// For ChannelEventBus, group is stored but all subscribers still receive all events
// (no competing consumer semantics in-process).
func (b *ChannelEventBus) SubscribeGroup(group string, handler platev.EventHandler) *platev.Subscription {
	sub := &platev.Subscription{
		ID:      id.New(),
		Group:   group,
		Handler: handler,
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
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("ERROR: recovered panic in event handler %s: %v", sub.ID, r)
					}
				}()
				sub.Handler(ev)
			}()
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
