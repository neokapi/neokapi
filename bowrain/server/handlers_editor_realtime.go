package server

import (
	"net/http"
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
//   - PUT  /:ws/:id/blocks/:ref/:bid/review — set a block's translation-status
//     QA flag (reviewed / translated). Distinct from the governance workflow
//     lifecycle (draft/in_review/published) at .../status.
//   - POST /:ws/:id/presence — report the caller's editing focus; published to
//     the event bus and fanned out to watchers over the /:ws/events SSE relay.

// ReviewBlockRequest sets or clears a block target's reviewed QA flag.
type ReviewBlockRequest struct {
	TargetLocale string `json:"target_locale"`
	ItemName     string `json:"item_name,omitempty"`
	Reviewed     bool   `json:"reviewed"`
}

// HandleReviewBlock sets a block's translation-status property to "reviewed" or
// "translated". This is the QA/coverage flag surfaced in the editor's review
// checkbox — it is deliberately separate from HandleSetBlockStatus, which drives
// the governance workflow lifecycle (draft → in_review → published, PG-only,
// four-eyes on publish). Marking translation-status is part of the translation
// workflow, so it requires PermTranslate (the same gate as editing the target),
// not the privileged review/publish permission.
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
	if err := s.requireLanguagePermission(c, platauth.PermTranslate, req.TargetLocale); err != nil {
		return err
	}

	ctx := c.Request().Context()
	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, bid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found: " + err.Error()})
	}

	if sb.Block.Properties == nil {
		sb.Block.Properties = make(map[string]string)
	}
	if req.Reviewed {
		sb.Block.Properties["translation-status"] = "reviewed"
	} else {
		sb.Block.Properties["translation-status"] = "translated"
	}

	if err := s.ContentStore.StoreBlocks(ctx, pid, stream, []*model.Block{sb.Block}); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "store block: " + err.Error()})
	}

	s.emitEditorBlockChange(c, pid, bid, req.ItemName, stream, "updated")

	wsID, _ := c.Get("workspace_id").(string)
	s.invalidateDashboardCache(wsID, pid)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "block_id": bid, "reviewed": req.Reviewed})
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
