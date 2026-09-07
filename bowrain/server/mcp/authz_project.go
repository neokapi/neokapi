package mcp

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// ErrProjectNotFound is the one answer a project-scoped tool gives for a
// project the caller cannot reach, whether it is absent or owned by a
// workspace the caller does not belong to. The two cases read alike on
// purpose: a separate "forbidden" would confirm that a guessed project id
// names something real.
var ErrProjectNotFound = errors.New("project not found")

// callerID is the authenticated principal behind a request, empty when the
// request carries no bearer token. It is deliberately not extractUserID, whose
// "anonymous" placeholder is an analytics label rather than an identity.
//
// It is written over mcp.ServerRequest rather than over the tool request alone
// because resources and prompts reach the same voice profiles: CallToolRequest,
// ReadResourceRequest and GetPromptRequest are all aliases of it.
func callerID[P mcp.Params](req *mcp.ServerRequest[P]) string {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return ""
	}
	return req.Extra.TokenInfo.UserID
}

// authorizeProject resolves a client-supplied project_id and proves the
// authenticated principal belongs to the workspace that owns the project.
// Every tool taking a project_id goes through it before it touches the
// project, so the membership rule and its failure answer have one
// implementation.
//
// The returned string is the project id to act on: a client may name the
// project instead of identifying it, and the name is matched only among the
// projects the caller may see, so a name cannot cross a workspace either.
func (s *MCPServer) authorizeProject(ctx context.Context, req *mcp.CallToolRequest, projectID string) (string, error) {
	if projectID == "" {
		return "", ErrProjectNotFound
	}
	return s.authorizeProjectForUser(ctx, callerID(req), projectID)
}

// authorizeOptionalProject is authorizeProject for a tool whose project_id is
// optional. An empty id passes through and the tool runs without a project.
func (s *MCPServer) authorizeOptionalProject(ctx context.Context, req *mcp.CallToolRequest, projectID string) (string, error) {
	if projectID == "" {
		return "", nil
	}
	return s.authorizeProjectForUser(ctx, callerID(req), projectID)
}

func (s *MCPServer) authorizeProjectForUser(ctx context.Context, userID, projectID string) (string, error) {
	if s.contentStore == nil {
		// There is nothing to read the project's workspace from. Without a
		// membership checker there is no rule to apply either (a single-user or
		// no-auth deployment); with one, the claim cannot be proved and the
		// call is refused.
		if s.membership == nil {
			return projectID, nil
		}
		return "", ErrProjectNotFound
	}

	p, err := s.contentStore.GetProject(ctx, projectID)
	switch {
	case err == nil && p != nil:
		if !s.mayReachProject(ctx, userID, p) {
			return "", ErrProjectNotFound
		}
		return projectID, nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		// A genuine store failure, not "no such project". Fail closed: a
		// transient error must not open the workspace boundary.
		slog.WarnContext(ctx, "mcp: project lookup failed; denying",
			"project_id", projectID, "error", err)
		return "", ErrProjectNotFound
	}

	// The id names no project. Agents pass project names here, so match one
	// among the projects this caller may see.
	projects, err := s.visibleProjects(ctx, userID)
	if err != nil {
		slog.WarnContext(ctx, "mcp: project listing failed; denying",
			"project_id", projectID, "error", err)
		return "", ErrProjectNotFound
	}
	for _, p := range projects {
		if strings.EqualFold(p.Name, projectID) {
			return p.ID, nil
		}
	}
	return "", ErrProjectNotFound
}

// mayReachProject reports whether the principal belongs to the workspace that
// owns p. Without a membership checker every project is reachable, matching
// authorizeWorkspaceForUser: a deployment with no auth has no tenant to cross.
func (s *MCPServer) mayReachProject(ctx context.Context, userID string, p *store.Project) bool {
	if s.membership == nil {
		return true
	}
	return userID != "" && p.WorkspaceID != "" && s.membership.IsMember(ctx, p.WorkspaceID, userID)
}

// visibleProjects lists the projects the caller may see: every project when no
// membership checker is configured, otherwise those owned by a workspace the
// caller belongs to. Membership is asked once per workspace rather than once
// per project.
func (s *MCPServer) visibleProjects(ctx context.Context, userID string) ([]*store.Project, error) {
	all, err := s.contentStore.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	if s.membership == nil {
		return all, nil
	}
	member := map[string]bool{}
	visible := make([]*store.Project, 0, len(all))
	for _, p := range all {
		ok, asked := member[p.WorkspaceID]
		if !asked {
			ok = s.mayReachProject(ctx, userID, p)
			member[p.WorkspaceID] = ok
		}
		if ok {
			visible = append(visible, p)
		}
	}
	return visible, nil
}
