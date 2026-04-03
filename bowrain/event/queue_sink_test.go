package event

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueueSinkRoutesContentPush(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
		Data:      map[string]string{"workspace_id": "ws-1"},
	})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)
	assert.Equal(t, "content-pushed", pub.GetMessages()[0].Queue)

	var msg QueueMessage
	require.NoError(t, json.Unmarshal(pub.GetMessages()[0].Data, &msg))
	assert.Equal(t, "connector.push.completed", msg.EventType)
	assert.Equal(t, "proj-1", msg.ProjectID)
	assert.Equal(t, "ws-1", msg.WorkspaceID)
}

func TestQueueSinkRoutesExtractionToContentPushed(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:      platev.EventExtractionCompleted,
		ProjectID: "proj-2",
	})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)
	assert.Equal(t, "content-pushed", pub.GetMessages()[0].Queue)
}

func TestQueueSinkRoutesBlockCreatedWithLocale(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:      platev.EventBlockCreated,
		ProjectID: "proj-1",
		Data:      map[string]string{"locale": "fr-FR"},
	})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)
	assert.Equal(t, "tasks-created-fr-fr", pub.GetMessages()[0].Queue)
}

func TestQueueSinkRoutesBlockCreatedWithTargetLocale(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:      platev.EventBlockCreated,
		ProjectID: "proj-1",
		Data:      map[string]string{"target_locale": "de-DE"},
	})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)
	assert.Equal(t, "tasks-created-de-de", pub.GetMessages()[0].Queue)
}

func TestQueueSinkRoutesBlockCreatedNoLocale(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:      platev.EventBlockCreated,
		ProjectID: "proj-1",
		Data:      map[string]string{},
	})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)
	assert.Equal(t, "tasks-created", pub.GetMessages()[0].Queue)
}

func TestQueueSinkRoutesBlockUpdated(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:      platev.EventBlockUpdated,
		ProjectID: "proj-1",
	})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)
	assert.Equal(t, "translation-complete", pub.GetMessages()[0].Queue)
}

func TestQueueSinkRoutesQualityGatePass(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:      platev.EventQualityGatePass,
		ProjectID: "proj-1",
	})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)
	assert.Equal(t, "qa-passed", pub.GetMessages()[0].Queue)
}

func TestQueueSinkRoutesFlowCompletedQA(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:      platev.EventFlowCompleted,
		ProjectID: "proj-1",
		Data:      map[string]string{"flow_type": "qa"},
	})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)
	assert.Equal(t, "qa-passed", pub.GetMessages()[0].Queue)
}

func TestQueueSinkIgnoresNonQAFlowCompleted(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:      platev.EventFlowCompleted,
		ProjectID: "proj-1",
		Data:      map[string]string{"flow_type": "translation"},
	})

	time.Sleep(50 * time.Millisecond)

	assert.Len(t, pub.GetMessages(), 0)
}

func TestQueueSinkIgnoresUnknownEvents(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{Type: platev.EventProjectCreated, ProjectID: "proj-1"})
	bus.Publish(platev.Event{Type: platev.EventStreamCreated, ProjectID: "proj-1"})
	bus.Publish(platev.Event{Type: platev.EventAgentMessageSent, ProjectID: "proj-1"})

	time.Sleep(50 * time.Millisecond)

	assert.Len(t, pub.GetMessages(), 0)
}

func TestQueueSinkChannelPrefix(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{
		Enabled:       true,
		ChannelPrefix: "agentic:",
	})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
	})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)
	assert.Equal(t, "agentic:content-pushed", pub.GetMessages()[0].Queue)
}

func TestQueueSinkCustomRoutes(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{
		Enabled: true,
		Routes: []EventRoute{
			{
				EventType: platev.EventProjectCreated,
				QueueFunc: func(_ platev.Event) string { return "custom-queue" },
			},
		},
	})
	defer sink.Close(bus)

	// Custom route should work.
	bus.Publish(platev.Event{Type: platev.EventProjectCreated})

	// Default route should not (only custom routes active).
	bus.Publish(platev.Event{Type: platev.EventPushCompleted})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)
	assert.Equal(t, "custom-queue", pub.GetMessages()[0].Queue)
}

func TestQueueSinkErrorHandling(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &ErrorQueuePublisher{Err: errors.New("connection refused")}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	// Should not panic; error is logged.
	bus.Publish(platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
	})

	time.Sleep(50 * time.Millisecond)
}

func TestQueueSinkConcurrentPublish(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &syncMemoryPublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	const n = 50
	for i := 0; i < n; i++ {
		bus.Publish(platev.Event{
			Type:      platev.EventPushCompleted,
			ProjectID: "proj-1",
		})
	}

	time.Sleep(200 * time.Millisecond)

	pub.mu.Lock()
	assert.Equal(t, n, len(pub.messages))
	pub.mu.Unlock()
}

func TestQueueSinkMessagePayload(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})
	defer sink.Close(bus)

	bus.Publish(platev.Event{
		Type:        platev.EventQualityGatePass,
		ProjectID:   "proj-42",
		CausationID: "cause-1",
		Data: map[string]string{
			"workspace_id": "ws-7",
			"score":        "98",
		},
	})

	time.Sleep(50 * time.Millisecond)

	require.Len(t, pub.GetMessages(), 1)

	var msg QueueMessage
	require.NoError(t, json.Unmarshal(pub.GetMessages()[0].Data, &msg))
	assert.Equal(t, "quality.gate.pass", msg.EventType)
	assert.Equal(t, "proj-42", msg.ProjectID)
	assert.Equal(t, "ws-7", msg.WorkspaceID)
	assert.Equal(t, "cause-1", msg.CausationID)
	assert.Equal(t, "98", msg.Data["score"])
	assert.False(t, msg.Timestamp.IsZero())
	assert.NotEmpty(t, msg.EventID)
}

func TestQueueSinkClose(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	pub := &MemoryQueuePublisher{}
	sink := NewQueueSink(bus, pub, QueueSinkConfig{Enabled: true})

	// Close should be safe to call.
	sink.Close(bus)

	// Events after close should not reach publisher.
	bus.Publish(platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
	})

	time.Sleep(50 * time.Millisecond)
	assert.Len(t, pub.GetMessages(), 0)
}

// syncMemoryPublisher is a thread-safe MemoryQueuePublisher for concurrent tests.
type syncMemoryPublisher struct {
	mu       sync.Mutex
	messages []PublishedMessage
}

func (s *syncMemoryPublisher) PublishMessage(_ context.Context, queue string, data []byte) error {
	s.mu.Lock()
	s.messages = append(s.messages, PublishedMessage{Queue: queue, Data: data})
	s.mu.Unlock()
	return nil
}
