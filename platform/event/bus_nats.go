package event

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/neokapi/neokapi/core/id"
	platev "github.com/neokapi/neokapi/platform/event"
)

const (
	natsEventStream  = "EVENTS"
	natsEventSubject = "EVENTS"   // publish to EVENTS.<type>
	natsEventFilter  = "EVENTS.>" // subscribe to all events
	natsMaxDeliver   = 5
	natsEventAckWait = 30 * time.Second
)

// NATSEventBus implements EventBus using NATS JetStream.
// Publish sends to EVENTS.<type>. SubscribeGroup creates a durable
// consumer per group with competing consumer semantics.
type NATSEventBus struct {
	nc *nats.Conn
	js jetstream.JetStream

	mu        sync.Mutex
	consumers map[string]*natsConsumer
	closed    bool
}

type natsConsumer struct {
	sub    *platev.Subscription
	cancel context.CancelFunc
	done   chan struct{}
}

// NewNATSEventBus creates an event bus backed by NATS JetStream.
func NewNATSEventBus(url string) (*NATSEventBus, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, err
	}

	// Create or update the events stream.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      natsEventStream,
		Subjects:  []string{natsEventFilter},
		Retention: jetstream.InterestPolicy, // Keep messages while consumers have interest
		MaxAge:    7 * 24 * time.Hour,
	})
	if err != nil {
		nc.Close()
		return nil, err
	}

	return &NATSEventBus{
		nc:        nc,
		js:        js,
		consumers: make(map[string]*natsConsumer),
	}, nil
}

// Publish sends an event to EVENTS.<type>.
func (b *NATSEventBus) Publish(ev platev.Event) {
	if ev.ID == "" {
		ev.ID = id.New()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(ev)
	if err != nil {
		log.Printf("nats-event-bus: marshal error: %v", err)
		return
	}

	subject := natsEventSubject + "." + string(ev.Type)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := b.js.Publish(ctx, subject, data); err != nil {
		log.Printf("nats-event-bus: publish error: %v", err)
	}
}

// Subscribe creates a consumer filtered by event type.
func (b *NATSEventBus) Subscribe(eventType platev.EventType, handler platev.EventHandler) *platev.Subscription {
	return b.subscribeInternal(string(eventType), natsEventSubject+"."+string(eventType), handler)
}

// SubscribeAll subscribes to all events.
func (b *NATSEventBus) SubscribeAll(handler platev.EventHandler) *platev.Subscription {
	return b.subscribeInternal("all-"+id.New()[:4], natsEventFilter, handler)
}

// SubscribeGroup creates a durable consumer with competing consumer semantics.
func (b *NATSEventBus) SubscribeGroup(group string, handler platev.EventHandler) *platev.Subscription {
	return b.subscribeInternal(group, natsEventFilter, handler)
}

func (b *NATSEventBus) subscribeInternal(consumerName, filter string, handler platev.EventHandler) *platev.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	sub := &platev.Subscription{
		ID:      id.New(),
		Group:   consumerName,
		Handler: handler,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create or update durable consumer.
	consumer, err := b.js.CreateOrUpdateConsumer(ctx, natsEventStream, jetstream.ConsumerConfig{
		Durable:       consumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: filter,
		MaxDeliver:    natsMaxDeliver,
		AckWait:       natsEventAckWait,
	})
	if err != nil {
		log.Printf("nats-event-bus: failed to create consumer %s: %v", consumerName, err)
		cancel()
		return sub
	}

	nc := &natsConsumer{
		sub:    sub,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	b.consumers[sub.ID] = nc

	go b.consumeLoop(ctx, consumer, nc)

	return sub
}

func (b *NATSEventBus) consumeLoop(ctx context.Context, consumer jetstream.Consumer, nc *natsConsumer) {
	defer close(nc.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}

		for msg := range batch.Messages() {
			var ev platev.Event
			if err := json.Unmarshal(msg.Data(), &ev); err != nil {
				log.Printf("nats-event-bus: unmarshal error: %v", err)
				_ = msg.Ack()
				continue
			}

			nc.sub.Handler(ev)
			_ = msg.Ack()
		}
	}
}

// Unsubscribe stops a consumer.
func (b *NATSEventBus) Unsubscribe(sub *platev.Subscription) {
	b.mu.Lock()
	nc, ok := b.consumers[sub.ID]
	if ok {
		delete(b.consumers, sub.ID)
	}
	b.mu.Unlock()

	if ok {
		nc.cancel()
		<-nc.done
	}
}

// Close shuts down all consumers and the NATS connection.
func (b *NATSEventBus) Close() {
	b.mu.Lock()
	b.closed = true
	consumers := make([]*natsConsumer, 0, len(b.consumers))
	for _, nc := range b.consumers {
		consumers = append(consumers, nc)
	}
	b.consumers = make(map[string]*natsConsumer)
	b.mu.Unlock()

	for _, nc := range consumers {
		nc.cancel()
		<-nc.done
	}
	b.nc.Close()
}
