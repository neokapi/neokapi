package server

import (
	"context"
	"errors"
	"fmt"
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
//
// It is also the expensive half: the terms component walks a workspace's whole
// vocabulary, while the other three are indexed lookups on one project. So this
// belongs on the route that exists to answer for the ref, and not on the ones
// that carry it as a convenience — see streamRefSource.
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

// streamRefSource is the ref a content route can afford: the three components
// that come from this project's own rows.
//
// The terms component is deliberately absent, and the client's cache is what
// makes that correct — an empty component is recorded as "this answer said
// nothing", not as "there is no terminology", so a pull leaves the value a
// terminology pull observed exactly where it was. Terminology freshness is the
// terminology path's to refresh; making every page of every block pull list a
// workspace vocabulary would buy nothing and cost that.
func (s *Server) streamRefSource() bowsync.RefSource {
	return bowsync.RefSource{Content: s.ContentStore}
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
	for _, component := range components {
		asserted := expected.Identity(component)
		if asserted == "" {
			continue
		}
		current, err := s.currentComponent(ctx, projectID, stream, component)
		if err != nil {
			return err
		}
		if err := ref.Assert(component, asserted, current); err != nil {
			return err
		}
	}
	return nil
}

// currentComponent reads one component in force. Reading them one at a time is
// what keeps a write paying only for the governance it actually asserts.
//
// A component with no reader is an error rather than an empty string: an empty
// string passes every assertion, so answering that way would silently accept
// exactly the write the caller asked to have checked.
func (s *Server) currentComponent(ctx context.Context, projectID, stream string, component ref.Component) (string, error) {
	switch component {
	case ref.ComponentContext:
		return bowsync.ContextComponentOf(ctx, s.ContentStore, projectID, stream)
	case ref.ComponentDecisions:
		return bowsync.DecisionsComponentOf(ctx, s.ContentStore, projectID, stream)
	case ref.ComponentTerms:
		return bowsync.TermsComponentOf(ctx, s.refSource(ctx, projectID).Terms)
	default:
		return "", fmt.Errorf("sync: no reader for the %s component", component)
	}
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
