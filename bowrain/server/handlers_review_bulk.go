package server

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// ApprovePassingRequest scopes a bulk approve-passing pass. Both fields are
// optional: an empty stream defaults to the project's default stream (then
// "main"), and an empty Locales approves across every target language the caller
// may review.
type ApprovePassingRequest struct {
	Stream  string   `json:"stream,omitempty"`
	Locales []string `json:"locales,omitempty"`
}

// ApprovePassingResponse reports what the bulk pass did. RemainingPending is the
// count of targets still awaiting review after the pass (the excluded
// failing/off-brand blocks). ReviewCompleted is true when this pass emptied the
// project's whole review queue and kicked off the completing convergence run →
// delivery, so a surface can show "all approved · delivering…".
type ApprovePassingResponse struct {
	Approved         int  `json:"approved"`
	Skipped          int  `json:"skipped"`
	RemainingPending int  `json:"remaining_pending"`
	ReviewCompleted  bool `json:"review_completed"`
}

// HandleApprovePassing bulk-approves every block whose target for a locale is
// awaiting review AND clears the ship bar — passes the project's QA checks with
// no error-severity finding AND meets the brand on-brand bar (the same #1365
// shipstate predicate the dashboard aggregates). Blocks that fail checks or fall
// below the bar are EXCLUDED and left pending for a person. Approved blocks are
// promoted to reviewed; the locales that clear their review queue have their
// review task(s) closed, and if that empties the project's whole review queue
// the loop continues to a completing convergence run → delivery (RV-B).
//
// POST /:ws/:id/review/approve-passing  { "stream"?: "main", "locales"?: ["fr"] }
func (s *Server) HandleApprovePassing(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermReview); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	var req ApprovePassingRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found: " + err.Error()})
	}
	stream := refParamWithProject(c, proj)
	if req.Stream != "" {
		stream = req.Stream
	}

	// Candidate locales: the project's target languages, narrowed by the optional
	// locales filter and by the caller's own language scope (a reviewer scoped to
	// fr cannot approve de). project_languages empty = all languages allowed.
	wanted := map[string]bool{}
	for _, l := range req.Locales {
		wanted[l] = true
	}
	allowed, _ := c.Get("project_languages").([]string)
	allowedSet := map[string]bool{}
	for _, l := range allowed {
		allowedSet[l] = true
	}
	var locales []model.LocaleID
	for _, loc := range proj.TargetLanguages {
		ls := string(loc)
		if len(wanted) > 0 && !wanted[ls] {
			continue
		}
		if len(allowedSet) > 0 && !allowedSet[ls] {
			continue
		}
		locales = append(locales, loc)
	}

	blocks, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: pid, Stream: stream})
	if err != nil {
		return serverErr(c, fmt.Errorf("load blocks: %w", err))
	}
	scores := latestVoiceScores(ctx, s.BrandStore, pid, stream)
	// The same terminology gate the dashboard ship/on-brand pass uses, resolved
	// once (workspace terms snapshot + per-locale brand profile): a pending
	// draft that uses a forbidden term or misses a mandated one is off-brand and
	// must not be auto-approved. Nil gate (no terms/brand store) is a no-op.
	wsID, _ := c.Get("workspace_id").(string)
	gate := s.resolveTermGate(ctx, proj, stream, wsID)

	approved, skipped := 0, 0
	touchedSet := map[model.LocaleID]bool{}
	var toStore []*model.Block
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil || !sb.Block.Translatable {
			continue
		}
		modified := false
		for _, loc := range locales {
			if !targetPendingReview(sb.Block, loc) {
				continue // untranslated, or already approved — not a candidate
			}
			scored := scores[string(locale.Normalize(loc))]
			if blockOnBrandAndPassing(ctx, sb.Block, loc, scored, gate) {
				sb.Block.Target(loc).Status = model.TargetStatusReviewed
				approved++
				touchedSet[loc] = true
				modified = true
			} else {
				skipped++ // failing/off-brand: left pending for a person
			}
		}
		if modified {
			toStore = append(toStore, sb.Block)
		}
	}

	if len(toStore) > 0 {
		if err := s.ContentStore.StoreBlocks(ctx, pid, stream, toStore); err != nil {
			return serverErr(c, fmt.Errorf("store blocks: %w", err))
		}
	}

	// Remaining pending after the pass: the excluded (failing/off-brand) targets
	// still awaiting review. Computed over the same in-memory blocks, whose
	// approved targets were just promoted above.
	remaining := 0
	for _, sb := range blocks {
		for _, loc := range locales {
			if targetPendingReview(sb.Block, loc) {
				remaining++
			}
		}
	}

	touched := make([]model.LocaleID, 0, len(touchedSet))
	for loc := range touchedSet {
		touched = append(touched, loc)
	}
	actor, _ := c.Get("user_id").(string)
	reviewCompleted := s.advanceReviewLoop(ctx, proj, stream, touched, actor)

	s.invalidateDashboardCache(wsID, pid)

	return c.JSON(http.StatusOK, ApprovePassingResponse{
		Approved:         approved,
		Skipped:          skipped,
		RemainingPending: remaining,
		ReviewCompleted:  reviewCompleted,
	})
}
