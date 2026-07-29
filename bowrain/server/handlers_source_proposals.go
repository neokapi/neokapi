package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
)

// This file adds the back-to-source lane (RV-F), the one direction the review
// loop lacked: every other review effect is target/locale-scoped ("reviewing
// French never touches German"), so a reviewer who catches a source-TEXT problem
// while reviewing one locale had no governed way to fix it. Now they PROPOSE a
// source change (any reviewer, PermReview); a source owner (PermEditSource)
// APPROVES it — which applies the fix to the block's source and re-drafts EVERY
// locale — or rejects it. The proposal is the reviewed hand-off between the two
// permissions.
//
// The re-draft fan-out is the crux. On approve, applySourceProposal writes the
// new source and CLEARS the block's targets for every locale, then starts a
// convergence run. Clearing the targets (not merely demoting their status) is
// what forces the re-draft: the translation worker skips any block that still
// carries a non-empty target (memory_recycle.hasLocaleTarget), so a demoted-but-kept
// target would never be regenerated. This mirrors the CLI's source-drift
// mechanism, where a changed source mints a new block identity whose targets do
// not carry over, leaving it untranslated → re-drafted on the next pass.

// CreateSourceProposalRequest proposes a change to a block's source. Sent from
// the review session by any reviewer.
type CreateSourceProposalRequest struct {
	BlockID        string `json:"block_id"`
	ItemName       string `json:"item_name,omitempty"`
	ProposedSource string `json:"proposed_source"`
	Kind           string `json:"kind,omitempty"`
	Rationale      string `json:"rationale,omitempty"`
	FoundInLocale  string `json:"found_in_locale,omitempty"`
}

// DecideSourceProposalRequest approves or rejects a proposal. Gated on
// PermEditSource — the existing source-authoring permission.
type DecideSourceProposalRequest struct {
	Decision string `json:"decision"` // "approve" | "reject"
	Reason   string `json:"reason,omitempty"`
}

// HandleListSourceProposals returns the open source-change proposals for a
// project — the source owner's review surface.
//
// GET /:ws/:id/source-proposals
func (s *Server) HandleListSourceProposals(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.SourceProposalStore == nil {
		return c.JSON(http.StatusOK, map[string]any{"proposals": []bstore.ProposedSourceChange{}})
	}
	pid := projectParam(c)
	proposals, err := s.SourceProposalStore.ListOpen(c.Request().Context(), pid)
	if err != nil {
		return serverErr(c, err)
	}
	if proposals == nil {
		proposals = []bstore.ProposedSourceChange{}
	}
	return c.JSON(http.StatusOK, map[string]any{"proposals": proposals})
}

// HandleCreateSourceProposal records a reviewer's proposed source change and
// surfaces it to source owners as a source-review task. It applies nothing — a
// PermEditSource owner must approve first.
//
// POST /:ws/:id/source-proposals
func (s *Server) HandleCreateSourceProposal(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermReview); err != nil {
		return err
	}
	if s.SourceProposalStore == nil || s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "source proposals not configured"})
	}

	pid := projectParam(c)
	stream := streamParam(c)

	var req CreateSourceProposalRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.BlockID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "block_id is required"})
	}
	if strings.TrimSpace(req.ProposedSource) == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "proposed_source is required"})
	}
	kind := bstore.SourceProposalKind(req.Kind)
	switch kind {
	case "", bstore.SourceProposalTextFix:
		kind = bstore.SourceProposalTextFix
	case bstore.SourceProposalMarkDNT:
	default:
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: `kind must be "text-fix" or "mark-dnt"`})
	}

	ctx := c.Request().Context()
	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, req.BlockID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found: " + err.Error()})
	}
	itemName := req.ItemName
	if itemName == "" {
		itemName = sb.ItemName
	}
	if strings.TrimSpace(req.ProposedSource) == sb.Block.SourceText() && kind == bstore.SourceProposalTextFix {
		return c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: "proposed_source is identical to the current source"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	finder, _ := c.Get("user_id").(string)
	proposal := &bstore.ProposedSourceChange{
		WorkspaceID:    wsID,
		ProjectID:      pid,
		Stream:         stream,
		ItemName:       itemName,
		BlockID:        req.BlockID,
		Kind:           kind,
		OriginalSource: sb.Block.SourceText(),
		ProposedSource: req.ProposedSource,
		Rationale:      req.Rationale,
		FoundInLocale:  req.FoundInLocale,
		FinderUser:     finder,
	}
	if err := s.SourceProposalStore.Create(ctx, proposal); err != nil {
		return serverErr(c, err)
	}

	// Surface to source owners: a source-review task in their queue. Best-effort —
	// the proposal is authoritative even if task creation is skipped.
	if proj, perr := s.ContentStore.GetProject(ctx, pid); perr == nil {
		s.createSourceProposalTask(context.WithoutCancel(ctx), proj, proposal)
	}

	return c.JSON(http.StatusCreated, proposal)
}

// HandleDecideSourceProposal approves or rejects an open proposal. Approval is
// governed on PermEditSource — the source-authoring permission — because it
// changes the source and fans out to every locale.
//
// POST /:ws/:id/source-proposals/:pid/decide
func (s *Server) HandleDecideSourceProposal(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermEditSource); err != nil {
		return err
	}
	if s.SourceProposalStore == nil || s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "source proposals not configured"})
	}

	pid := projectParam(c)
	proposalID := c.Param("pid")

	var req DecideSourceProposalRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Decision != "approve" && req.Decision != "reject" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "decision must be 'approve' or 'reject'"})
	}

	ctx := c.Request().Context()
	proposal, err := s.SourceProposalStore.Get(ctx, proposalID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "proposal not found"})
	}
	if proposal.ProjectID != pid {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "proposal not found"})
	}
	if proposal.Status != bstore.SourceProposalOpen {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: "proposal already " + string(proposal.Status)})
	}

	decider, _ := c.Get("user_id").(string)

	if req.Decision == "reject" {
		if err := s.SourceProposalStore.Resolve(ctx, proposalID, bstore.SourceProposalRejected, decider, req.Reason); err != nil {
			return serverErr(c, err)
		}
		s.completeSourceProposalTasks(context.WithoutCancel(ctx), proposal, decider)
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "status": "rejected"})
	}

	// Approve: apply the source change (which re-drafts every locale) then record
	// the decision.
	runStarted, err := s.applySourceProposal(ctx, proposal, decider)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: "apply source change: " + err.Error()})
	}
	if err := s.SourceProposalStore.Resolve(ctx, proposalID, bstore.SourceProposalApproved, decider, req.Reason); err != nil {
		return serverErr(c, err)
	}
	s.completeSourceProposalTasks(context.WithoutCancel(ctx), proposal, decider)

	return c.JSON(http.StatusOK, map[string]any{
		"ok": true, "status": "approved", "applied": true, "run_started": runStarted,
	})
}

// applySourceProposal writes the approved source change to the block and forces a
// re-draft of every locale. It reports whether a convergence run was started (the
// nudge that makes the fan-out happen without a second user action).
//
// Why CLEAR the targets rather than demote them: the translation worker skips any
// block that still carries a non-empty target for a locale (memory_recycle
// hasLocaleTarget), so a target kept at draft would never be regenerated. Clearing
// the targets removes them from every locale's coverage, the locale re-enters the
// pending set, and the next convergence pass re-drafts the block from the new
// source — the server-side analog of the CLI's source-drift → new-identity →
// untranslated path.
func (s *Server) applySourceProposal(ctx context.Context, p *bstore.ProposedSourceChange, actor string) (bool, error) {
	stream := p.Stream
	if stream == "" {
		stream = "main"
	}
	sb, err := s.ContentStore.GetBlock(ctx, p.ProjectID, stream, p.BlockID)
	if err != nil {
		return false, err
	}
	block := sb.Block

	// Apply the source transform. A single TextRun is the faithful shape for a
	// plain-text source fix; inline markup, if any, is not carried by this lane.
	block.SetSourceText(p.ProposedSource)
	// The source changed, so its authoring status must be re-settled — reset it to
	// the New baseline so the next run's source-first phase re-checks it.
	block.SourceStatus = model.SourceStatusNew
	// Drop the (now-stale) targets from the in-memory block so the write below does
	// not re-persist them, then persist the new source.
	block.Targets = map[model.VariantKey]*model.Target{}
	if err := s.ContentStore.StoreBlocks(ctx, p.ProjectID, stream, []*model.Block{block}); err != nil {
		return false, err
	}
	// Invalidate every locale: the store's target write is upsert-only, so removing
	// targets from the block above does not delete their rows — clear them
	// explicitly. Now the block carries no target for any locale and the next
	// convergence pass re-drafts them all (the worker skips a block that still has a
	// target). This is the server analog of the CLI source-drift path.
	if err := s.ContentStore.ClearBlockTargets(ctx, p.ProjectID, stream, block.ID); err != nil {
		return false, err
	}

	s.invalidateDashboardCache(p.WorkspaceID, p.ProjectID)
	s.publishEditorBlockChange(p.ProjectID, block.ID, p.ItemName, stream, "updated", actor, "")

	// A mark-dnt proposal additionally records the (new) source string as
	// do-not-translate so the re-draft leaves it untranslated.
	if p.Kind == bstore.SourceProposalMarkDNT && s.ReviewQueueStore != nil {
		_ = s.ReviewQueueStore.AddDNTEntry(ctx, p.ProjectID, p.ProposedSource, "", p.FoundInLocale, "source-proposal")
	}

	// Nudge: start a convergence run so the cleared targets are re-drafted straight
	// away. The run outlives this request, so a fresh background context is correct
	// (matching the review-completion run start). It is a no-op for a project with
	// no convergence wired.
	if s.convergence == nil {
		return false, nil
	}
	if _, _, rerr := s.convergence.StartRun(context.WithoutCancel(ctx), p.ProjectID, "source-change", nil); rerr != nil {
		// The source change already persisted; a failed run start is not fatal —
		// the next converge pass (manual or on-push) still picks up the cleared
		// targets. Log via the caller's response, not a hard failure.
		return false, nil
	}
	return true, nil
}

// createSourceProposalTask surfaces a proposal to a source owner as a
// source-review task, assigning it to the first project member holding
// PermEditSource (the same owner-reach fallback create_source_review uses). Data
// carries the proposal id so the decision path can close the task.
func (s *Server) createSourceProposalTask(ctx context.Context, proj *platstore.Project, p *bstore.ProposedSourceChange) {
	if s.TaskStore == nil {
		return
	}
	task := &bstore.Task{
		WorkspaceID: proj.WorkspaceID,
		ProjectID:   proj.ID,
		Stream:      p.Stream,
		Type:        bstore.TaskSourceReview,
		Status:      bstore.TaskStatusOpen,
		Priority:    bstore.TaskPriorityNormal,
		Title:       "Review a proposed source change",
		AssigneeID:  s.firstSourceOwner(ctx, proj),
		CreatedBy:   "system",
		Data: map[string]string{
			"proposal_id": p.ID,
			"block_id":    p.BlockID,
			"locale":      p.FoundInLocale,
			"kind":        string(p.Kind),
		},
	}
	if err := s.TaskStore.Create(ctx, task); err != nil {
		return
	}
	if task.AssigneeID != "" && s.NotificationDispatcher != nil {
		s.NotificationDispatcher.DispatchTaskNotification(
			ctx, task, bstore.NotificationTaskAssigned,
			"Source change proposed", "A reviewer proposed a change to your source content")
	}
}

// completeSourceProposalTasks closes the open source-review task(s) linked to a
// decided proposal, so the owner's queue clears when the decision lands.
func (s *Server) completeSourceProposalTasks(ctx context.Context, p *bstore.ProposedSourceChange, actor string) {
	if s.TaskStore == nil {
		return
	}
	res, err := s.TaskStore.List(ctx, bstore.TaskQuery{
		WorkspaceID: p.WorkspaceID,
		ProjectID:   p.ProjectID,
		Type:        string(bstore.TaskSourceReview),
		Statuses:    []string{string(bstore.TaskStatusOpen), string(bstore.TaskStatusInProgress)},
		Limit:       500,
	})
	if err != nil {
		return
	}
	completedBy := actor
	if completedBy == "" {
		completedBy = "system"
	}
	for _, t := range res.Tasks {
		if t.Data["proposal_id"] != p.ID {
			continue
		}
		_ = s.TaskStore.Complete(ctx, t.ID, completedBy)
	}
}

// firstSourceOwner returns the id of the first project member holding
// PermEditSource, or "" if none — the owner-reach fallback for routing a source
// proposal (matching createSourceReviewTask).
func (s *Server) firstSourceOwner(ctx context.Context, proj *platstore.Project) string {
	if s.AuthStore == nil {
		return ""
	}
	members, err := s.AuthStore.ListProjectMembers(ctx, proj.ID)
	if err != nil {
		return ""
	}
	for _, m := range members {
		rt, err := s.AuthStore.GetRoleTemplate(ctx, proj.WorkspaceID, m.RoleID)
		if err == nil && rt.Permissions.Has(platauth.PermEditSource) {
			return m.UserID
		}
	}
	return ""
}
