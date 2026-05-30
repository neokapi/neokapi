package server

import (
	"github.com/labstack/echo/v4"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// auditEvent describes an auditable action to record on the event bus. It is the
// handler-side counterpart to the events the EventEmittingStore emits for
// content mutations: it captures security and governance actions (membership,
// roles, invites, tokens, auth, authorization decisions) that do not flow
// through the ContentStore.
type auditEvent struct {
	Type         platev.EventType
	WorkspaceID  string            // defaults to the request's workspace_id
	ProjectID    string            // optional, for project-scoped actions
	ResourceType string            // e.g. "member", "role_template", "invite", "token"
	ResourceID   string            // ID of the affected resource
	Effect       string            // "allow" | "deny" (for authz decisions)
	Data         map[string]string // event-specific detail
	Before       map[string]string // prior state (for change diffs)
	After        map[string]string // new state (for change diffs)
}

// emitAudit publishes a security/governance audit event attributed to the
// authenticated caller, enriched with request metadata. It is best-effort: a
// nil bus (e.g. in unit tests without a bus) is a no-op, and the event is
// fire-and-forget so it never blocks or fails the request.
func (s *Server) emitAudit(c echo.Context, ev auditEvent) {
	if s.EventBus == nil {
		return
	}

	actor, _ := c.Get("user_id").(string)
	name, _ := c.Get("name").(string)

	data := ev.Data
	if data == nil {
		data = map[string]string{}
	}
	if name != "" {
		if _, ok := data["actor_name"]; !ok {
			data["actor_name"] = name
		}
	}

	wsID := ev.WorkspaceID
	if wsID == "" {
		wsID, _ = c.Get("workspace_id").(string)
	}

	meta := requestMeta(c)

	s.EventBus.Publish(platev.Event{
		Type:         ev.Type,
		Source:       "server",
		ProjectID:    ev.ProjectID,
		WorkspaceID:  wsID,
		Actor:        actor,
		Data:         data,
		ResourceType: ev.ResourceType,
		ResourceID:   ev.ResourceID,
		Effect:       ev.Effect,
		Before:       ev.Before,
		After:        ev.After,
		RequestID:    meta.RequestID,
		IP:           meta.IP,
		UserAgent:    meta.UserAgent,
	})
}

// emitAuthEvent records an identity event (login/logout/failed) for an explicit
// user. Unlike emitAudit it does not read the actor from the request context,
// because identity events fire before (login) or around (logout) the auth
// middleware establishes the context actor.
func (s *Server) emitAuthEvent(c echo.Context, evType platev.EventType, userID, userName, method string) {
	if s.EventBus == nil {
		return
	}
	data := map[string]string{"method": method}
	if userName != "" {
		data["actor_name"] = userName
	}
	meta := requestMeta(c)
	s.EventBus.Publish(platev.Event{
		Type:         evType,
		Source:       "server",
		Actor:        userID,
		ResourceType: "session",
		ResourceID:   userID,
		Data:         data,
		RequestID:    meta.RequestID,
		IP:           meta.IP,
		UserAgent:    meta.UserAgent,
	})
}
