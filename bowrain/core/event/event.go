// Package event provides an event bus for publishing and subscribing to
// events in the neokapi platform, plus automation rules and quality gates.
package event

import (
	"context"
	"time"
)

type actorKeyType struct{}
type actorNameKeyType struct{}

// WithActor returns a context that carries the given actor (user) ID and name.
// The EventEmittingStore uses this to attribute events to the authenticated user.
func WithActor(ctx context.Context, actorID, actorName string) context.Context {
	ctx = context.WithValue(ctx, actorKeyType{}, actorID)
	if actorName != "" {
		ctx = context.WithValue(ctx, actorNameKeyType{}, actorName)
	}
	return ctx
}

// ActorFromContext extracts the actor ID from the context, or "" if not set.
func ActorFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(actorKeyType{}).(string); ok {
		return v
	}
	return ""
}

// ActorNameFromContext extracts the actor name from the context, or "" if not set.
func ActorNameFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(actorNameKeyType{}).(string); ok {
		return v
	}
	return ""
}

type reqMetaKeyType struct{}

// RequestMeta carries audit-relevant request metadata for event attribution.
type RequestMeta struct {
	RequestID string
	IP        string
	UserAgent string
}

// WithRequestMeta returns a context carrying request metadata (IP, user-agent,
// request ID) so emitted events can record where an action came from.
func WithRequestMeta(ctx context.Context, m RequestMeta) context.Context {
	return context.WithValue(ctx, reqMetaKeyType{}, m)
}

// RequestMetaFromContext extracts request metadata from the context, or the zero
// value if not set.
func RequestMetaFromContext(ctx context.Context) RequestMeta {
	if v, ok := ctx.Value(reqMetaKeyType{}).(RequestMeta); ok {
		return v
	}
	return RequestMeta{}
}

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
	EventStreamCreated  EventType = "stream.created"
	EventStreamMerged   EventType = "stream.merged"
	EventStreamDeleted  EventType = "stream.deleted"
	EventStreamLocked   EventType = "stream.locked"
	EventStreamUnlocked EventType = "stream.unlocked"
	EventStreamTagged   EventType = "stream.tagged"

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

	// Workflow events (Bowrain AD-014)
	EventPushAutomationsCompleted EventType = "push.automations.completed"
	EventSourceReviewCompleted    EventType = "source.review.completed"

	// Agent events (Bowrain AD-016)
	EventAgentConversationCreated EventType = "agent.conversation.created"
	EventAgentMessageSent         EventType = "agent.message.sent"
	EventAgentToolExecuted        EventType = "agent.tool.executed"
	EventAgentToolApproved        EventType = "agent.tool.approved"
	EventAgentToolDenied          EventType = "agent.tool.denied"
	EventAgentCodeExecuted        EventType = "agent.code.executed"

	// Membership & access-governance events (security audit)
	EventMemberAdded         EventType = "member.added"
	EventMemberRemoved       EventType = "member.removed"
	EventMemberRoleChanged   EventType = "member.role_changed"
	EventRoleTemplateCreated EventType = "role.template.created"
	EventRoleTemplateUpdated EventType = "role.template.updated"
	EventRoleTemplateDeleted EventType = "role.template.deleted"
	EventInviteCreated       EventType = "invite.created"
	EventInviteAccepted      EventType = "invite.accepted"
	EventInviteRevoked       EventType = "invite.revoked"

	// Identity & credential events (security audit)
	EventTokenCreated        EventType = "token.created"
	EventTokenRevoked        EventType = "token.revoked"
	EventAuthLogin           EventType = "auth.login"
	EventAuthLogout          EventType = "auth.logout"
	EventAuthFailed          EventType = "auth.failed"
	EventSessionGrantCreated EventType = "session.grant.created"

	// Authorization-decision events (security audit). Emitted when a request is
	// denied by the permission layer so that access failures are visible.
	EventAuthzDenied EventType = "authz.denied"

	// Change-governance events
	EventRollbackPerformed EventType = "rollback.performed"
)

// Event is a typed message emitted by the system.
type Event struct {
	ID          string            `json:"id"`
	Type        EventType         `json:"type"`
	Source      string            `json:"source"` // Component that emitted the event
	ProjectID   string            `json:"project_id"`
	WorkspaceID string            `json:"workspace_id,omitempty"` // Set for workspace-scoped (non-project) events
	Actor       string            `json:"actor,omitempty"`        // User or system that triggered the event
	Data        map[string]string `json:"data"`                   // Event-specific key-value data
	CausationID string            `json:"causation_id"`           // For tracing automation chains / grouping a batch
	Timestamp   time.Time         `json:"timestamp"`

	// Audit enrichment (who/what/where/outcome). All optional; populated for
	// auditable mutations and security events.
	ResourceType string            `json:"resource_type,omitempty"` // e.g. "member", "role_template", "tm_entry"
	ResourceID   string            `json:"resource_id,omitempty"`   // ID of the affected resource
	Effect       string            `json:"effect,omitempty"`        // "allow" | "deny" for authorization decisions
	Before       map[string]string `json:"before,omitempty"`        // prior state (for change diffs)
	After        map[string]string `json:"after,omitempty"`         // new state (for change diffs)
	RequestID    string            `json:"request_id,omitempty"`    // correlates with request logs
	IP           string            `json:"ip,omitempty"`            // client IP
	UserAgent    string            `json:"user_agent,omitempty"`    // client user-agent
}

// EventHandler processes an event.
type EventHandler func(Event)

// Subscription represents an active event subscription.
type Subscription struct {
	ID        string
	EventType EventType // Empty = all events
	Group     string    // Consumer group name (for distributed buses; empty = broadcast)
	Handler   EventHandler
}

// EventBus is the interface for publishing and subscribing to events.
type EventBus interface {
	Publish(event Event)
	Subscribe(eventType EventType, handler EventHandler) *Subscription
	SubscribeAll(handler EventHandler) *Subscription
	// SubscribeGroup subscribes with a named consumer group. In distributed
	// buses, only one instance in the group receives each event. Falls back
	// to SubscribeAll for in-process buses.
	SubscribeGroup(group string, handler EventHandler) *Subscription
	Unsubscribe(sub *Subscription)
	Close()
}
