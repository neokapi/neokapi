package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
)

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
	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, bid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found: " + err.Error()})
	}

	loc := model.LocaleID(req.TargetLocale)
	target := sb.Block.Target(loc) // locale-only variant (tone/channel empty)

	var status model.TargetStatus
	if req.Reviewed {
		if target == nil || strings.TrimSpace(sb.Block.TargetText(loc)) == "" {
			return c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
				Error: fmt.Sprintf("block %q has no %s translation to review: translate it first (an untranslated block falls back to source, which is not a reviewable translation)", bid, req.TargetLocale),
			})
		}
		if target.Status == model.TargetStatusSignedOff {
			// Signed-off sits above reviewed on the ladder; re-approving must
			// not demote it. Idempotent success, keeping the higher rung.
			return c.JSON(http.StatusOK, map[string]any{
				"ok": true, "block_id": bid, "target_locale": req.TargetLocale,
				"reviewed": true, "status": string(target.Status),
			})
		}
		status = model.TargetStatusReviewed
	} else {
		if target == nil {
			// Nothing to demote. Clear the legacy block-global flag if present so
			// a block reviewed under the old scheme can be un-reviewed at all.
			if _, ok := sb.Block.Properties[legacyTranslationStatusProperty]; ok {
				delete(sb.Block.Properties, legacyTranslationStatusProperty)
				if err := s.ContentStore.StoreBlocks(ctx, pid, stream, []*model.Block{sb.Block}); err != nil {
					return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "store block: " + err.Error()})
				}
				s.emitEditorBlockChange(c, pid, bid, req.ItemName, stream, "updated")
				wsID, _ := c.Get("workspace_id").(string)
				s.invalidateDashboardCache(wsID, pid)
			}
			return c.JSON(http.StatusOK, map[string]any{"ok": true, "block_id": bid, "target_locale": req.TargetLocale, "reviewed": false})
		}
		if target.Status == model.TargetStatusSignedOff {
			// Undoing a sign-off is a review-level action, not ordinary
			// translation work: without this gate a PermTranslate caller could
			// drop a signed-off target two rungs to translated with no audit
			// trail distinct from an ordinary un-review.
			if err := s.requireLanguagePermission(c, platauth.PermReview, req.TargetLocale); err != nil {
				return err
			}
		}
		status = demoteTo
	}
	target.Status = status

	if err := s.ContentStore.StoreBlocks(ctx, pid, stream, []*model.Block{sb.Block}); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "store block: " + err.Error()})
	}

	s.emitEditorBlockChange(c, pid, bid, req.ItemName, stream, "updated")

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)
	return c.JSON(http.StatusOK, map[string]any{
		"ok": true, "block_id": bid, "target_locale": req.TargetLocale,
		"reviewed": req.Reviewed, "status": string(status),
	})
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
	if s.EventBus == nil {
		return
	}
	userName, _ := c.Get("name").(string)
	userID, _ := c.Get("user_id").(string)
	s.EventBus.Publish(platev.Event{
		ID:        id.New(),
		Type:      platev.EventType("editor.block." + changeType),
		Source:    "editor-rest",
		ProjectID: projectID,
		Actor:     userID,
		Data: map[string]string{
			"block_id":    blockID,
			"item_name":   itemName,
			"stream":      stream,
			"change_type": changeType,
			"changed_by":  userName,
		},
		Timestamp: time.Now(),
	})
}
