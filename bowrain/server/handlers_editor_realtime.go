package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// recordReviewDecision appends a server-made review to the decision ledger
// with the decider's identity, the time, and the hash of the translation it
// blesses, then promotes the wording into the workspace content memory. It
// opens a ledger for the single decision it writes; a pass over many blocks
// opens one and writes batches through it instead.
func (s *Server) recordReviewDecision(ctx context.Context, c echo.Context, projectID, stream string, sb *venue.StoredBlock, locale string, status model.TargetStatus, approved bool) {
	if sb == nil || sb.SourceID == "" {
		return
	}
	ledger := s.newReviewLedger(ctx, c, projectID, stream)
	if ledger == nil {
		return
	}
	ledger.write(ctx, []venue.UnitDecision{unitDecisionFor(sb, locale, status, approved, ledger.decider)})
}

// This file gives the two real-time editor operations that used to travel over
// the desktop's bespoke gRPC EditorService a REST home on the same routes the
// web app already uses:
//
//   - PUT  /:ws/:id/blocks/:ref/:bid/review — set the per-locale review status
//     on the block's target (Target.Status: signed-off / reviewed /
//     translated / draft). Distinct from the governance workflow lifecycle
//     (draft/in_review/published) at .../status.
//   - POST /:ws/:id/presence — report the caller's editing focus; published to
//     the event bus and fanned out to watchers over the /:ws/events SSE relay.

// ReviewBlockRequest sets or clears the reviewed status on one block target.
//
// Status optionally selects the rung the call lands on, and which rungs are
// available depends on the direction. A clearing request (reviewed=false)
// demotes to "translated" (the default, a plain un-review) or to "draft" (a
// reviewer REJECTION, so the unit re-enters the work queue, the same mapping
// the host review service uses for ReviewDecisionRejected). An approving
// request (reviewed=true) lands on "reviewed" by default, or on "signed-off",
// the rung above it, when the reviewer signs the target off. Any other pairing
// is a 400.
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
// reviewed=true moves the target to model.TargetStatusReviewed, or with
// status:"signed-off" to model.TargetStatusSignedOff, the rung above it;
// reviewed=false moves it back to model.TargetStatusTranslated — or, with
// status:"draft", down to model.TargetStatusDraft (a reviewer REJECTION: the
// unit re-enters the work queue, matching host/convergereport.go's
// ReviewDecisionRejected → draft mapping). The status lives on
// Block.Targets[Variant(locale)].Status — the framework target ladder
// (draft → translated → reviewed → signed-off) that convergence/coverage and
// ship gates consume — so reviewing French never touches German. The legacy
// block-global Properties["translation-status"] is no longer written.
//
// It is deliberately separate from HandleSetBlockStatus, which drives the
// governance workflow lifecycle (draft → in_review → published, PG-only,
// four-eyes on publish).
//
// Approving and signing off are the review permission for the language being
// decided: PermReview, language-scoped. Withdrawing an approval
// (reviewed=false) and rejecting (status:"draft") stay with PermTranslate, the
// same gate that edits the target, so a translator can still take back their
// own work. A target at TargetStatusSignedOff (the top of the ladder) is
// protected either way: approving or re-signing it is an idempotent no-op that
// keeps signed-off, and demoting it requires PermReview, so a translator's
// ordinary un-review click cannot silently undo a sign-off (and the ship gates
// keyed on "at least signed-off" coverage) two rungs down to translated.
//
// Every promotion also passes the workspace separation-of-duties policy:
// whoever last wrote the translation by hand may not be the one who approves or
// signs it off, unless the workspace has the policy off or set to warn. A
// target a run produced has no human author and stays approvable by one person.
// Signing off a target already at reviewed is a fresh decision and is vetted
// again; the policy draws no second line between the approver and the signer.
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
	// The optional status picks the rung, and each direction has its own two.
	// Clearing lands on translated (a plain un-review) or draft (a reviewer
	// rejection, which re-enters the work queue, mirroring host's
	// ReviewDecisionRejected mapping). Approving lands on reviewed or, when the
	// reviewer signs the target off, on signed-off.
	demoteTo := model.TargetStatusTranslated
	promoteTo := model.TargetStatusReviewed
	if req.Reviewed {
		switch req.Status {
		case "":
		case string(model.TargetStatusSignedOff):
			promoteTo = model.TargetStatusSignedOff
		default:
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: `status must be "signed-off" or omitted when reviewed is true`})
		}
	} else {
		switch req.Status {
		case "", string(model.TargetStatusTranslated):
		case string(model.TargetStatusDraft):
			demoteTo = model.TargetStatusDraft
		default:
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: `status must be "translated" or "draft" when reviewed is false`})
		}
	}
	// Approving and signing off are both the review permission, for the
	// language being decided. Un-reviewing and rejecting stay with the
	// translate permission that edits the target, so a translator can still
	// withdraw their own work.
	if err := s.requireLanguagePermission(c, reviewGateFor(req.Reviewed), req.TargetLocale); err != nil {
		return err
	}

	ctx := c.Request().Context()
	sod, err := s.newReviewSoD(ctx, c, pid, stream, []string{bid}, []string{req.TargetLocale})
	if err != nil {
		return serverErr(c, err)
	}
	out, err := s.applyBlockReview(ctx, c, blockReviewInput{
		ProjectID: pid, Stream: stream, BlockID: bid, Request: req,
		DemoteTo: demoteTo, PromoteTo: promoteTo,
		Elevate: func() error { return s.requireLanguagePermission(c, platauth.PermReview, req.TargetLocale) },
		Vet:     sod.vet,
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

	if out.Changed {
		s.emitReviewDecisionAudit(c, pid, stream, bid, req.TargetLocale, out.From, out.Status, req.Reviewed, "")
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
//
// Vet is the separation-of-duties gate, called before an approval that actually
// promotes a target. It answers with a reviewFault, so one refused block is
// recorded against that block rather than failing a whole selection.
type blockReviewInput struct {
	ProjectID string
	Stream    string
	BlockID   string
	Request   ReviewBlockRequest
	DemoteTo  model.TargetStatus
	// PromoteTo is the rung an approving call lands on: reviewed, or
	// signed-off when the reviewer signs the target off. Empty means reviewed,
	// so a caller that only ever approves says nothing.
	PromoteTo model.TargetStatus
	Elevate   func() error
	Vet       func(blockID, locale string) error
}

// blockReviewOutcome reports what the review did to one block.
type blockReviewOutcome struct {
	// HadTarget is false when the locale had no target at all — an
	// un-review with nothing to demote.
	HadTarget bool
	// From is the rung the target held before the call, for the audit trail.
	From model.TargetStatus
	// Status is the rung the target now holds.
	Status model.TargetStatus
	// Approval is true when the call moved the target UP to reviewed or above
	// from below it: the signal the governed review continuation keys on, so an
	// idempotent re-approve advances nothing and neither does signing off a
	// target that was already reviewed.
	Approval bool
	// Changed is false when the call moved no rung: an idempotent re-approve
	// of a signed-off target, or an un-review with nothing to demote. Nothing
	// happened, so nothing is audited.
	Changed bool
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

	promoteTo := in.PromoteTo
	if promoteTo == "" {
		promoteTo = model.TargetStatusReviewed
	}

	var status model.TargetStatus
	if req.Reviewed {
		if target == nil || strings.TrimSpace(sb.Block.TargetText(loc)) == "" {
			return blockReviewOutcome{}, reviewFault{http.StatusUnprocessableEntity, fmt.Sprintf(
				"block %q has no %s translation to review: translate it first (an untranslated block falls back to source, which is not a reviewable translation)",
				in.BlockID, req.TargetLocale)}
		}
		if target.Status == model.TargetStatusSignedOff {
			// Signed-off is the top of the ladder; approving or re-signing it
			// must not demote it. Idempotent success, keeping the rung.
			return blockReviewOutcome{HadTarget: true, From: target.Status, Status: target.Status}, nil
		}
		// Separation of duties applies to a real promotion. A call that lands
		// on a rung the target already holds moves nothing, so there is no
		// decision to refuse: re-approving a reviewed target passes, while
		// signing one off is a fresh decision and is vetted.
		if in.Vet != nil && target.Status.Rank() < promoteTo.Rank() {
			if err := in.Vet(in.BlockID, req.TargetLocale); err != nil {
				return blockReviewOutcome{}, err
			}
		}
		status = promoteTo
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
	from := target.Status
	// A sign-off counts for the review loop exactly as an approval does: it
	// leaves the project one pending unit lighter. Signing off a target that
	// was already reviewed leaves the pending count where it was, so it does
	// not advance the loop, the same way a re-approve does not.
	approval := req.Reviewed && status.Rank() >= model.TargetStatusReviewed.Rank() &&
		from.Rank() < model.TargetStatusReviewed.Rank()
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

	return blockReviewOutcome{HadTarget: true, From: from, Status: status, Approval: approval, Changed: from != status}, nil
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
