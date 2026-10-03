package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	sender, err := s.userSender(ctx, userID, proj)
	if err != nil {
		return nil, nil, err
	}
	sc := s.newStreamChange(ctx, nil, proj, stream, proj.WorkspaceID, s.workspaceSlug(ctx, "", proj.WorkspaceID), sender)
	return sc.service(), func(ctx context.Context, res *change.Result) { sc.landed(ctx, res) }, nil
}

// userSender is a user as a change set's sender off a request: the
// permissions the user holds on the project, resolved by the project access
// middleware's resolver as a request to the project's workspace resolves them.
//
// A membership or workspace read that fails is an error, as the workspace
// middleware answers it with 503: the plan decides whether a custodian's
// authority stands, so resolving without it would keep authority a lapsed
// plan suspends.
func (s *Server) userSender(ctx context.Context, userID string, proj *store.Project) (changeSender, error) {
	sender := changeSender{userID: userID, name: userID}
	if s.AuthStore == nil {
		// A deployment with no auth store holds nobody to permissions.
		sender.allows = func(platauth.Permission, string) bool { return true }
		return sender, nil
	}
	if u, err := s.AuthStore.GetUser(ctx, userID); err == nil && u != nil {
		if u.Name != "" {
			sender.name = u.Name
		} else if u.Email != "" {
			sender.name = u.Email
		}
	}
	req := accessRequest{userID: userID, projectID: proj.ID, workspaceID: proj.WorkspaceID}
	m, err := s.AuthStore.GetMembership(ctx, proj.WorkspaceID, userID)
	switch {
	case err == nil && m != nil:
		req.role = m.Role
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return changeSender{}, fmt.Errorf("the workspace membership could not be read; try again: %w", err)
	}
	w, err := s.AuthStore.GetWorkspace(ctx, proj.WorkspaceID)
	if err != nil {
		return changeSender{}, fmt.Errorf("the project's workspace could not be read; try again: %w", err)
	}
	req.plan = w.Plan
	access := s.resolveProjectAccess(ctx, req)
	if access.custodyLapsed {
		s.recordMCPCustodyLapse(userID, proj.WorkspaceID, access.coordinates)
	}
	sender.allows = func(perm platauth.Permission, locale string) bool {
		if !access.permissions.Has(perm) {
			return false
		}
		return locale == "" || !perm.LanguageScoped() || len(access.languages) == 0 || slices.Contains(access.languages, locale)
	}
	return sender, nil
}
