package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
)

// registerChangesetRoutes registers the change-set half of the brand
// knowledge-graph REST API (AD-021) on the workspace content group: the
// /:ws/changesets collection and its op/review/pilot lifecycle. A change-set is
// the governed path into the graph — ordinary edits land directly through the
// concept routes, but a governed change (banning or promoting a term, deleting a
// concept, a REPLACED_BY relation, any brand-voice rule) must travel through a
// reviewed change-set, which this surface drafts, submits, reviews, merges,
// pilots, and previews (blast radius).
//
// Permissions follow the data-model note: reads gate on workspace membership
// (view_content); drafting and editing a change-set, its ops, and its pilots
// gate on manage_terms; authoring a brand-voice op additionally requires
// manage_brand; approving, rejecting, and merging a governed change-set require
// manage_brand and enforce separation of duties (the reviewer/approver must not
// be the change-set's author). It reuses s.knowledgeEngineFor (blast radius,
// merge, pilots), s.publishKnowledgeEvents (event-bus + audit chain), and
// s.KnowledgeStore (the governance store) from the concept stage.
func (s *Server) registerChangesetRoutes(g *echo.Group) {
	// Collection + single change-set.
	g.GET("/changesets", s.HandleListChangeSets)
	g.POST("/changesets", s.HandleCreateChangeSet)
	g.GET("/changesets/:id", s.HandleGetChangeSet)
	g.PATCH("/changesets/:id", s.HandleUpdateChangeSet)

	// Ops — the ordered edits a change-set accumulates while a draft.
	g.POST("/changesets/:id/ops", s.HandleAddChangeSetOp)
	g.DELETE("/changesets/:id/ops/:seq", s.HandleRemoveChangeSetOp)

	// Lifecycle: draft → in_review → approved → merged, or abandoned.
	g.POST("/changesets/:id/submit", s.HandleSubmitChangeSet)
	g.POST("/changesets/:id/approve", s.HandleApproveChangeSet)
	g.POST("/changesets/:id/reject", s.HandleRejectChangeSet)
	g.POST("/changesets/:id/merge", s.HandleMergeChangeSet)
	g.POST("/changesets/:id/abandon", s.HandleAbandonChangeSet)

	// Blast radius (preview) and pilots (stream-scoped shadows of the draft).
	g.GET("/changesets/:id/blast-radius", s.HandleChangeSetBlastRadius)
	g.POST("/changesets/:id/pilots", s.HandleStartPilot)
	g.DELETE("/changesets/:id/pilots/:project/:stream", s.HandleStopPilot)
}

// ---------------------------------------------------------------------------
// Request/response DTOs
// ---------------------------------------------------------------------------

// CreateChangeSetRequest opens a new change-set in the draft status.
type CreateChangeSetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// UpdateChangeSetRequest patches a draft change-set's header fields. Nil
// pointers leave a field unchanged.
type UpdateChangeSetRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// AddChangeSetOpRequest appends one ordered op to a draft change-set. Op selects
// the operation; Payload is the op-specific JSON validated by knowledge.ValidateOp;
// BaseRev optionally pins the op to the concept revision it was authored against
// for stale-draft conflict detection at merge.
type AddChangeSetOpRequest struct {
	Op      knowledge.OpType `json:"op"`
	Payload json.RawMessage  `json:"payload"`
	BaseRev int64            `json:"base_rev,omitempty"`
}

// ReviewRequest carries an optional reviewer comment on approve/reject.
type ReviewRequest struct {
	Comment string `json:"comment,omitempty"`
}

// StartPilotRequest binds a change-set to a project's content stream as a pilot.
type StartPilotRequest struct {
	ProjectID string `json:"project_id"`
	Stream    string `json:"stream"`
}

// ChangeSetDetailResponse is a change-set with its ops, reviews, and pilots, plus
// whether it carries any governed op (which determines whether it needs an
// approval before merge).
type ChangeSetDetailResponse struct {
	*knowledge.ChangeSet
	Governed bool                         `json:"governed"`
	Ops      []*knowledge.ChangeSetOp     `json:"ops"`
	Reviews  []*knowledge.ChangeSetReview `json:"reviews"`
	Pilots   []*knowledge.Pilot           `json:"pilots"`
}

// ---------------------------------------------------------------------------
// Collection + single change-set
// ---------------------------------------------------------------------------

// HandleListChangeSets lists the workspace's change-sets, newest first, optionally
// filtered to a single status.
func (s *Server) HandleListChangeSets(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	status := knowledge.ChangeSetStatus(c.QueryParam("status"))
	if status != "" && !status.IsValid() {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("unknown change-set status %q", status)})
	}
	sets, err := s.KnowledgeStore.ListChangeSets(c.Request().Context(), wsID, status)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if sets == nil {
		sets = []*knowledge.ChangeSet{}
	}
	return c.JSON(http.StatusOK, sets)
}

// HandleCreateChangeSet opens a new draft change-set and announces it.
func (s *Server) HandleCreateChangeSet(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)

	var req CreateChangeSetRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if strings.TrimSpace(req.Name) == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}

	cs := &knowledge.ChangeSet{
		WorkspaceID: wsID,
		Name:        req.Name,
		Description: req.Description,
		Status:      knowledge.ChangeSetDraft,
		CreatedBy:   actor,
	}
	if err := s.KnowledgeStore.CreateChangeSet(c.Request().Context(), cs); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		changesetEvent(knowledge.EventChangeSetCreated, wsID, cs.ID, actor),
	})
	return c.JSON(http.StatusCreated, cs)
}

// HandleGetChangeSet returns a change-set with its ops, reviews, and pilots.
func (s *Server) HandleGetChangeSet(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	opPtrs, err := s.KnowledgeStore.ListOps(ctx, wsID, cs.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	reviews, err := s.KnowledgeStore.ListReviews(ctx, wsID, cs.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	pilots, err := s.KnowledgeStore.ListPilots(ctx, wsID, cs.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	governed, _ := knowledge.ChangeSetIsGoverned(changeSetOpValues(opPtrs))

	if opPtrs == nil {
		opPtrs = []*knowledge.ChangeSetOp{}
	}
	if reviews == nil {
		reviews = []*knowledge.ChangeSetReview{}
	}
	if pilots == nil {
		pilots = []*knowledge.Pilot{}
	}
	return c.JSON(http.StatusOK, ChangeSetDetailResponse{
		ChangeSet: cs,
		Governed:  governed,
		Ops:       opPtrs,
		Reviews:   reviews,
		Pilots:    pilots,
	})
}

// HandleUpdateChangeSet patches a draft change-set's name and description.
func (s *Server) HandleUpdateChangeSet(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	if cs.Status != knowledge.ChangeSetDraft {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: fmt.Sprintf("a change-set can only be edited while a draft (status %q)", cs.Status)})
	}

	var req UpdateChangeSetRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name cannot be empty"})
		}
		cs.Name = *req.Name
	}
	if req.Description != nil {
		cs.Description = *req.Description
	}
	if err := s.KnowledgeStore.UpdateChangeSet(c.Request().Context(), cs); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, cs)
}

// ---------------------------------------------------------------------------
// Ops
// ---------------------------------------------------------------------------

// HandleAddChangeSetOp appends one validated op to a draft change-set. A
// brand-voice op (voice.rule.add/remove) is a governed brand edit and so
// additionally requires manage_brand to author.
func (s *Server) HandleAddChangeSetOp(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	if cs.Status != knowledge.ChangeSetDraft {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: fmt.Sprintf("ops can only be added while the change-set is a draft (status %q)", cs.Status)})
	}

	var req AddChangeSetOpRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	op := knowledge.ChangeSetOp{
		WorkspaceID: wsID,
		ChangesetID: cs.ID,
		Op:          req.Op,
		Payload:     req.Payload,
		BaseRev:     req.BaseRev,
		CreatedBy:   actor,
	}
	if err := knowledge.ValidateOp(op); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	// Authoring a brand-voice op is a governed brand action.
	if isVoiceOpType(op.Op) {
		if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
			return err
		}
	}
	if err := s.KnowledgeStore.AppendOp(c.Request().Context(), &op); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, &op)
}

// HandleRemoveChangeSetOp removes an op from a draft change-set by its sequence.
func (s *Server) HandleRemoveChangeSetOp(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	if cs.Status != knowledge.ChangeSetDraft {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: fmt.Sprintf("ops can only be removed while the change-set is a draft (status %q)", cs.Status)})
	}
	seq, err := strconv.ParseInt(c.Param("seq"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "seq must be an integer"})
	}
	if err := s.KnowledgeStore.RemoveOp(c.Request().Context(), wsID, cs.ID, seq); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// HandleSubmitChangeSet moves a draft change-set into review. An empty change-set
// cannot be submitted.
func (s *Server) HandleSubmitChangeSet(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
	ctx := c.Request().Context()

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	if cs.Status != knowledge.ChangeSetDraft {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: fmt.Sprintf("only a draft change-set can be submitted for review (status %q)", cs.Status)})
	}
	ops, err := s.KnowledgeStore.ListOps(ctx, wsID, cs.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if len(ops) == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cannot submit an empty change-set"})
	}
	if err := s.KnowledgeStore.SetChangeSetStatus(ctx, wsID, cs.ID, knowledge.ChangeSetInReview); err != nil {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		changesetEvent(knowledge.EventChangeSetSubmitted, wsID, cs.ID, actor),
	})
	return s.refreshedChangeSet(c, wsID, cs.ID)
}

// HandleApproveChangeSet records an approval and moves an in-review change-set to
// approved. Separation of duties: the approver must not be the change-set's
// author.
func (s *Server) HandleApproveChangeSet(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
	ctx := c.Request().Context()

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	if actor != "" && actor == cs.CreatedBy {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: "separation of duties: a change-set's author cannot review their own change-set"})
	}
	if cs.Status != knowledge.ChangeSetInReview {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: fmt.Sprintf("only an in-review change-set can be approved (status %q)", cs.Status)})
	}

	var req ReviewRequest
	_ = c.Bind(&req)
	review := &knowledge.ChangeSetReview{
		WorkspaceID: wsID,
		ChangesetID: cs.ID,
		Reviewer:    actor,
		Verdict:     knowledge.VerdictApprove,
		Comment:     req.Comment,
	}
	if err := s.KnowledgeStore.AddReview(ctx, review); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if err := s.KnowledgeStore.SetChangeSetStatus(ctx, wsID, cs.ID, knowledge.ChangeSetApproved); err != nil {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		changesetEvent(knowledge.EventChangeSetApproved, wsID, cs.ID, actor),
	})
	return s.refreshedChangeSet(c, wsID, cs.ID)
}

// HandleRejectChangeSet records a rejection and reopens an in-review change-set
// as a draft. Separation of duties: the reviewer must not be the author.
func (s *Server) HandleRejectChangeSet(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
	ctx := c.Request().Context()

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	if actor != "" && actor == cs.CreatedBy {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: "separation of duties: a change-set's author cannot review their own change-set"})
	}
	if cs.Status != knowledge.ChangeSetInReview {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: fmt.Sprintf("only an in-review change-set can be rejected (status %q)", cs.Status)})
	}

	var req ReviewRequest
	_ = c.Bind(&req)
	review := &knowledge.ChangeSetReview{
		WorkspaceID: wsID,
		ChangesetID: cs.ID,
		Reviewer:    actor,
		Verdict:     knowledge.VerdictReject,
		Comment:     req.Comment,
	}
	if err := s.KnowledgeStore.AddReview(ctx, review); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if err := s.KnowledgeStore.SetChangeSetStatus(ctx, wsID, cs.ID, knowledge.ChangeSetDraft); err != nil {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		changesetEvent(knowledge.EventChangeSetRejected, wsID, cs.ID, actor),
	})
	return s.refreshedChangeSet(c, wsID, cs.ID)
}

// HandleMergeChangeSet merges a change-set into the workspace graph and brand
// profiles. An ordinary change-set (no governed op) merges directly from draft by
// anyone with manage_terms; a governed change-set requires manage_brand and an
// approval from a reviewer other than its author (the engine's separation-of-duties
// gate). A stale-draft conflict aborts the merge with the per-op conflict list and
// 409; nothing is applied.
func (s *Server) HandleMergeChangeSet(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
	ctx := c.Request().Context()

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	opPtrs, err := s.KnowledgeStore.ListOps(ctx, wsID, cs.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	governed, err := knowledge.ChangeSetIsGoverned(changeSetOpValues(opPtrs))
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	// A governed change-set is a brand action; escalate the permission gate.
	if governed {
		if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
			return err
		}
	}

	engine, err := s.knowledgeEngineFor(c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: err.Error()})
	}

	cs.MergedBy = actor
	res, mergeErr := engine.MergeChangeSet(ctx, wsID, s.KnowledgeStore, *cs)
	if mergeErr != nil {
		switch {
		case errors.Is(mergeErr, knowledge.ErrMergeConflict):
			return c.JSON(http.StatusConflict, map[string]any{
				"error":     "change-set has stale-draft conflicts; re-base the listed ops and resubmit",
				"conflicts": res.Conflicts,
			})
		case res != nil && len(res.AppliedOps) > 0:
			// A mid-apply failure: report which ops already landed so the partial
			// state is visible and the merge can be re-driven once fixed.
			return c.JSON(http.StatusInternalServerError, map[string]any{
				"error":   mergeErr.Error(),
				"applied": res.AppliedOps,
			})
		default:
			// The merge gate refused it (not approved / separation of duties / not
			// in a mergeable state).
			return c.JSON(http.StatusConflict, ErrorResponse{Error: mergeErr.Error()})
		}
	}

	s.publishKnowledgeEvents(c, res.Events)
	return c.JSON(http.StatusOK, res)
}

// HandleAbandonChangeSet retires a change-set: it stops all of the change-set's
// pilots and marks it abandoned. Already-terminal change-sets (merged or
// abandoned) are refused.
func (s *Server) HandleAbandonChangeSet(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if s.KnowledgeStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errKnowledgeUnavailable.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
	ctx := c.Request().Context()

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	if cs.Status == knowledge.ChangeSetMerged || cs.Status == knowledge.ChangeSetAbandoned {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: fmt.Sprintf("change-set is already %s", cs.Status)})
	}

	engine, err := s.knowledgeEngineFor(c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: err.Error()})
	}
	_, pilotEvents, err := engine.StopAllPilots(ctx, wsID, s.KnowledgeStore, *cs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if err := s.KnowledgeStore.SetChangeSetStatus(ctx, wsID, cs.ID, knowledge.ChangeSetAbandoned); err != nil {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	}

	events := append(pilotEvents, changesetEvent(knowledge.EventChangeSetAbandoned, wsID, cs.ID, actor))
	s.publishKnowledgeEvents(c, events)
	return s.refreshedChangeSet(c, wsID, cs.ID)
}

// ---------------------------------------------------------------------------
// Blast radius + pilots
// ---------------------------------------------------------------------------

// HandleChangeSetBlastRadius previews a change-set's blast radius over the
// workspace's stored content — how many blocks the draft would newly flag or
// resolve, per project → collection → (stream, locale) — including the content
// already exercising the draft through its pilot streams. Nothing is persisted.
func (s *Server) HandleChangeSetBlastRadius(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	engine, err := s.knowledgeEngineFor(c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: err.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	opPtrs, err := s.KnowledgeStore.ListOps(ctx, wsID, cs.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	pilots, err := s.KnowledgeStore.ListPilots(ctx, wsID, cs.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	impact, err := engine.EvaluateChangeSet(ctx, wsID, *cs, changeSetOpValues(opPtrs), knowledge.EvalOptions{
		PilotStreams: pilotStreams(pilots),
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, impact)
}

// HandleStartPilot binds a change-set to a project's content stream as a pilot,
// so real content and real checks resolve through the draft before it merges.
func (s *Server) HandleStartPilot(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	engine, err := s.knowledgeEngineFor(c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: err.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
	ctx := c.Request().Context()

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	var req StartPilotRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if strings.TrimSpace(req.ProjectID) == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "project_id is required"})
	}
	if strings.TrimSpace(req.Stream) == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "stream is required"})
	}
	pilot, err := engine.StartPilot(ctx, wsID, s.KnowledgeStore, *cs, req.ProjectID, req.Stream)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		pilotEvent(knowledge.EventPilotStarted, wsID, cs.ID, req.ProjectID, req.Stream, actor),
	})
	return c.JSON(http.StatusCreated, pilot)
}

// HandleStopPilot retires a single pilot of a change-set, removing its
// stream-scoped shadow and candidate voice binding.
func (s *Server) HandleStopPilot(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	engine, err := s.knowledgeEngineFor(c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: err.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	actor, _ := c.Get("user_id").(string)
	ctx := c.Request().Context()

	cs, resp := s.getChangeSetOr404(c, wsID, c.Param("id"))
	if cs == nil {
		return resp
	}
	project := c.Param("project")
	stream := c.Param("stream")
	if err := engine.StopPilot(ctx, wsID, s.KnowledgeStore, *cs, project, stream); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	s.publishKnowledgeEvents(c, []knowledge.MergeEvent{
		pilotEvent(knowledge.EventPilotStopped, wsID, cs.ID, project, stream, actor),
	})
	return c.NoContent(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// getChangeSetOr404 loads a change-set, or writes a 404 and returns a nil
// change-set. The second return is the already-written response (nil on the
// happy path, and nil-but-written on the 404 path) for the caller to return.
func (s *Server) getChangeSetOr404(c echo.Context, wsID, id string) (*knowledge.ChangeSet, error) {
	cs, err := s.KnowledgeStore.GetChangeSet(c.Request().Context(), wsID, id)
	if err != nil {
		return nil, c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return cs, nil
}

// refreshedChangeSet re-reads a change-set after a lifecycle transition and
// returns it as the response, so the caller sees the new state.
func (s *Server) refreshedChangeSet(c echo.Context, wsID, id string) error {
	cs, err := s.KnowledgeStore.GetChangeSet(c.Request().Context(), wsID, id)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}
	return c.JSON(http.StatusOK, cs)
}

// changeSetOpValues dereferences a slice of op pointers into the value slice the
// pure change-set functions (ChangeSetIsGoverned, EvaluateChangeSet) operate on.
func changeSetOpValues(ptrs []*knowledge.ChangeSetOp) []knowledge.ChangeSetOp {
	ops := make([]knowledge.ChangeSetOp, 0, len(ptrs))
	for _, p := range ptrs {
		if p != nil {
			ops = append(ops, *p)
		}
	}
	return ops
}

// pilotStreams groups a change-set's pilots into the per-project stream map an
// EvalOptions blast-radius walk includes beyond each project's "main" stream.
func pilotStreams(pilots []*knowledge.Pilot) map[string][]string {
	if len(pilots) == 0 {
		return nil
	}
	m := map[string][]string{}
	for _, p := range pilots {
		if p == nil {
			continue
		}
		m[p.ProjectID] = append(m[p.ProjectID], p.Stream)
	}
	return m
}

// isVoiceOpType reports whether an op type targets a brand-voice profile (and is
// therefore a governed brand edit requiring manage_brand to author).
func isVoiceOpType(o knowledge.OpType) bool {
	return o == knowledge.OpVoiceRuleAdd || o == knowledge.OpVoiceRuleRemove
}

// changesetEvent builds a change-set-scoped knowledge event for a lifecycle
// transition the handler fires directly (create/submit/approve/reject/abandon).
func changesetEvent(t knowledge.EventType, workspaceID, changesetID, actor string) knowledge.MergeEvent {
	return knowledge.MergeEvent{
		Type:        t,
		WorkspaceID: workspaceID,
		ChangesetID: changesetID,
		Actor:       actor,
	}
}

// pilotEvent builds a pilot-scoped knowledge event (pilot.started/stopped) the
// handler fires directly.
func pilotEvent(t knowledge.EventType, workspaceID, changesetID, projectID, stream, actor string) knowledge.MergeEvent {
	return knowledge.MergeEvent{
		Type:        t,
		WorkspaceID: workspaceID,
		ChangesetID: changesetID,
		ProjectID:   projectID,
		Stream:      stream,
		Actor:       actor,
	}
}
