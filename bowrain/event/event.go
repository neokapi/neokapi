// Package event provides event bus implementations (in-memory bus,
// webhook delivery, automation engine, store event tracking).
// Core event types and the EventBus interface are defined in
// platform/event and re-exported here via type aliases.
package event

import platev "github.com/gokapi/gokapi/platform/event"

// Type aliases — canonical definitions live in platform/event.
type (
	EventType    = platev.EventType
	Event        = platev.Event
	EventHandler = platev.EventHandler
	Subscription = platev.Subscription
	EventBus     = platev.EventBus
)

// Re-export constants.
const (
	EventBlockCreated    = platev.EventBlockCreated
	EventBlockUpdated    = platev.EventBlockUpdated
	EventBlockDeleted    = platev.EventBlockDeleted
	EventProjectCreated  = platev.EventProjectCreated
	EventProjectUpdated  = platev.EventProjectUpdated
	EventProjectDeleted  = platev.EventProjectDeleted
	EventVersionCreated  = platev.EventVersionCreated
	EventPullCompleted   = platev.EventPullCompleted
	EventPushCompleted   = platev.EventPushCompleted
	EventSyncCompleted   = platev.EventSyncCompleted
	EventFlowStarted     = platev.EventFlowStarted
	EventFlowCompleted   = platev.EventFlowCompleted
	EventFlowFailed      = platev.EventFlowFailed
	EventQualityGatePass = platev.EventQualityGatePass
	EventQualityGateFail = platev.EventQualityGateFail
)
