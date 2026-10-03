package server

import (
	"context"
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
// permissions the user holds on the project, resolved as the project access
// middleware resolves them (project membership, else the workspace role and
// its override, less the deny rules).
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
	var role platauth.Role
	if m, err := s.AuthStore.GetMembership(ctx, proj.WorkspaceID, userID); err == nil && m != nil {
		role = m.Role
	}
	resolved, err := s.AuthStore.ResolveProjectPermissions(ctx, proj.ID, userID)
	if err != nil || resolved == nil {
		resolved = &platauth.ResolvedPermission{}
		if role != "" {
			resolved = platauth.DefaultPermissionsForRole(role)
			if perms, ok, oerr := s.AuthStore.GetWorkspaceRoleOverride(ctx, proj.WorkspaceID, role); oerr == nil && ok {
				resolved = &platauth.ResolvedPermission{Permissions: perms}
			}
		}
	}
	perms := resolved.Permissions
	if denied, derr := s.AuthStore.ResolveDenies(ctx, proj.WorkspaceID, proj.ID, userID, role); derr == nil {
		perms &^= denied
	}
	languages := resolved.Languages
	sender.allows = func(perm platauth.Permission, locale string) bool {
		if !perms.Has(perm) {
			return false
		}
		return locale == "" || !perm.LanguageScoped() || len(languages) == 0 || slices.Contains(languages, locale)
	}
	return sender
}
