package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeMembership struct{ members map[string]bool }

func (f *fakeMembership) IsMember(_ context.Context, ws, uid string) bool {
	return f.members[ws+"/"+uid]
}

// TestAuthorizeWorkspaceForUser verifies workspace-scoped MCP tools validate a
// client-supplied workspace_id against the authenticated principal's membership
// rather than trusting it (no spoofable-field tenant bypass).
func TestAuthorizeWorkspaceForUser(t *testing.T) {
	ctx := context.Background()

	// No membership checker configured (single-user / no-auth): always allowed.
	open := &MCPServer{}
	require.NoError(t, open.authorizeWorkspaceForUser(ctx, "ws-a", "user-a"))
	require.NoError(t, open.authorizeWorkspaceForUser(ctx, "", ""))

	// With a checker, only members of the named workspace pass.
	s := &MCPServer{membership: &fakeMembership{members: map[string]bool{"ws-a/user-a": true}}}
	require.NoError(t, s.authorizeWorkspaceForUser(ctx, "ws-a", "user-a"))

	// A spoofed workspace_id the caller does not belong to is rejected.
	require.Error(t, s.authorizeWorkspaceForUser(ctx, "ws-b", "user-a"))
	// An unknown principal is rejected.
	require.Error(t, s.authorizeWorkspaceForUser(ctx, "ws-a", "user-z"))
	// Missing identity or workspace fails closed.
	require.Error(t, s.authorizeWorkspaceForUser(ctx, "ws-a", ""))
	require.Error(t, s.authorizeWorkspaceForUser(ctx, "", "user-a"))
}
