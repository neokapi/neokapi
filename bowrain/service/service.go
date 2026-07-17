// Package service provides the shared business logic layer used by
// both the REST/gRPC server and the CLI.
package service

import (
	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/registry"
)

// EventTracker captures product analytics events. It is satisfied by
// *analytics.PostHogClient. Captures are fire-and-forget and must never block
// or fail a user operation; a nil tracker disables capture.
type EventTracker interface {
	CaptureEvent(distinctID, event string, properties map[string]any)
}

// track fires an analytics event if a tracker is configured. distinctID falls
// back to "server" when the call site has no actor identity.
func track(t EventTracker, distinctID, event string, props map[string]any) {
	if t == nil {
		return
	}
	if distinctID == "" {
		distinctID = "server"
	}
	t.CaptureEvent(distinctID, event, props)
}

// Services aggregates all service dependencies.
type Services struct {
	Project   *ProjectService
	Connector *ConnectorService
	Flow      *FlowService
	Auth      *AuthService
}

// NewServices creates the full service layer.
func NewServices(
	contentStore store.ContentStore,
	connectorReg *connector.Registry,
	formatReg *registry.FormatRegistry,
	toolReg *registry.ToolRegistry,
) *Services {
	ps := NewProjectService(contentStore)
	cs := NewConnectorService(contentStore, connectorReg)
	fs := NewFlowService(contentStore, formatReg, toolReg)

	return &Services{
		Project:   ps,
		Connector: cs,
		Flow:      fs,
	}
}

// SetEventTracker wires a product-analytics tracker into every service that
// captures domain events (epic 018 workstream D). Safe to call with nil and
// safe to skip entirely — capture is disabled either way.
func (s *Services) SetEventTracker(t EventTracker) {
	if s == nil {
		return
	}
	if s.Connector != nil {
		s.Connector.tracker = t
	}
	if s.Flow != nil {
		s.Flow.tracker = t
	}
	if s.Auth != nil {
		s.Auth.tracker = t
	}
}
