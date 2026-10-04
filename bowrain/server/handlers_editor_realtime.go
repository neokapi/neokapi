package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// recordReviewDecision appends a server-made review to the decision ledger
// with the decider's identity, the time, and the revision of the translation
// it blesses, then promotes the wording into the workspace content memory. It
// writes through ledger, or opens one for the single decision it writes when
// ledger is nil.
//
// A verdict that is not an approval carries the row's existing basis forward
// rather than stamping the source and context in front of it, so the one row
// this call reads is the row it is about to replace.
func (s *Server) recordReviewDecision(ctx context.Context, c echo.Context, ledger *reviewLedger, projectID, stream string, sb *venue.StoredBlock, locale string, status model.TargetStatus, approved bool) {
	if sb == nil || sb.SourceID == "" {
		return
	}
	if ledger == nil {
		ledger = s.newReviewLedger(ctx, c, projectID, stream)
	}
	if ledger == nil {
		return
	}
	var prev *venue.UnitDecision
	if !approved {
		prev = s.unitDecisionFor(ctx, projectID, stream, sb, locale)
	}
	governing := ledger.governingFingerprint(ctx, sb.ItemName, locale)
	ledger.write(ctx, []venue.UnitDecision{unitDecisionFor(sb, locale, status, approved, ledger.decider, governing, prev)})
	if status == model.TargetStatusDraft {
		ledger.clearDraftBasis(ctx, sb, locale)
	}
}

// ReviewBlockRequest is one review decision on one block's translation for one
// locale, as the change service's decide operation applies it
// (streamDecisions): Reviewed lands the translation on established; otherwise
// Status "draft" is a rejection, which re-enters the work queue, and empty or
// "translated" withdraws an approval. BaseRevision is the revision of the
// translation the decision was made on; the decision is refused when the
// translation has moved since.
type ReviewBlockRequest struct {
	TargetLocale string
	ItemName     string
	Reviewed     bool
	Status       string
	BaseRevision string
}

// legacyTranslationStatusProperty is the pre-per-locale review flag: a
// block-global property an earlier review route wrote. Review state lives on
// the per-locale Edition.Status, but blocks written before the change still
// carry it, so a withdrawal clears it when there is no target to demote.
const legacyTranslationStatusProperty = "translation-status"

// reviewFault is the refusal of one decision with the HTTP status its cause
// maps to; decisionRefusal turns it into the change contract's code.
type reviewFault struct {
	code int
	msg  string
}

func (e reviewFault) Error() string { return e.msg }

// blockReviewInput is one block's review, as a decide operation poses it
// (streamDecisions). Elevate is called before demoting an established target,
// and Vet, the separation-of-duties gate, before an approval that promotes
// one; each answers with an error that refuses the decision. The change
// service asks both when it prepares the decision, before anything is written.
type blockReviewInput struct {
	ProjectID string
	Stream    string
	BlockID   string
	Request   ReviewBlockRequest
	DemoteTo  model.TargetStatus
	// PromoteTo is the rung an approving call lands on. Empty means
	// established, the only rung a person's approval reaches.
	PromoteTo model.TargetStatus
	Elevate   func() error
	Vet       func(blockID, locale string) error
	// Ledger, when set, is the decision ledger a pass of many decisions writes
	// through; nil opens one for the decision.
	Ledger *reviewLedger
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
	// of an established target, or an un-review with nothing to demote. Nothing
	// happened, so nothing is audited.
	Changed bool
}

// applyBlockReview moves one block's target for one locale to the requested
// rung: the status transition, the demotion rules, the decision-ledger write
// and the change event. It is the whole of the review semantics: every decide
// operation a stream applies goes through it (streamDecisions). The caller
// owns the dashboard-cache invalidation and the review-loop continuation,
// which are per change set rather than per block.
func (s *Server) applyBlockReview(ctx context.Context, c echo.Context, in blockReviewInput) (blockReviewOutcome, error) {
	req := in.Request
	loc := model.LocaleID(req.TargetLocale)

	promoteTo := in.PromoteTo
	if promoteTo == "" {
		promoteTo = model.TargetStatusEstablished
	}

	// The decision is made on the block as the write holds it, so the status
	// lands on the wording that was judged. out is what the decision leaves.
	var out blockReviewOutcome
	clearedLegacy := false
	sb, err := s.ContentStore.UpdateBlock(ctx, in.ProjectID, in.Stream, in.BlockID, func(sb *venue.StoredBlock) error {
		if err := checkBaseRevision(sb, loc, req.BaseRevision); err != nil {
			return err
		}
		target, held := sb.Block.TargetEdition(loc) // locale-only variant (tone/channel empty)
		from := model.TargetStatus(target.Status)

		var status model.TargetStatus
		if req.Reviewed {
			if !held || strings.TrimSpace(sb.Block.TargetText(loc)) == "" {
				return reviewFault{http.StatusUnprocessableEntity, fmt.Sprintf(
					"block %q has no %s translation to review: translate it first (an untranslated block falls back to source, which is not a reviewable translation)",
					in.BlockID, req.TargetLocale)}
			}
			if from == model.TargetStatusEstablished {
				// Established is the top of the ladder; approving it again
				// must not demote it. Idempotent success, keeping the rung.
				out = blockReviewOutcome{HadTarget: true, From: from, Status: from}
				return errNothingToWrite
			}
			// Separation of duties applies to a real promotion. A call that lands
			// on a rung the target already holds moves nothing, so there is no
			// decision to refuse: re-approving a reviewed target passes, while
			// signing one off is a fresh decision and is vetted.
			if in.Vet != nil && from.Rank() < promoteTo.Rank() {
				if err := in.Vet(in.BlockID, req.TargetLocale); err != nil {
					return err
				}
			}
			status = promoteTo
		} else {
			if !held {
				// Nothing to demote. Clear the legacy block-global flag if present so
				// a block reviewed under the old scheme can be un-reviewed at all.
				if _, ok := sb.Block.Properties[legacyTranslationStatusProperty]; !ok {
					return errNothingToWrite
				}
				delete(sb.Block.Properties, legacyTranslationStatusProperty)
				clearedLegacy = true
				return nil
			}
			if from == model.TargetStatusEstablished {
				// Undoing an established unit is a review-level action, not ordinary
				// translation work: without this gate a PermTranslate caller could
				// drop an established target two rungs to translated with no audit
				// trail distinct from an ordinary un-review.
				if err := in.Elevate(); err != nil {
					return err
				}
			}
			status = in.DemoteTo
		}
		// An approval counts for the review loop: it
		// leaves the project one pending unit lighter. Signing off a target that
		// was already reviewed leaves the pending count where it was, so it does
		// not advance the loop, the same way a re-approve does not.
		approval := req.Reviewed && status.Rank() >= model.TargetStatusEstablished.Rank() &&
			from.Rank() < model.TargetStatusEstablished.Rank()
		target.Status = model.Status(status)
		sb.Block.SetTargetEdition(model.Variant(loc), target)
		out = blockReviewOutcome{HadTarget: true, From: from, Status: status, Approval: approval, Changed: from != status}
		return nil
	})
	switch {
	case nothingToWrite(err):
		return out, nil
	case err != nil && sb == nil:
		return blockReviewOutcome{}, reviewFault{http.StatusNotFound, "block not found: " + err.Error()}
	case err != nil:
		return blockReviewOutcome{}, err
	}
	if clearedLegacy {
		s.emitEditorBlockChange(c, in.ProjectID, in.BlockID, req.ItemName, in.Stream, "updated")
		return blockReviewOutcome{}, nil
	}

	// The review is a DECISION, and decisions live in the ledger — with the
	// decider's identity, the time, and the revision of the translation it
	// blesses — not only in the projected status the write above landed. The
	// ledger is what travels to the client on pull, where the same record
	// lands in the project's committed state.
	s.recordReviewDecision(ctx, c, in.Ledger, in.ProjectID, in.Stream, sb, req.TargetLocale, out.Status, req.Reviewed)
	s.emitEditorBlockChange(c, in.ProjectID, in.BlockID, req.ItemName, in.Stream, "updated")

	return out, nil
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
