package server

import (
	"context"
	"log/slog"
	"slices"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
)

// mcpChangeService builds the change service an agent's change sets on one
// stream go through, for the server MCP: the policy holds the agent to what
// the user it acts for may do on the project, and to the rules every surface
// holds an agent to. landed does what a change set that landed asks of the
// rest of the server.
func (s *Server) mcpChangeService(ctx context.Context, userID, projectID, stream string) (*change.Service, func(context.Context, *change.Result), error) {
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	sender := s.userSender(ctx, userID, proj)
	sc := s.newStreamChange(ctx, nil, proj, stream, proj.WorkspaceID, s.workspaceSlug(ctx, "", proj.WorkspaceID), sender)
	return sc.service(), func(ctx context.Context, res *change.Result) { sc.landed(ctx, res) }, nil
}

// userSender is a user as a change set's sender off a request: the
// permissions the user holds on the project, resolved by the project access
// middleware's resolver as a request to the project's workspace resolves them.
func (s *Server) userSender(ctx context.Context, userID string, proj *store.Project) changeSender {
	sender := changeSender{userID: userID, name: userID}
	if s.AuthStore == nil {
		// A deployment with no auth store holds nobody to permissions.
		sender.allows = func(platauth.Permission, string) bool { return true }
		return sender
	}
	if u, err := s.AuthStore.GetUser(ctx, userID); err == nil && u != nil {
		if u.Name != "" {
			sender.name = u.Name
		} else if u.Email != "" {
			sender.name = u.Email
		}
	}
	req := accessRequest{userID: userID, projectID: proj.ID, workspaceID: proj.WorkspaceID}
	if m, err := s.AuthStore.GetMembership(ctx, proj.WorkspaceID, userID); err == nil && m != nil {
		req.role = m.Role
	}
	if w, err := s.AuthStore.GetWorkspace(ctx, proj.WorkspaceID); err == nil && w != nil {
		req.plan = w.Plan
	}
	access := s.resolveProjectAccess(ctx, req)
	if access.custodyLapsed {
		slog.InfoContext(ctx, "mcp: custodial authority suspended by the workspace's plan",
			"user", userID, "project", proj.ID, "coordinates", access.coordinates.String())
	}
	sender.allows = func(perm platauth.Permission, locale string) bool {
		if !access.permissions.Has(perm) {
			return false
		}
		return locale == "" || !perm.LanguageScoped() || len(access.languages) == 0 || slices.Contains(access.languages, locale)
	}
	return sender
}
