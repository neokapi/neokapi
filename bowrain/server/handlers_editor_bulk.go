package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// A selection the editor can make in one gesture is a few hundred blocks; the
// cap keeps a hand-written request from turning a batch route into an
// unbounded job.
const maxBulkBlocks = 1000

// BulkReviewRequest applies one review decision to a selection of blocks.
// Status picks the demotion rung when Approve is false: "translated" (default,
// a withdrawn approval) or "draft" (a rejection). Comment, when present, is
// left on every block as a note and is the decision's note in the audit trail.
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
// blocks in a single request. It is a server action over the change contract:
// each block's decision is a decide operation on the translation the request
// read, applied through the stream's change service as the person, so the
// status transitions, the separation-of-duties policy and the decision-ledger
// writes are those of every decision. A block whose decision is refused (its
// translation moved, the person wrote it, it has no translation to approve)
// is reported in its own result and the rest land. The review-loop
// continuation runs once, after the pass.
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
	outcome, landsOn := change.OutcomeWithdraw, model.TargetStatusTranslated
	switch {
	case req.Approve && req.Status != "":
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "status only applies when approve is false (approval always lands on approved)"})
	case req.Approve:
		outcome, landsOn = change.OutcomeEstablish, model.TargetStatusEstablished
	case req.Status == string(model.TargetStatusDraft):
		outcome, landsOn = change.OutcomeReject, model.TargetStatusDraft
	case req.Status == "" || req.Status == string(model.TargetStatusTranslated):
	default:
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: `status must be "translated" or "draft"`})
	}
	// Approving is the review permission for the language; withdrawing an
	// approval or rejecting stays with translate.
	if err := s.requireLanguagePermission(c, reviewGateFor(req.Approve), req.TargetLocale); err != nil {
		return err
	}

	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found"})
	}
	stored, err := s.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: pid, Stream: stream, IDs: req.BlockIDs, Limit: len(req.BlockIDs),
	})
	if err != nil {
		return serverErr(c, err)
	}

	loc := model.LocaleID(req.TargetLocale)
	results := make(map[string]*BlockResult, len(req.BlockIDs))
	set := change.Set{Note: req.Comment}
	note, _ := json.Marshal(map[string]string{"text": req.Comment})
	for _, sb := range stored {
		if sb == nil || sb.Block == nil {
			continue
		}
		bid := sb.Block.ID
		results[bid] = &BlockResult{BlockID: bid}
		set.Ops = append(set.Ops, change.Op{Kind: change.KindDecide,
			At:      change.Ref{Doc: sb.ItemName, Block: bid, Edition: model.EditionKey{Locale: loc}},
			IfMatch: store.TargetRevision(sb, loc), Body: &change.Decide{Outcome: outcome}})
		if req.Comment != "" {
			set.Ops = append(set.Ops, change.Op{Kind: change.KindAnnotate,
				At:   change.Ref{Doc: sb.ItemName, Block: bid},
				Body: &change.Annotate{Type: noteAnnotation, Value: note}})
		}
	}

	wsID, _ := c.Get("workspace_id").(string)
	sc := s.newStreamChange(ctx, c, proj, stream, wsID, c.Param("ws"), requestSender(c))
	// One authorship query for the whole selection: the separation-of-duties
	// gate then costs nothing per block.
	if sc.decide.sod, err = s.newReviewSoD(ctx, c, pid, stream, req.BlockIDs, []string{req.TargetLocale}); err != nil {
		return serverErr(c, err)
	}
	res, applied, refused, err := sc.applyEach(ctx, set, change.Actor{Kind: change.ActorPerson, Name: sc.sender.userID})
	if err != nil {
		return serverErr(c, err)
	}
	for _, r := range refused {
		if br := results[r.Block]; br != nil {
			br.Error = "the server could not apply the change to this block"
			if r.Error != nil {
				br.Error = r.Error.Message
			}
		}
	}
	if res != nil && res.Status != change.SetRefused {
		for i, op := range res.Ops {
			if applied[i].Kind != change.KindDecide || op.At == nil {
				continue
			}
			br := results[applied[i].At.Block]
			if br == nil || br.Error != "" {
				continue
			}
			br.OK = true
			if op.Before != model.AbsentRevision {
				br.Status = string(landsOn)
			}
		}
	}

	out := BulkReviewResponse{Results: make([]BlockResult, 0, len(req.BlockIDs)), ReviewCompleted: sc.completed}
	for _, bid := range req.BlockIDs {
		br := results[bid]
		if br == nil {
			br = &BlockResult{BlockID: bid, Error: "block not found"}
		}
		if !br.OK && br.Error == "" {
			br.Error = "the server could not apply the change to this block"
		}
		if br.OK {
			out.Succeeded++
		} else {
			out.Failed++
		}
		out.Results = append(out.Results, *br)
	}
	return c.JSON(http.StatusOK, out)
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
// threshold into each selected block's target, in one request. The workspace
// memory and the project's source language resolve once, and the accepted
// matches are committed through the stream's change service as the
// content-memory tool's drafts: a block a person changed since the pass read
// it keeps the person's change and is reported skipped.
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

	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found"})
	}
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
	// The state each block was read in, before a match changes it in place.
	pass := beginToolPass(stored, proj.DefaultSourceLanguage)
	byID := make(map[string]*model.Block, len(stored))
	for _, sb := range stored {
		byID[sb.Block.ID] = sb.Block
	}

	loc := model.LocaleID(req.TargetLocale)
	var drafted []*model.Block
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
			// The access state a person's edit meets: restricted or published
			// content is not editable by an ordinary translate permission.
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
		// The match is applied to the block this request read, which a preview
		// then never commits — the preview's verdict is the pass's own rather
		// than a second copy of it.
		memoryTarget(b, loc, best.Target)
		drafted = append(drafted, b)
		resp.Applied = append(resp.Applied, AppliedMemory{BlockID: bid, Text: best.Target, Score: best.Score})
	}

	if len(drafted) == 0 || req.Preview {
		return c.JSON(http.StatusOK, resp)
	}
	wsID, _ := c.Get("workspace_id").(string)
	landed, err := pass.commit(ctx, s.commitTo(c, proj, stream, wsID), "bulk-apply-memory", drafted)
	if err != nil {
		return serverErr(c, fmt.Errorf("store blocks: %w", err))
	}
	took := make(map[string]bool, len(landed))
	for _, id := range landed {
		took[id] = true
	}
	applied := resp.Applied[:0]
	for _, a := range resp.Applied {
		if took[a.BlockID] {
			applied = append(applied, a)
			continue
		}
		resp.Skipped = append(resp.Skipped, SkippedMemory{BlockID: a.BlockID, Reason: "the block changed since it was read"})
	}
	resp.Applied = applied
	return c.JSON(http.StatusOK, resp)
}

// memoryTarget writes text as b's translation into loc, produced by the
// content memory: an established translation whose wording the match changes
// drops to translated, since the approval judged other wording.
func memoryTarget(b *model.Block, loc model.LocaleID, text string) {
	before, had := b.TargetEdition(loc)
	established := had && model.TargetStatus(before.Status) == model.TargetStatusEstablished
	oldRuns := b.TargetRuns(loc)
	b.SetTargetText(loc, text)
	t, ok := b.TargetEdition(loc)
	if !ok {
		return
	}
	t.Origin = model.Origin{Kind: model.OriginMemory, Timestamp: time.Now().UTC().Format(time.RFC3339)}
	if established && !reflect.DeepEqual(oldRuns, t.Runs) {
		t.Status = model.Status(model.TargetStatusTranslated)
	}
	b.SetTargetEdition(model.Variant(loc), t)
}
