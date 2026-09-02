package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
)

// A selection the editor can make in one gesture is a few hundred blocks; the
// cap keeps a hand-written request from turning a batch route into an
// unbounded job.
const maxBulkBlocks = 1000

// BulkReviewRequest applies one review decision to a selection of blocks.
// Status picks the demotion rung when Approve is false, exactly as the
// single-block route's field does: "translated" (default) or "draft" (a
// rejection). Comment, when present, is attached to every block as a note.
type BulkReviewRequest struct {
	BlockIDs     []string `json:"block_ids"`
	TargetLocale string   `json:"target_locale"`
	Approve      bool     `json:"approve"`
	Status       string   `json:"status,omitempty"`
	Comment      string   `json:"comment,omitempty"`
	ItemName     string   `json:"item_name,omitempty"`
}

// BlockResult reports one block's outcome inside a batch. Error is empty
// exactly when OK is true.
type BlockResult struct {
	BlockID string `json:"block_id"`
	OK      bool   `json:"ok"`
	Status  string `json:"status,omitempty"`
	Error   string `json:"error,omitempty"`
}

// BulkReviewResponse reports the batch. ReviewCompleted is true when the
// approvals emptied the project's whole review queue and the completing
// convergence run was handed off.
type BulkReviewResponse struct {
	Results         []BlockResult `json:"results"`
	Succeeded       int           `json:"succeeded"`
	Failed          int           `json:"failed"`
	ReviewCompleted bool          `json:"review_completed"`
}

// HandleBulkReviewBlocks applies one review decision across a selection of
// blocks in a single request, replacing the editor's per-block request loop.
// Each block goes through the same code path as the single-block route
// (applyBlockReview), so the status transitions, the demotion rules and the
// decision-ledger writes are identical; a block that refuses is recorded in
// its own result rather than failing the batch. The review-loop continuation
// runs once, after the pass, for the locale that saw a real approval.
//
// The gates are the single-block route's gates: approving needs PermReview for
// the language, withdrawing or rejecting needs PermTranslate, and an approval
// of work the caller wrote themselves meets the workspace separation-of-duties
// policy. The authorship the policy needs is read once for the whole selection.
//
// POST /:ws/:id/blocks/:ref/bulk-review
//
//	{ "block_ids": ["b1","b2"], "target_locale": "fr", "approve": true }
func (s *Server) HandleBulkReviewBlocks(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	stream := streamParam(c)

	var req BulkReviewRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.TargetLocale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "target_locale is required"})
	}
	if len(req.BlockIDs) == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "block_ids is required"})
	}
	if len(req.BlockIDs) > maxBulkBlocks {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("block_ids holds %d blocks, more than the %d a single request applies", len(req.BlockIDs), maxBulkBlocks),
		})
	}
	demoteTo := model.TargetStatusTranslated
	switch req.Status {
	case "", string(model.TargetStatusTranslated):
	case string(model.TargetStatusDraft):
		demoteTo = model.TargetStatusDraft
	default:
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: `status must be "translated" or "draft"`})
	}
	if req.Approve && req.Status != "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "status only applies when approve is false (approval always lands on reviewed)"})
	}
	// Approving is the review permission for the language, exactly as the
	// single-block route gates it; withdrawing an approval or rejecting stays
	// with translate.
	if err := s.requireLanguagePermission(c, reviewGateFor(req.Approve), req.TargetLocale); err != nil {
		return err
	}

	// The elevated gate for demoting a signed-off target, as a predicate: one
	// protected block in the selection must not answer for the whole batch.
	elevate := func() error {
		if allowsLanguage(c, platauth.PermReview, req.TargetLocale) {
			return nil
		}
		return reviewFault{http.StatusForbidden, "demoting a signed-off target requires review permission"}
	}

	ctx := c.Request().Context()
	// One authorship query for the whole selection, before the pass: the
	// separation-of-duties gate then costs nothing per block.
	sod, err := s.newReviewSoD(ctx, c, pid, stream, req.BlockIDs, []string{req.TargetLocale})
	if err != nil {
		return serverErr(c, err)
	}
	results := make([]BlockResult, 0, len(req.BlockIDs))
	succeeded, failed, approvals := 0, 0, 0
	for _, bid := range req.BlockIDs {
		out, err := s.applyBlockReview(ctx, c, blockReviewInput{
			ProjectID: pid, Stream: stream, BlockID: bid, DemoteTo: demoteTo, Elevate: elevate,
			Vet: sod.vet,
			Request: ReviewBlockRequest{
				TargetLocale: req.TargetLocale,
				ItemName:     req.ItemName,
				Reviewed:     req.Approve,
				Status:       req.Status,
			},
		})
		if err != nil {
			failed++
			results = append(results, BlockResult{BlockID: bid, Error: bulkBlockError(err)})
			continue
		}
		succeeded++
		if out.Approval {
			approvals++
		}
		if out.Changed {
			s.emitReviewDecisionAudit(c, pid, stream, bid, req.TargetLocale, out.From, out.Status, req.Approve, req.Comment)
		}
		results = append(results, BlockResult{BlockID: bid, OK: true, Status: string(out.Status)})
		if req.Comment != "" {
			s.addReviewComment(ctx, c, pid, stream, bid, req.Comment)
		}
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)

	reviewCompleted := false
	if approvals > 0 {
		if proj, perr := s.ContentStore.GetProject(ctx, pid); perr == nil {
			actor, _ := c.Get("user_id").(string)
			reviewCompleted = s.advanceReviewLoop(ctx, proj, stream, []model.LocaleID{model.LocaleID(req.TargetLocale)}, actor)
		}
	}

	return c.JSON(http.StatusOK, BulkReviewResponse{
		Results: results, Succeeded: succeeded, Failed: failed, ReviewCompleted: reviewCompleted,
	})
}

// bulkBlockError renders one block's failure for its result entry. A refusal
// carries its own sentence; anything else is a server fault the batch reports
// without leaking internals.
func bulkBlockError(err error) string {
	if fault, ok := errors.AsType[reviewFault](err); ok {
		return fault.msg
	}
	return "the server could not apply the change to this block"
}

// addReviewComment attaches the batch's comment to one block. Best-effort: the
// review decision is already written, and a note that fails to store must not
// undo it.
func (s *Server) addReviewComment(ctx context.Context, c echo.Context, projectID, stream, blockID, text string) {
	_ = s.ContentStore.AddBlockNote(ctx, projectID, stream, blockID, model.BlockNote{
		ID:        id.New(),
		BlockID:   blockID,
		Author:    extractAuthor(c),
		Text:      text,
		CreatedAt: time.Now().UTC(),
	})
}

// BulkApplyMemoryRequest applies the best content-memory match to a selection
// of blocks. Threshold is the minimum score a match must reach; it defaults to
// 1, an exact match.
type BulkApplyMemoryRequest struct {
	BlockIDs     []string `json:"block_ids"`
	TargetLocale string   `json:"target_locale"`
	Threshold    *float64 `json:"threshold,omitempty"`
	// Preview asks what the pass WOULD write and writes nothing. The response
	// is the same one the pass returns, so a surface can name every match and
	// every skip before a reviewer commits to them — the point of the flag is
	// that the answer comes from the pass itself rather than from a second
	// prediction of it.
	Preview bool `json:"preview,omitempty"`
}

// AppliedMemory names a block that took a match, and what it took.
type AppliedMemory struct {
	BlockID string  `json:"block_id"`
	Text    string  `json:"text"`
	Score   float64 `json:"score"`
}

// SkippedMemory names a block that took nothing, and why.
type SkippedMemory struct {
	BlockID string `json:"block_id"`
	Reason  string `json:"reason"`
}

// BulkApplyMemoryResponse reports the pass.
type BulkApplyMemoryResponse struct {
	Applied []AppliedMemory `json:"applied"`
	Skipped []SkippedMemory `json:"skipped"`
}

// HandleBulkApplyMemory writes the best content-memory match above the
// threshold into each selected block's target, in one request — the pass the
// editor used to make with a lookup and an update call per block. The
// workspace memory and the project's source language resolve once; the
// accepted blocks are stored in a single write, so a review a match
// invalidates is demoted by the same rule an ordinary edit follows.
//
// POST /:ws/:id/blocks/:ref/bulk-apply-memory
//
//	{ "block_ids": ["b1","b2"], "target_locale": "fr", "threshold": 1 }
func (s *Server) HandleBulkApplyMemory(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermTranslate); err != nil {
		return err
	}
	if s.ContentStore == nil || s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ws := c.Param("ws")
	pid := projectParam(c)
	stream := streamParam(c)

	var req BulkApplyMemoryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.TargetLocale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "target_locale is required"})
	}
	if len(req.BlockIDs) == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "block_ids is required"})
	}
	if len(req.BlockIDs) > maxBulkBlocks {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("block_ids holds %d blocks, more than the %d a single request applies", len(req.BlockIDs), maxBulkBlocks),
		})
	}
	if err := s.requireLanguagePermission(c, platauth.PermTranslate, req.TargetLocale); err != nil {
		return err
	}
	threshold := 1.0
	if req.Threshold != nil {
		threshold = *req.Threshold
	}

	ctx := c.Request().Context()
	resp := BulkApplyMemoryResponse{Applied: []AppliedMemory{}, Skipped: []SkippedMemory{}}

	lookup, err := newMemoryLookup(ctx, s.ContentStore, s.wsStores, ws, pid)
	if err != nil {
		return serverErr(c, err)
	}
	if lookup == nil {
		for _, bid := range req.BlockIDs {
			resp.Skipped = append(resp.Skipped, SkippedMemory{BlockID: bid, Reason: "content memory is empty"})
		}
		return c.JSON(http.StatusOK, resp)
	}

	stored, err := s.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: pid, Stream: stream, IDs: req.BlockIDs, Limit: len(req.BlockIDs),
	})
	if err != nil {
		return serverErr(c, err)
	}
	byID := make(map[string]*model.Block, len(stored))
	for _, sb := range stored {
		byID[sb.Block.ID] = sb.Block
	}

	loc := model.LocaleID(req.TargetLocale)
	var toStore []*model.Block
	for _, bid := range req.BlockIDs {
		b := byID[bid]
		switch {
		case b == nil:
			resp.Skipped = append(resp.Skipped, SkippedMemory{BlockID: bid, Reason: "block not found"})
			continue
		case !b.Translatable:
			resp.Skipped = append(resp.Skipped, SkippedMemory{BlockID: bid, Reason: "block is not translatable"})
			continue
		case !s.blockEditAllowed(c, pid, bid, req.TargetLocale):
			// The same ABAC gate the single-block target update enforces:
			// restricted or published content is not editable by an ordinary
			// translate permission.
			resp.Skipped = append(resp.Skipped, SkippedMemory{BlockID: bid, Reason: "not permitted to edit this block"})
			continue
		}
		matches, err := lookup.matches(ctx, b, req.TargetLocale)
		if err != nil {
			resp.Skipped = append(resp.Skipped, SkippedMemory{BlockID: bid, Reason: "content-memory lookup failed"})
			continue
		}
		if len(matches) == 0 || matches[0].Score < threshold || matches[0].Target == "" {
			resp.Skipped = append(resp.Skipped, SkippedMemory{BlockID: bid, Reason: "no match at or above the threshold"})
			continue
		}
		best := matches[0]
		// The edit is applied to the block this request read, which a preview
		// then never stores — the mutation dies with the request, and the
		// preview's verdict is the pass's own rather than a second copy of it.
		applyTargetTextEdit(b, loc, best.Target, model.Origin{
			Kind:      model.OriginMemory,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		toStore = append(toStore, b)
		resp.Applied = append(resp.Applied, AppliedMemory{BlockID: bid, Text: best.Target, Score: best.Score})
	}

	if len(toStore) > 0 && !req.Preview {
		if err := s.ContentStore.StoreBlocks(ctx, pid, stream, toStore); err != nil {
			return serverErr(c, fmt.Errorf("store blocks: %w", err))
		}
		for _, b := range toStore {
			s.emitEditorBlockChange(c, pid, b.ID, "", stream, "updated")
		}
		wsID, _ := c.Get("workspace_id").(string)
		s.invalidateDashboardCache(wsID, pid)
	}

	return c.JSON(http.StatusOK, resp)
}
