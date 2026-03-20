// Package event provides an event bus for publishing and subscribing to
// events in the neokapi platform, plus automation rules and quality gates.
package event

import (
	"time"
)

// EventType classifies events emitted by the system.
type EventType string

const (
	// Content events
	EventBlockCreated EventType = "block.created"
	EventBlockUpdated EventType = "block.updated"
	EventBlockDeleted EventType = "block.deleted"

	// Project events
	EventProjectCreated EventType = "project.created"
	EventProjectUpdated EventType = "project.updated"
	EventProjectDeleted EventType = "project.deleted"

	// Version events
	EventVersionCreated EventType = "version.created"

	// Connector events
	EventPullCompleted EventType = "connector.pull.completed"
	EventPushCompleted EventType = "connector.push.completed"
	EventSyncCompleted EventType = "connector.sync.completed"

	// Flow events
	EventFlowStarted   EventType = "flow.started"
	EventFlowCompleted EventType = "flow.completed"
	EventFlowFailed    EventType = "flow.failed"

	// Extraction events
	EventExtractionCompleted EventType = "extraction.completed"

	// Quality events
	EventQualityGatePass EventType = "quality.gate.pass"
	EventQualityGateFail EventType = "quality.gate.fail"

	// Stream events
	EventStreamCreated EventType = "stream.created"
	EventStreamMerged  EventType = "stream.merged"
	EventStreamDeleted EventType = "stream.deleted"

	// Collection events
	EventCollectionCreated EventType = "collection.created"
	EventCollectionUpdated EventType = "collection.updated"
	EventCollectionDeleted EventType = "collection.deleted"

	// Item events
	EventItemCreated EventType = "item.created"
	EventItemDeleted EventType = "item.deleted"

	// Brand voice events
	EventBrandVoiceCheckStarted   EventType = "brand.voice.check.started"
	EventBrandVoiceCheckCompleted EventType = "brand.voice.check.completed"
	EventBrandVoiceGateFailed     EventType = "brand.voice.gate.failed"
	EventBrandVoiceGatePassed     EventType = "brand.voice.gate.passed"
	EventBrandVoiceDrift          EventType = "brand.voice.drift"
	EventBrandVoiceCorrected      EventType = "brand.voice.corrected"
	EventBrandProfileUpdated      EventType = "brand.profile.updated"

	// Agent events (AD-028)
	EventAgentConversationCreated EventType = "agent.conversation.created"
	EventAgentMessageSent         EventType = "agent.message.sent"
	EventAgentToolExecuted        EventType = "agent.tool.executed"
	EventAgentToolApproved        EventType = "agent.tool.approved"
	EventAgentToolDenied          EventType = "agent.tool.denied"
	EventAgentCodeExecuted        EventType = "agent.code.executed"
)

// Event is a typed message emitted by the system.
type Event struct {
	ID          string            `json:"id"`
	Type        EventType         `json:"type"`
	Source      string            `json:"source"` // Component that emitted the event
	ProjectID   string            `json:"project_id"`
	Actor       string            `json:"actor,omitempty"` // User or system that triggered the event
	Data        map[string]string `json:"data"`            // Event-specific key-value data
	CausationID string            `json:"causation_id"`    // For tracing automation chains
	Timestamp   time.Time         `json:"timestamp"`
}

// EventHandler processes an event.
type EventHandler func(Event)

// Subscription represents an active event subscription.
type Subscription struct {
	ID        string
	EventType EventType // Empty = all events
	Handler   EventHandler
}

// EventBus is the interface for publishing and subscribing to events.
type EventBus interface {
	Publish(event Event)
	Subscribe(eventType EventType, handler EventHandler) *Subscription
	SubscribeAll(handler EventHandler) *Subscription
	Unsubscribe(sub *Subscription)
	Close()
}
