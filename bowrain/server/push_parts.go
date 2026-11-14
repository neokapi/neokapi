package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/termspush"
	"github.com/neokapi/neokapi/core/workspace"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// acceptPushParts checks the terminology and context ledger parts of a push
// commit before the push is accepted, and fills in the fields of m the worker
// reads beside them. The worker applies both parts in the push's transaction,
// after the content; what this request can already refuse (a missing
// permission, a governed edit on the direct path, a ledger operation outside
// the pusher's reach, terminology that moved under a proposal) refuses the
// whole push here, with nothing stored and nothing queued.
//
// refused reports that the answer has been written.
func (s *Server) acceptPushParts(c echo.Context, m *pushCommitManifest) (refused bool, err error) {
	if refused, err := s.acceptPushTerms(c, m); refused || err != nil {
		return true, err
	}
	return s.acceptPushLedger(c, m)
}

// acceptPushTerms checks the terminology part.
func (s *Server) acceptPushTerms(c echo.Context, m *pushCommitManifest) (bool, error) {
	if len(m.Terms) == 0 || string(m.Terms) == "null" {
		m.Terms = nil
		return false, nil
	}
	var edits apiclient.TermsEdits
	if err := json.Unmarshal(m.Terms, &edits); err != nil {
		return true, apiErr(c, http.StatusBadRequest, "invalid terms: "+err.Error())
	}
	if edits.Empty() {
		m.Terms = nil
		return false, nil
	}
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return true, err
	}
	if m.WorkspaceSlug == "" {
		return true, apiErr(c, http.StatusBadRequest, "terminology edits need the project to be in a workspace")
	}
	// The terms are the workspace's. The concept and change-set routes resolve
	// permission on the workspace (/:ws/concepts carries no project), so a
	// grant on this one project does not reach them through a push either.
	if !s.workspaceAllows(c, m.ProjectID, platauth.PermManageTerms) {
		s.emitAuthzDenied(c, platauth.PermManageTerms, "workspace_permission")
		return true, deny(c, "terminology edits need manage_terms in the workspace")
	}
	// The worker applies the terms on the push's transaction over the content
	// store's PostgreSQL pool. A server whose content store cannot do that
	// refuses here rather than accept a push the worker would then fail.
	if !appliesPushes(s.ContentStore) {
		return true, c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error: "this server cannot apply terminology edits in a push's transaction",
		})
	}
	if err := termspush.Validate(&edits); err != nil {
		if errors.Is(err, termspush.ErrGoverned) {
			return true, conceptGovernedConflict(c, err.Error())
		}
		return true, apiErr(c, http.StatusBadRequest, "invalid terms: "+err.Error())
	}
	if edits.Proposed() > 0 && s.KnowledgeStore == nil {
		return true, c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	// The proposal is a diff against the terminology the project last
	// observed. Checked here so a waiting `kapi push` is told at once, and
	// again in the worker, where the write lands.
	if s.wsStores != nil {
		if tb, terr := s.wsStores.getTerms(c.Request().Context(), m.WorkspaceSlug); terr == nil && tb != nil {
			if err := termspush.AssertRef(c.Request().Context(), tb, &edits); err != nil {
				if resp, ok := governanceConflict(c, err); ok {
					return true, resp
				}
				return true, serverErr(c, err)
			}
		}
	}
	m.TermsAuthor = authorIdentity(c)
	m.TermsOnBehalfOf = onBehalfOf(c)
	return false, nil
}

// acceptPushLedger checks the context ledger part: every name is in the
// context layout, every blob is named by the address of its bytes, and every
// operation passes the checks a push of context objects passes
// (acceptContextBatch). The binding of an unbound project is left to the
// worker's transaction.
func (s *Server) acceptPushLedger(c echo.Context, m *pushCommitManifest) (bool, error) {
	if len(m.ContextObjects) == 0 {
		return false, nil
	}
	if perms, _ := c.Get("project_permissions").(platauth.Permission); perms&contextObjectWritePermission == 0 {
		if err := s.requirePermission(c, contextObjectWritePermission); err != nil {
			return true, err
		}
	}
	batch, refused, err := contextBatchOf(c, m.ContextObjects)
	if refused || err != nil {
		return true, err
	}
	cos, ok := s.contextObjects()
	if !ok {
		return true, contextObjectsUnavailable(c)
	}
	accepted, refused, err := s.acceptContextBatch(c, cos, batch, false)
	if refused || err != nil {
		return true, err
	}
	m.ContextKey = string(accepted.bindKey)
	m.ContextOperations = len(accepted.fresh)
	m.ContextEvent = pushedContextData(c, accepted)
	return false, nil
}

// contextBatchOf reads the objects of a push into a batch, refusing a name
// outside the context layout or a blob whose name is not the address of its
// bytes. refused reports that the answer has been written.
func contextBatchOf(c echo.Context, objects []apiclient.ContextObjectWire) ([]platstore.ContextObject, bool, error) {
	batch := make([]platstore.ContextObject, 0, len(objects))
	for _, obj := range objects {
		if !workspace.ValidObjectName(obj.Name) {
			return nil, true, c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "not a name in the context layout", Details: obj.Name, Code: contextObjectInvalidCode,
			})
		}
		if strings.HasPrefix(obj.Name, workspace.RemoteBlobDir) &&
			workspace.BlobObjectName(workspace.BlobAddress(obj.Data)) != obj.Name {
			return nil, true, c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "the blob's name is not the address of its bytes", Details: obj.Name, Code: contextObjectInvalidCode,
			})
		}
		batch = append(batch, platstore.ContextObject{Name: obj.Name, Data: obj.Data})
	}
	return batch, false, nil
}

// workspaceAllows reports whether the caller holds perm on the workspace the
// project belongs to, resolved as a workspace route resolves it: from the
// workspace role, with workspace denies applied. An API token's scopes and a
// session's grants narrow the request's own permissions, which the caller
// checks beside this.
func (s *Server) workspaceAllows(c echo.Context, projectID string, perm platauth.Permission) bool {
	if s.AuthStore == nil || s.ContentStore == nil {
		return false
	}
	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return false
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil || proj == nil || proj.WorkspaceID == "" {
		return false
	}
	m, err := s.AuthStore.GetMembership(ctx, proj.WorkspaceID, userID)
	if err != nil || m == nil {
		return false
	}
	access := s.resolveProjectAccess(ctx, accessRequest{userID: userID, workspaceID: proj.WorkspaceID, role: m.Role})
	return access.permissions.Has(perm)
}

// appliesPushes reports whether the content store, through any wrappers,
// applies a push on one transaction.
func appliesPushes(cs platstore.ContentStore) bool {
	for cs != nil {
		if _, ok := cs.(platstore.PushApplyStore); ok {
			return true
		}
		inner, ok := cs.(interface{ Unwrap() platstore.ContentStore })
		if !ok {
			return false
		}
		cs = inner.Unwrap()
	}
	return false
}
