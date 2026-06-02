package server

import (
	"context"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
)

// eventTrackerAdapter bridges analytics.PostHogClient → mcpserver.EventTracker.
type eventTrackerAdapter struct {
	client *analytics.PostHogClient
}

func (a *eventTrackerAdapter) TrackEvent(userID, event string, properties map[string]any) {
	a.client.CaptureEvent(userID, event, properties)
}

// mcpMembershipAdapter bridges auth.AuthStore → mcpserver.MembershipChecker so
// workspace-scoped MCP tools validate the caller's membership in the
// client-supplied workspace_id rather than trusting it.
type mcpMembershipAdapter struct {
	auth auth.AuthStore
}

func (a *mcpMembershipAdapter) IsMember(ctx context.Context, workspaceID, userID string) bool {
	if a.auth == nil || workspaceID == "" || userID == "" {
		return false
	}
	m, err := a.auth.GetMembership(ctx, workspaceID, userID)
	return err == nil && m != nil
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
