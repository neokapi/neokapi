package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
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
func (s *Server) HandleWidenConcept(c *echo.Context) error {
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
	// Widening asks for an outcome, not for a value the person read: it is
	// derived again from whatever the concept holds when it is written, so
	// an edit saved in between stays.
	found := true
	_, widened, err := sqlterms.Rederive(ctx, tb, cid, "", func(concept terms.Concept, exists bool) (terms.Concept, bool, error) {
		found = exists
		if !exists || !termscope.Widen(&concept) {
			return concept, false, nil
		}
		concept.UpdatedAt = time.Now().UTC()
		return concept, true, nil
	})
	if err != nil {
		return refuseConceptWrite(c, err)
	}
	if !found {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("concept %q not found", cid)})
	}
	if widened {
		// The event re-checks the targets of every project the concept now
		// governs (review_recheck.go).
		s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
			conceptEvent(knowledge.EventConceptUpdated, wsID, cid, actor),
		})
	}
	return c.JSON(http.StatusOK, conceptScopeResponse{ConceptID: cid, Projects: []string{}})
}
