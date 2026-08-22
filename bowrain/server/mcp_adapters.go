package server

import (
	"context"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
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

// mcpWorkspaceDefaultAdapter bridges auth.AuthStore → voicescope.WorkspaceDefault
// so the MCP scoring tools resolve the workspace-level default voice
// profile (the base rung of the ladder) from the workspace record.
type mcpWorkspaceDefaultAdapter struct {
	auth auth.AuthStore
}

func (a *mcpWorkspaceDefaultAdapter) WorkspaceVoiceProfileID(ctx context.Context, workspaceID string) (string, error) {
	if a.auth == nil || workspaceID == "" {
		return "", nil
	}
	ws, err := a.auth.GetWorkspace(ctx, workspaceID)
	if err != nil || ws == nil {
		return "", err
	}
	return ws.VoiceProfileID, nil
}

// memoryResolverAdapter bridges workspaceStores → MCPServer.MemoryResolver.
type memoryResolverAdapter struct {
	ws *workspaceStores
}

func (a *memoryResolverAdapter) GetMemory(workspaceID string) (memory.Store, error) {
	return a.ws.getMemory(workspaceID)
}

// tbResolverAdapter bridges workspaceStores → MCPServer.TermsResolver.
type tbResolverAdapter struct {
	ws *workspaceStores
}

func (a *tbResolverAdapter) GetTB(workspaceID string) (terms.Store, error) {
	return a.ws.getTerms(workspaceID)
}
