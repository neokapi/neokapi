package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// recordReviewDecision appends a server-made review to the decision ledger
// with the decider's identity, the time, and the hash of the translation it
// blesses. Best-effort by design: the projected status is already written, so
// a store without the ledger capability (or a failed write, which is logged
// by the store) must not fail the review the user just made — but the ledger
// is what makes the review a decision rather than an enum flip, so the write
// is attempted on every rung change.
func (s *Server) recordReviewDecision(ctx context.Context, c echo.Context, projectID, stream string, sb *venue.StoredBlock, locale string, status model.TargetStatus, approved bool) {
	ds, ok := s.ContentStore.(platstore.DecisionStore)
	if !ok || sb == nil || sb.SourceID == "" {
		return
	}
	// The decider, as readably as the auth store can name them. Falls back to
	// the opaque user id — an identity, if a terse one.
	decider, _ := c.Get("user_id").(string)
	if s.AuthStore != nil && decider != "" {
		if u, err := s.AuthStore.GetUser(ctx, decider); err == nil && u != nil && u.Email != "" {
			decider = u.Email
		}
	}
	reviewState := ""
	switch {
	case approved && status == model.TargetStatusSignedOff:
		reviewState = "signed-off"
	case approved:
		reviewState = "approved"
	case status == model.TargetStatusDraft:
		reviewState = "rejected"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := ds.UpsertUnitDecisions(ctx, projectID, stream, []venue.UnitDecision{{
		ItemName:    sb.ItemName,
		Unit:        sb.SourceID,
		Variant:     locale,
		Status:      string(status),
		TargetHash:  state.TargetHash(sb.Block.TargetText(model.LocaleID(locale))),
		ContentHash: state.SourceHash(sb.Block.SourceText()),
		ReviewState: reviewState,
		DecidedBy:   decider,
		DecidedAt:   now,
		Updated:     now,
	}}); err != nil {
		// Best-effort must still be observed: the projected status is written,
		// but a ledger that quietly misses reviews is worse than none.
		slog.WarnContext(ctx, "review decision not recorded in ledger",
			"project", projectID, "block", sb.SourceID, "locale", locale, "error", err)
		return
	}

	// The corpus follows the decision: an approval promotes its wording into
	// the workspace content memory, a rejection evicts it — the same single
	// door the push-ingest path uses (jobs.PromoteDecisionsToMemory), so the
	// two entry points cannot disagree about what an approval admits.
	if reviewState == "" {
		return // a plain un-review is not a corpus verdict
	}
	proj, perr := s.ContentStore.GetProject(ctx, projectID)
	if perr != nil || proj == nil || proj.DefaultSourceLanguage == "" {
		slog.WarnContext(ctx, "memory promotion skipped: project unresolved",
			"project", projectID, "error", perr)
		return
	}
	slug := ""
	if s.AuthStore != nil && proj.WorkspaceID != "" {
		if ws, werr := s.AuthStore.GetWorkspace(ctx, proj.WorkspaceID); werr == nil && ws != nil {
			slug = ws.Slug
		}
	}
	if slug == "" {
		slog.WarnContext(ctx, "memory promotion skipped: no workspace slug", "project", projectID)
		return
	}
	tm, terr := s.wsStores.getMemory(slug)
	if terr != nil {
		slog.WarnContext(ctx, "memory promotion skipped: workspace memory unavailable",
			"workspace", slug, "error", terr)
		return
	}
	jobs.PromoteDecisionsToMemory(ctx, s.ContentStore, tm, projectID, stream, proj.DefaultSourceLanguage, []venue.UnitDecision{{
		ItemName:    sb.ItemName,
		Unit:        sb.SourceID,
		Variant:     locale,
		Status:      string(status),
		TargetHash:  state.TargetHash(sb.Block.TargetText(model.LocaleID(locale))),
		ContentHash: state.SourceHash(sb.Block.SourceText()),
		ReviewState: reviewState,
		DecidedBy:   decider,
	}})
}

// This file gives the two real-time editor operations that used to travel over
// the desktop's bespoke gRPC EditorService a REST home on the same routes the
// web app already uses:
//
//   - PUT  /:ws/:id/blocks/:ref/:bid/review — set the per-locale review status
//     on the block's target (Target.Status: reviewed / translated). Distinct
//     from the governance workflow lifecycle (draft/in_review/published) at
//     .../status.
//   - POST /:ws/:id/presence — report the caller's editing focus; published to
//     the event bus and fanned out to watchers over the /:ws/events SSE relay.

// ReviewBlockRequest sets or clears the reviewed status on one block target.
//
// Status optionally selects the rung a clearing request (reviewed=false)
// demotes the target to: "translated" (the default — a plain un-review) or
// "draft" (a reviewer REJECTION — the unit re-enters the work queue, the same
// mapping the host review service uses for ReviewDecisionRejected). It is only
// valid with reviewed=false; approval always lands on "reviewed".
type ReviewBlockRequest struct {
	TargetLocale string `json:"target_locale"`
	ItemName     string `json:"item_name,omitempty"`
	Reviewed     bool   `json:"reviewed"`
	Status       string `json:"status,omitempty"`
}

// legacyTranslationStatusProperty is the pre-per-locale review flag: a
// block-GLOBAL property the old editor endpoint wrote. It is write-never now —
// review state lives on the per-locale Target.Status (the framework ladder that
// convergence/coverage and ship gates consume) — but blocks written before the
// change still carry it, so readers keep it as a fallback and the un-review
// path clears it when there is no target to demote (see HandleReviewBlock).
const legacyTranslationStatusProperty = "translation-status"

// HandleReviewBlock sets the review status of a block's target for ONE locale:
// reviewed=true moves the target to model.TargetStatusReviewed, reviewed=false
// back to model.TargetStatusTranslated — or, with status:"draft", down to
// model.TargetStatusDraft (a reviewer REJECTION: the unit re-enters the work
// queue, matching host/convergereport.go's ReviewDecisionRejected → draft
// mapping). The status lives on
// Block.Targets[Variant(locale)].Status — the framework target ladder
// (draft → translated → reviewed → signed-off) that convergence/coverage and
// ship gates consume — so reviewing French never touches German. The legacy
// block-global Properties["translation-status"] is no longer written.
//
// It is deliberately separate from HandleSetBlockStatus, which drives the
// governance workflow lifecycle (draft → in_review → published, PG-only,
// four-eyes on publish). Marking review status is part of the translation
// workflow, so it requires PermTranslate (the same gate as editing the target),
// not the privileged review/publish permission — with one exception: a target
// at TargetStatusSignedOff (the rung ABOVE reviewed) is protected. Re-approving
// it is an idempotent no-op that keeps signed-off, and demoting it
// (reviewed=false) requires the elevated PermReview, so a translator's ordinary
// un-review click cannot silently undo a sign-off (and the ship gates keyed on
// "at least signed-off" coverage) two rungs down to translated.
//
// No-target decision (documented per epic 006 task 3): approving a block that
// has no non-empty translation for the locale is a 422. The visual editor lets
// a reviewer step onto untranslated blocks (they render with source fallback),
// but "reviewed" is a rung on the target ladder — convergence.TargetState
// counts a unit at its Target.Status only when a non-empty target exists, and
// the host review service (host.ApplyReviewDecision) refuses to approve empty
// translations for the same reason. Persisting an approval that coverage could
// never count would silently recreate the split vocabulary this endpoint
// removes, and an empty Target row would inflate the dashboard's translated
// counts. Un-reviewing a locale with no target is an idempotent no-op success;
// if the block carries the legacy block-global property (old scheme), that
// property is cleared so a legacy "reviewed" block can actually be un-reviewed
// (the legacy flag was block-global, so this is the only faithful reading).
//
// PUT /:ws/:id/blocks/:ref/:bid/review  { "target_locale": "fr", "reviewed": true }
func (s *Server) HandleReviewBlock(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermTranslate); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")
	stream := streamParam(c)

	var req ReviewBlockRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.TargetLocale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "target_locale is required"})
	}
	// The optional status picks the demotion rung for reviewed=false: a plain
	// un-review lands on translated, a reviewer rejection on draft (re-enters
	// the work queue, mirroring host's ReviewDecisionRejected mapping).
	demoteTo := model.TargetStatusTranslated
	switch req.Status {
	case "", string(model.TargetStatusTranslated):
	case string(model.TargetStatusDraft):
		demoteTo = model.TargetStatusDraft
	default:
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: `status must be "translated" or "draft"`})
	}
	if req.Reviewed && req.Status != "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "status only applies when reviewed is false (approval always lands on reviewed)"})
	}
	if err := s.requireLanguagePermission(c, platauth.PermTranslate, req.TargetLocale); err != nil {
		return err
	}

	ctx := c.Request().Context()
	out, err := s.applyBlockReview(ctx, c, blockReviewInput{
		ProjectID: pid, Stream: stream, BlockID: bid, Request: req, DemoteTo: demoteTo,
		Elevate: func() error { return s.requireLanguagePermission(c, platauth.PermReview, req.TargetLocale) },
	})
	if err != nil {
		if fault, ok := errors.AsType[reviewFault](err); ok {
			return c.JSON(fault.code, ErrorResponse{Error: fault.msg})
		}
		if errors.Is(err, errAccessDenied) {
			return err // Elevate already wrote the 403
		}
		return serverErr(c, err)
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)

	// Governed review continuation (RV-B): when this approval leaves the project
	// with zero blocks pending review for any configured locale, hand off to a
	// completing convergence run so the approved content ships without a second
	// user action. advanceReviewLoop is a no-op for non-governed projects, so the
	// per-block review response above is unchanged for them. Only a real approval
	// transition advances the loop; an un-review/rejection re-opens work and an
	// idempotent re-approve of already-reviewed content advances nothing.
	if out.Approval {
		if proj, perr := s.ContentStore.GetProject(ctx, pid); perr == nil {
			actor, _ := c.Get("user_id").(string)
			s.advanceReviewLoop(ctx, proj, stream, []model.LocaleID{model.LocaleID(req.TargetLocale)}, actor)
		}
	}

	resp := map[string]any{
		"ok": true, "block_id": bid, "target_locale": req.TargetLocale, "reviewed": req.Reviewed,
	}
	if out.HadTarget {
		resp["status"] = string(out.Status)
	}
	return c.JSON(http.StatusOK, resp)
}

// reviewFault is a per-block refusal that the single-block route answers with
// its own HTTP status and the bulk route records against the block.
type reviewFault struct {
	code int
	msg  string
}

func (e reviewFault) Error() string { return e.msg }

// blockReviewInput is one block's review, as both the single-block route and
// the bulk route pose it. Elevate is called before demoting a signed-off
// target: the single-block route hands over the standard language-permission
// gate (which writes its own 403), the bulk route a plain predicate so one
// protected block cannot answer for the whole batch.
type blockReviewInput struct {
	ProjectID string
	Stream    string
	BlockID   string
	Request   ReviewBlockRequest
	DemoteTo  model.TargetStatus
	Elevate   func() error
}

// blockReviewOutcome reports what the review did to one block.
type blockReviewOutcome struct {
	// HadTarget is false when the locale had no target at all — an
	// un-review with nothing to demote.
	HadTarget bool
	// Status is the rung the target now holds.
	Status model.TargetStatus
	// Approval is true when the call moved the target UP to reviewed from
	// below: the signal the governed review continuation keys on, so an
	// idempotent re-approve advances nothing.
	Approval bool
}

// applyBlockReview moves one block's target for one locale to the requested
// rung: the status transition, the demotion rules, the decision-ledger write
// and the change event. It is the whole of the review semantics; both review
// routes go through it so they cannot drift apart. The caller owns the
// dashboard-cache invalidation and the review-loop continuation, which are
// per-request rather than per-block.
func (s *Server) applyBlockReview(ctx context.Context, c echo.Context, in blockReviewInput) (blockReviewOutcome, error) {
	req := in.Request
	sb, err := s.ContentStore.GetBlock(ctx, in.ProjectID, in.Stream, in.BlockID)
	if err != nil {
		return blockReviewOutcome{}, reviewFault{http.StatusNotFound, "block not found: " + err.Error()}
	}

	loc := model.LocaleID(req.TargetLocale)
	target := sb.Block.Target(loc) // locale-only variant (tone/channel empty)

	var status model.TargetStatus
	if req.Reviewed {
		if target == nil || strings.TrimSpace(sb.Block.TargetText(loc)) == "" {
			return blockReviewOutcome{}, reviewFault{http.StatusUnprocessableEntity, fmt.Sprintf(
				"block %q has no %s translation to review: translate it first (an untranslated block falls back to source, which is not a reviewable translation)",
				in.BlockID, req.TargetLocale)}
		}
		if target.Status == model.TargetStatusSignedOff {
			// Signed-off sits above reviewed on the ladder; re-approving must
			// not demote it. Idempotent success, keeping the higher rung.
			return blockReviewOutcome{HadTarget: true, Status: target.Status}, nil
		}
		status = model.TargetStatusReviewed
	} else {
		if target == nil {
			// Nothing to demote. Clear the legacy block-global flag if present so
			// a block reviewed under the old scheme can be un-reviewed at all.
			if _, ok := sb.Block.Properties[legacyTranslationStatusProperty]; ok {
				delete(sb.Block.Properties, legacyTranslationStatusProperty)
				if err := s.ContentStore.StoreBlocks(ctx, in.ProjectID, in.Stream, []*model.Block{sb.Block}); err != nil {
					return blockReviewOutcome{}, fmt.Errorf("store block: %w", err)
				}
				s.emitEditorBlockChange(c, in.ProjectID, in.BlockID, req.ItemName, in.Stream, "updated")
			}
			return blockReviewOutcome{}, nil
		}
		if target.Status == model.TargetStatusSignedOff {
			// Undoing a sign-off is a review-level action, not ordinary
			// translation work: without this gate a PermTranslate caller could
			// drop a signed-off target two rungs to translated with no audit
			// trail distinct from an ordinary un-review.
			if err := in.Elevate(); err != nil {
				return blockReviewOutcome{}, err
			}
		}
		status = in.DemoteTo
	}
	approval := req.Reviewed && status == model.TargetStatusReviewed &&
		target.Status.Rank() < model.TargetStatusReviewed.Rank()
	target.Status = status

	if err := s.ContentStore.StoreBlocks(ctx, in.ProjectID, in.Stream, []*model.Block{sb.Block}); err != nil {
		return blockReviewOutcome{}, fmt.Errorf("store block: %w", err)
	}

	// The review is a DECISION, and decisions live in the ledger — with the
	// decider's identity, the time, and the hash of the translation it
	// blesses — not only in the projected status the line above wrote. The
	// ledger is what travels to the client on pull, where the same record
	// lands in the project's committed state.
	s.recordReviewDecision(ctx, c, in.ProjectID, in.Stream, sb, req.TargetLocale, status, req.Reviewed)
	s.emitEditorBlockChange(c, in.ProjectID, in.BlockID, req.ItemName, in.Stream, "updated")

	return blockReviewOutcome{HadTarget: true, Status: status, Approval: approval}, nil
}

// PresenceRequest reports the caller's current editing focus in a project.
type PresenceRequest struct {
	ItemName string `json:"item_name,omitempty"`
	BlockID  string `json:"block_id,omitempty"`
}

// HandleUpdatePresence records the caller's editing focus and publishes an
// "editor.presence.moved" event to the bus. The change relay fans it out to
// every watcher subscribed to the project over the /:ws/events SSE stream. Real
// per-cursor presence is rendered from the Yjs awareness channel; this endpoint
// carries the coarse "who is looking at which item/block" signal used by the
// backend-mediated presence indicators.
//
// POST /:ws/:id/presence  { "item_name": "file.html", "block_id": "b-1" }
func (s *Server) HandleUpdatePresence(c echo.Context) error {
	if s.EventBus == nil {
		return c.NoContent(http.StatusNoContent) // presence is best-effort
	}

	pid := projectParam(c)
	var req PresenceRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	userID, _ := c.Get("user_id").(string)
	userName, _ := c.Get("name").(string)
	avatarURL := ""
	if s.AuthStore != nil && userID != "" {
		if u, err := s.AuthStore.GetUser(c.Request().Context(), userID); err == nil && u != nil {
			avatarURL = u.AvatarURL
		}
	}

	s.EventBus.Publish(platev.Event{
		ID:        id.New(),
		Type:      platev.EventType("editor.presence.moved"),
		Source:    "editor-rest",
		ProjectID: pid,
		Actor:     userID,
		Data: map[string]string{
			"event_kind": "presence",
			"user_id":    userID,
			"user_name":  userName,
			"avatar_url": avatarURL,
			"item_name":  req.ItemName,
			"block_id":   req.BlockID,
		},
		Timestamp: time.Now(),
	})
	return c.NoContent(http.StatusNoContent)
}

// emitEditorBlockChange publishes an editor.block.<changeType> event so watchers
// refresh the affected block. Mirrors the event the gRPC editor used to emit.
func (s *Server) emitEditorBlockChange(c echo.Context, projectID, blockID, itemName, stream, changeType string) {
	userName, _ := c.Get("name").(string)
	userID, _ := c.Get("user_id").(string)
	s.publishEditorBlockChange(projectID, blockID, itemName, stream, changeType, userID, userName)
}

// publishEditorBlockChange publishes the "editor.block.<changeType>" SSE-fanout
// event without an echo.Context, so background callers (e.g. the RV-E review
// re-check, which runs off the event bus with no request) can refresh watchers'
// views after mutating a block. actorName is the human display name (empty for a
// system actor).
func (s *Server) publishEditorBlockChange(projectID, blockID, itemName, stream, changeType, actor, actorName string) {
	if s.EventBus == nil {
		return
	}
	s.EventBus.Publish(platev.Event{
		ID:        id.New(),
		Type:      platev.EventType("editor.block." + changeType),
		Source:    "editor-rest",
		ProjectID: projectID,
		Actor:     actor,
		Data: map[string]string{
			"block_id":    blockID,
			"item_name":   itemName,
			"stream":      stream,
			"change_type": changeType,
			"changed_by":  actorName,
		},
		Timestamp: time.Now(),
	})
}
