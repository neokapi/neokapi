package server

import (
	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
)

// eventTrackerAdapter bridges billing.PostHogClient → mcpserver.EventTracker.
type eventTrackerAdapter struct {
	client *billing.PostHogClient
}

func (a *eventTrackerAdapter) TrackEvent(userID, event string, properties map[string]any) {
	a.client.CaptureEvent(userID, event, properties)
}

// tmResolverAdapter bridges workspaceStores → MCPServer.TMResolver.
type tmResolverAdapter struct {
	ws *workspaceStores
}

func (a *tmResolverAdapter) GetTM(workspaceID string) (sievepen.TMStore, error) {
	return a.ws.getTM(workspaceID)
}

// tbResolverAdapter bridges workspaceStores → MCPServer.TBResolver.
type tbResolverAdapter struct {
	ws *workspaceStores
}

func (a *tbResolverAdapter) GetTB(workspaceID string) (termbase.TBStore, error) {
	return a.ws.getTB(workspaceID)
}
