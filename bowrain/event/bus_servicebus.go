package event

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
)

// ServiceBusEventBus implements EventBus using Azure Service Bus topics.
// Publish sends to a topic; Subscribe/SubscribeGroup creates receivers
// on named subscriptions (consumer groups).
type ServiceBusEventBus struct {
	client *azservicebus.Client
	sender *azservicebus.Sender
	topic  string

	mu        sync.Mutex
	receivers map[string]*sbReceiver
	closed    bool
}

type sbReceiver struct {
	receiver *azservicebus.Receiver
	sub      *platev.Subscription
	cancel   context.CancelFunc
	done     chan struct{}
}

const defaultEventTopic = "bowrain-events"

// NewServiceBusEventBus creates an event bus backed by Azure Service Bus.
func NewServiceBusEventBus(connStr string) (*ServiceBusEventBus, error) {
	client, err := azservicebus.NewClientFromConnectionString(connStr, nil)
	if err != nil {
		return nil, err
	}
	sender, err := client.NewSender(defaultEventTopic, nil)
	if err != nil {
		return nil, err
	}
	return &ServiceBusEventBus{
		client:    client,
		sender:    sender,
		topic:     defaultEventTopic,
		receivers: make(map[string]*sbReceiver),
	}, nil
}

// Publish sends an event to the Service Bus topic.
func (b *ServiceBusEventBus) Publish(ev platev.Event) {
	if ev.ID == "" {
		ev.ID = id.New()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(ev)
	if err != nil {
		slog.Info("servicebus-event-bus: marshal error", "error", err)
		return
	}

	msg := &azservicebus.Message{
		Body:    data,
		Subject: new(string(ev.Type)),
	}
	if ev.ID != "" {
		msg.MessageID = &ev.ID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := b.sender.SendMessage(ctx, msg, nil); err != nil {
		slog.Info("servicebus-event-bus: publish error", "error", err)
	}
}

// Subscribe creates a subscription receiver filtered by event type.
// Uses the event type as the subscription name (must pre-exist in Azure).
func (b *ServiceBusEventBus) Subscribe(eventType platev.EventType, handler platev.EventHandler) *platev.Subscription {
	// For type-specific subscriptions, use the type as group name.
	return b.SubscribeGroup(string(eventType), handler)
}

// SubscribeAll subscribes to all events using a catch-all subscription.
// Requires a subscription named with a catch-all rule in Azure.
func (b *ServiceBusEventBus) SubscribeAll(handler platev.EventHandler) *platev.Subscription {
	return b.SubscribeGroup("catch-all", handler)
}

// SubscribeGroup creates a receiver on the named subscription (consumer group).
func (b *ServiceBusEventBus) SubscribeGroup(group string, handler platev.EventHandler) *platev.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	sub := &platev.Subscription{
		ID:      id.New(),
		Group:   group,
		Handler: handler,
	}

	receiver, err := b.client.NewReceiverForSubscription(b.topic, group, nil)
	if err != nil {
		slog.Info("servicebus-event-bus: failed to create receiver for", "id", group, "error", err)
		return sub
	}

	ctx, cancel := context.WithCancel(context.Background())
	sr := &sbReceiver{
		receiver: receiver,
		sub:      sub,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	b.receivers[sub.ID] = sr

	go b.receiveLoop(ctx, sr)

	return sub
}

func (b *ServiceBusEventBus) receiveLoop(ctx context.Context, sr *sbReceiver) {
	defer close(sr.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messages, err := sr.receiver.ReceiveMessages(ctx, 1, &azservicebus.ReceiveMessagesOptions{
			TimeAfterFirstMessage: 5 * time.Second,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Info("servicebus-event-bus: receive error on", "id", sr.sub.Group, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, msg := range messages {
			var ev platev.Event
			if err := json.Unmarshal(msg.Body, &ev); err != nil {
				slog.Info("servicebus-event-bus: unmarshal error", "error", err)
				_ = sr.receiver.CompleteMessage(ctx, msg, nil)
				continue
			}

			sr.sub.Handler(ev)
			_ = sr.receiver.CompleteMessage(ctx, msg, nil)
		}
	}
}

// Unsubscribe stops and removes a subscription receiver.
func (b *ServiceBusEventBus) Unsubscribe(sub *platev.Subscription) {
	b.mu.Lock()
	sr, ok := b.receivers[sub.ID]
	if ok {
		delete(b.receivers, sub.ID)
	}
	b.mu.Unlock()

	if ok {
		sr.cancel()
		<-sr.done
		_ = sr.receiver.Close(context.Background())
	}
}

// Close shuts down all receivers and the sender.
func (b *ServiceBusEventBus) Close() {
	b.mu.Lock()
	b.closed = true
	receivers := make([]*sbReceiver, 0, len(b.receivers))
	for _, sr := range b.receivers {
		receivers = append(receivers, sr)
	}
	b.receivers = make(map[string]*sbReceiver)
	b.mu.Unlock()

	for _, sr := range receivers {
		sr.cancel()
		<-sr.done
		_ = sr.receiver.Close(context.Background())
	}
	_ = b.sender.Close(context.Background())
	_ = b.client.Close(context.Background())
}
