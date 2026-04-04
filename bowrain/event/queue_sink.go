package event

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// QueueMessage is the JSON payload published to agentic event queues.
type QueueMessage struct {
	EventType   string            `json:"event_type"`
	WorkspaceID string            `json:"workspace_id,omitempty"`
	ProjectID   string            `json:"project_id,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	EventID     string            `json:"event_id"`
	CausationID string            `json:"causation_id,omitempty"`
	Data        map[string]string `json:"data,omitempty"`
}

// QueuePublisher is an abstraction for publishing messages to named queues.
// Both Redis pub/sub and Service Bus implement this interface.
type QueuePublisher interface {
	// PublishMessage sends a JSON-encoded message to the named queue/channel.
	PublishMessage(ctx context.Context, queue string, data []byte) error
}

// EventRoute maps a platform event type to a queue name.
// QueueFunc allows dynamic queue naming (e.g. locale-specific queues).
type EventRoute struct {
	EventType platev.EventType
	QueueFunc func(platev.Event) string
}

// QueueSinkConfig configures the event-to-queue adapter.
type QueueSinkConfig struct {
	// Enabled controls whether the sink is active.
	Enabled bool

	// Routes defines which events map to which queues.
	// If nil, DefaultRoutes() is used.
	Routes []EventRoute

	// ChannelPrefix is prepended to queue names (e.g. "agentic:" for Redis).
	ChannelPrefix string
}

// QueueSink subscribes to the ChannelEventBus and forwards selected events
// to external queues (Redis pub/sub or Azure Service Bus) for agentic
// testing handoffs.
type QueueSink struct {
	publisher QueuePublisher
	config    QueueSinkConfig
	routes    map[platev.EventType]func(platev.Event) string
	sub       *platev.Subscription
}

// Publisher returns the underlying queue publisher for reuse by other
// components (e.g., the agentic testing MCP dispatcher).
func (s *QueueSink) Publisher() QueuePublisher {
	return s.publisher
}

// DefaultRoutes returns the standard event-to-queue routing table
// for agentic testing.
func DefaultRoutes() []EventRoute {
	return []EventRoute{
		{
			EventType: platev.EventPushCompleted,
			QueueFunc: func(_ platev.Event) string { return "content-pushed" },
		},
		{
			EventType: platev.EventExtractionCompleted,
			QueueFunc: func(_ platev.Event) string { return "content-pushed" },
		},
		{
			EventType: platev.EventBlockCreated,
			QueueFunc: func(ev platev.Event) string {
				locale := ev.Data["locale"]
				if locale == "" {
					locale = ev.Data["target_locale"]
				}
				if locale != "" {
					return "tasks-created-" + strings.ToLower(locale)
				}
				return "tasks-created"
			},
		},
		{
			EventType: platev.EventBlockUpdated,
			QueueFunc: func(_ platev.Event) string { return "translation-complete" },
		},
		{
			EventType: platev.EventFlowCompleted,
			QueueFunc: func(ev platev.Event) string {
				if ev.Data["flow_type"] == "qa" || strings.Contains(ev.Data["flow_name"], "qa") {
					return "qa-passed"
				}
				return ""
			},
		},
		{
			EventType: platev.EventQualityGatePass,
			QueueFunc: func(_ platev.Event) string { return "qa-passed" },
		},
	}
}

// NewQueueSink creates a QueueSink that forwards events to the given publisher.
// It subscribes to the event bus immediately. Call Close to unsubscribe.
func NewQueueSink(bus platev.EventBus, publisher QueuePublisher, config QueueSinkConfig) *QueueSink {
	routes := config.Routes
	if routes == nil {
		routes = DefaultRoutes()
	}

	s := &QueueSink{
		publisher: publisher,
		config:    config,
		routes:    make(map[platev.EventType]func(platev.Event) string, len(routes)),
	}

	for _, r := range routes {
		s.routes[r.EventType] = r.QueueFunc
	}

	s.sub = bus.SubscribeGroup("queue-sink", s.handleEvent)
	return s
}

// Close unsubscribes the sink from the event bus.
func (s *QueueSink) Close(bus platev.EventBus) {
	if s.sub != nil {
		bus.Unsubscribe(s.sub)
	}
}

// handleEvent is the EventHandler callback registered with the bus.
func (s *QueueSink) handleEvent(ev platev.Event) {
	queueFunc, ok := s.routes[ev.Type]
	if !ok {
		return
	}

	queue := queueFunc(ev)
	if queue == "" {
		return
	}

	if s.config.ChannelPrefix != "" {
		queue = s.config.ChannelPrefix + queue
	}

	msg := QueueMessage{
		EventType:   string(ev.Type),
		WorkspaceID: ev.Data["workspace_id"],
		ProjectID:   ev.ProjectID,
		Timestamp:   ev.Timestamp,
		EventID:     ev.ID,
		CausationID: ev.CausationID,
		Data:        ev.Data,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		slog.Warn("queue sink: marshal event", "id", ev.ID, "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.publisher.PublishMessage(ctx, queue, data); err != nil {
		slog.Warn("queue sink: publish failed", "event_id", ev.ID, "queue", queue, "error", err)
	}
}

// RedisQueuePublisher implements QueuePublisher using Redis pub/sub.
type RedisQueuePublisher struct {
	publish func(ctx context.Context, channel string, message any) error
}

// NewRedisQueuePublisher creates a publisher that sends messages to Redis channels.
// The client parameter should be a *redis.Client; the function signature uses
// the minimal interface to avoid importing go-redis in the event package.
func NewRedisQueuePublisher(publishFn func(ctx context.Context, channel string, message any) error) *RedisQueuePublisher {
	return &RedisQueuePublisher{publish: publishFn}
}

func (r *RedisQueuePublisher) PublishMessage(ctx context.Context, queue string, data []byte) error {
	return r.publish(ctx, queue, data)
}

// ServiceBusQueuePublisher implements QueuePublisher using Azure Service Bus.
// It creates a sender per queue on first use.
type ServiceBusQueuePublisher struct {
	send func(ctx context.Context, queue string, body []byte) error
}

// NewServiceBusQueuePublisher creates a publisher backed by Azure Service Bus.
// The sendFn parameter wraps the Service Bus client to avoid importing the
// Azure SDK in the event package.
func NewServiceBusQueuePublisher(sendFn func(ctx context.Context, queue string, body []byte) error) *ServiceBusQueuePublisher {
	return &ServiceBusQueuePublisher{send: sendFn}
}

func (sb *ServiceBusQueuePublisher) PublishMessage(ctx context.Context, queue string, data []byte) error {
	return sb.send(ctx, queue, data)
}

// MemoryQueuePublisher is a QueuePublisher for testing that records messages.
type MemoryQueuePublisher struct {
	mu       sync.Mutex
	Messages []PublishedMessage
}

// PublishedMessage records a message sent to a queue.
type PublishedMessage struct {
	Queue string
	Data  []byte
}

func (m *MemoryQueuePublisher) PublishMessage(_ context.Context, queue string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, PublishedMessage{Queue: queue, Data: data})
	return nil
}

// GetMessages returns a copy of the published messages (thread-safe).
func (m *MemoryQueuePublisher) GetMessages() []PublishedMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]PublishedMessage, len(m.Messages))
	copy(result, m.Messages)
	return result
}

// ErrorQueuePublisher always returns an error (for testing error handling).
type ErrorQueuePublisher struct {
	Err error
}

func (e *ErrorQueuePublisher) PublishMessage(_ context.Context, _ string, _ []byte) error {
	return fmt.Errorf("publish failed: %w", e.Err)
}
