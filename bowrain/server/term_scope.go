package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/termscope"
	"github.com/neokapi/neokapi/terms"
)

// getProjectTerms returns the workspace's terms as one project of it reads
// them: the concepts of the whole workspace and the ones limited to the
// project (termscope.StoreView). An empty project id returns the whole
// store.
func (ws *workspaceStores) getProjectTerms(ctx context.Context, wsSlug, projectID string) (terms.Store, error) {
	tb, err := ws.getTerms(ctx, wsSlug)
	if err != nil || tb == nil {
		return tb, err
	}
	return termscope.StoreView(tb, projectID), nil
}

// projectTermsByID is getProjectTerms for a workspace named by its id
// (workspaceTermsByID).
func (s *Server) projectTermsByID(ctx context.Context, wsID, projectID string) (terms.Store, error) {
	tb, err := s.workspaceTermsByID(ctx, wsID)
	if err != nil || tb == nil {
		return tb, err
	}
	return termscope.StoreView(tb, projectID), nil
}

// conceptScopeResponse is the projects a concept applies in after a change
// to its scope. Projects is empty for a concept of the whole workspace.
type conceptScopeResponse struct {
	ConceptID string   `json:"concept_id"`
	Projects  []string `json:"projects"`
}

// HandleWidenConcept makes a concept apply in every project of the
// workspace (POST /:ws/concepts/:cid/widen). A term kept in a project
// applies in that project only; widening it is how a person who may manage
// terms makes it govern the others too. A concept of the whole workspace
// already is answered as it stands.
func (s *Server) HandleWidenConcept(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	ctx := c.Request().Context()
	cid := c.Param("cid")
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)

	tb, err := s.wsStores.getTerms(ctx, c.Param("ws"))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, err)
	}
	if err := s.assertTermsRef(c, c.Param("ws")); err != nil {
		if resp, ok := governanceConflict(c, err); ok {
			return resp
		}
		return serverErr(c, err)
	}
	concept, ok, err := tb.GetConcept(ctx, cid)
	if err != nil {
		return serverErr(c, err)
	}
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("concept %q not found", cid)})
	}
	if termscope.Widen(&concept) {
		concept.UpdatedAt = time.Now().UTC()
		if err := tb.AddConcept(ctx, concept); err != nil {
			return serverErr(c, err)
		}
		// The event re-checks the targets of every project the concept now
		// governs (review_recheck.go).
		s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
			conceptEvent(knowledge.EventConceptUpdated, wsID, cid, actor),
		})
	}
	return c.JSON(http.StatusOK, conceptScopeResponse{ConceptID: cid, Projects: []string{}})
}
