package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/apierror"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/core/ref"
)

// The freshness ref's server surface: publishing the ref a stream stands at,
// and the compare-and-swap that governance writes assert against it.
//
// The ref is DERIVED on every read rather than stored (bowrain/sync/ref.go), so
// publishing it is one read of the state it describes and no bookkeeping can
// fall out of step with the rows. That is also what makes the assertion sound:
// the value a write is checked against is computed from the same rows the write
// is about to change.

// HandleSyncRef publishes the server's current freshness ref for one stream.
// GET /sync/:ref/ref
//
// A client with no cached ref — a fresh clone, a deleted cache — reaches this
// once and knows everything the cache would have told it. That is the whole cost
// of the cache being disposable.
func (s *Server) HandleSyncRef(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "content store not configured")
	}

	current, err := bowsync.CurrentRef(c.Request().Context(),
		s.refSource(c.Request().Context(), c.Param("id")), c.Param("id"), refParam(c))
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, current)
}

// refSource assembles what a ref is computed from: the content store always,
// and the workspace terminology when the project belongs to a workspace whose
// terms store this server can open.
//
// Terminology is best-effort by design. An unclaimed project has no workspace,
// and a workspace whose terms store cannot be opened is a fault to fix, not a
// reason to withhold the three components that are perfectly readable — an
// empty terms component compares as unknown, so the client simply learns
// nothing about terminology this time.
func (s *Server) refSource(ctx context.Context, projectID string) bowsync.RefSource {
	src := bowsync.RefSource{Content: s.ContentStore}
	if s.wsStores == nil {
		return src
	}
	slug := s.workspaceSlugForProject(ctx, projectID)
	if slug == "" {
		return src
	}
	tb, err := s.wsStores.getTerms(slug)
	if err != nil || tb == nil {
		return src
	}
	src.Terms = tb
	return src
}

// workspaceSlugForProject resolves the workspace a project belongs to. The slug
// is read from the path when the route carries one and from the project row
// otherwise, because the flat sync routes an unclaimed project uses have no
// workspace segment at all.
func (s *Server) workspaceSlugForProject(ctx context.Context, projectID string) string {
	if s.ContentStore == nil {
		return ""
	}
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil || proj == nil || proj.WorkspaceID == "" || s.AuthStore == nil {
		return ""
	}
	ws, err := s.AuthStore.GetWorkspace(ctx, proj.WorkspaceID)
	if err != nil || ws == nil {
		return ""
	}
	return ws.Slug
}

// assertGovernance runs the compare-and-swap for one write.
//
// components names the components this write actually changes, and NOTHING
// else is asserted. That restriction is the composite ref's central promise: a
// push carrying blocks moves the position and no governance, so it must not be
// refused because a terminology change-set landed while it was uploading. The
// caller states what it writes; this function refuses to look further.
//
// An assertion the client did not make (an empty component) always passes,
// which is what keeps the check additive for clients that predate it.
func (s *Server) assertGovernance(ctx context.Context, projectID, stream string, expected ref.Ref, components ...ref.Component) error {
	stated := false
	for _, component := range components {
		if expected.Identity(component) != "" {
			stated = true
		}
	}
	if !stated {
		return nil
	}

	current, err := bowsync.CurrentRef(ctx, s.refSource(ctx, projectID), projectID, stream)
	if err != nil {
		return err
	}
	for _, component := range components {
		if err := ref.Assert(component, expected.Identity(component), current.Identity(component)); err != nil {
			return err
		}
	}
	return nil
}

// assertTermsRef runs the terms component's compare-and-swap for a governance
// write against a workspace's terminology, reading the asserted value from the
// request's `expected_terms_ref` query parameter.
//
// A query parameter rather than a body field because the routes this guards
// take no body, or take one whose shape is already the wire contract of a
// different concern. An absent parameter asserts nothing, which is how a client
// that predates the ref keeps working unchanged.
func (s *Server) assertTermsRef(c echo.Context, wsSlug string) error {
	expected := c.QueryParam("expected_terms_ref")
	if expected == "" || s.wsStores == nil || wsSlug == "" {
		return nil
	}
	tb, err := s.wsStores.getTerms(wsSlug)
	if err != nil || tb == nil {
		// A terminology store this server cannot open is a fault to fix, not a
		// conflict to report: refusing the write would turn an operational
		// problem into a governance one.
		return nil
	}
	current, err := bowsync.TermsComponentOf(c.Request().Context(), tb)
	if err != nil {
		return err
	}
	return ref.Assert(ref.ComponentTerms, expected, current)
}

// governanceConflict renders a compare-and-swap failure as the 409 a client
// turns into "governance moved — pull first". Any other error is ours.
func governanceConflict(c echo.Context, err error) (error, bool) {
	var conflict *ref.Conflict
	if !errors.As(err, &conflict) {
		return nil, false
	}
	return apiErr(c, http.StatusConflict, apierror.CodeGovernanceMoved, map[string]any{
		"component": string(conflict.Component),
		"expected":  conflict.Expected,
		"actual":    conflict.Actual,
	}), true
}
