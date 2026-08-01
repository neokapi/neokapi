package server

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// requireEditableStatus is the ABAC predicate gating edits by a block's workflow
// status (and ownership). It runs in addition to the base translate permission:
//
//   - draft     → no extra requirement (normal perms apply)
//   - in_review → requires PermReview for the locale, unless the actor owns the
//     block (an owner may keep working their own in-review content)
//   - published → editing requires PermManageProject (re-opening published
//     content is privileged)
//
// Returns a non-nil error (after writing the 403) when the edit is not allowed.
func (s *Server) requireEditableStatus(c echo.Context, projectID, blockID, locale string) error {
	pg, ok := s.ContentStore.(*bstore.PostgresStore)
	if !ok {
		return nil // status ABAC only enforced on the PostgreSQL store
	}
	status, owner, err := pg.GetBlockStatus(c.Request().Context(), projectID, blockID)
	if err != nil {
		return nil // don't block edits on a status-lookup error
	}
	switch status {
	case bstore.BlockStatusPublished:
		return s.requirePermission(c, platauth.PermManageProject)
	case bstore.BlockStatusInReview:
		if actor, _ := c.Get("user_id").(string); owner != "" && owner == actor {
			return nil
		}
		return s.requireLanguagePermission(c, platauth.PermReview, locale)
	default:
		return nil
	}
}

// BlockStatusRequest sets a block's workflow status and optional owner. Reason
// captures a structured rejection/send-back note when moving content back to a
// lower status.
type BlockStatusRequest struct {
	Status  string `json:"status"`
	OwnerID string `json:"owner_id,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// HandleSetBlockStatus changes a block's workflow status. Moving content to/from
// in_review or publishing requires PermReview; un-publishing (published →
// draft/in_review) requires PermManageProject.
//
// PUT /:ws/:id/blocks/:ref/:bid/status  { "status": "published" }
func (s *Server) HandleSetBlockStatus(c echo.Context) error {
	pg, ok := s.ContentStore.(*bstore.PostgresStore)
	if !ok || pg == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "status workflow requires the PostgreSQL store"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")
	var req BlockStatusRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if !bstore.ValidBlockStatuses[req.Status] {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "status must be draft, in_review, or published"})
	}

	ctx := c.Request().Context()
	// The current status decides which permission is required, so a failed read
	// must not be answered with the zero value: "" is not published, which
	// silently swaps the privileged un-publish gate below for the review one.
	// The store already separates the cases — a missing row comes back as
	// draft with a nil error — so an error here is a real fault, and the gate
	// fails closed on it.
	cur, _, err := pg.GetBlockStatus(ctx, pid, bid)
	if err != nil {
		return serverErr(c, fmt.Errorf("read block status for the permission gate: %w", err))
	}

	// Un-publishing is privileged; other workflow transitions are review actions.
	if cur == bstore.BlockStatusPublished && req.Status != bstore.BlockStatusPublished {
		if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
			return err
		}
	} else {
		if err := s.requirePermission(c, platauth.PermReview); err != nil {
			return err
		}
	}

	// Separation of duties: approving (publishing) is a four-eyes step — the
	// approver must not be the translator who authored the content.
	if req.Status == bstore.BlockStatusPublished {
		// Same shape, sharper consequence: enforceSoD reads an empty author as
		// "unknown — no conflict of interest" and allows the approval, so a
		// discarded error here disables the four-eyes check rather than
		// tightening it. The store returns ("", nil) when there genuinely is no
		// attributed history; anything else is a fault worth refusing on.
		author, err := pg.GetLastEditor(ctx, pid, bid)
		if err != nil {
			return serverErr(c, fmt.Errorf("read last editor for separation of duties: %w", err))
		}
		actor, _ := c.Get("user_id").(string)
		if err := s.enforceSoD(c, actor, author, "publish_block:"+bid); err != nil {
			return err
		}
	}

	if err := pg.SetBlockStatus(ctx, pid, bid, req.Status, req.OwnerID); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	data := map[string]string{}
	if req.Reason != "" {
		data["reason"] = req.Reason
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventType("content.status_changed"),
		ProjectID:    pid,
		ResourceType: "block",
		ResourceID:   bid,
		Data:         data,
		Before:       map[string]string{"status": cur},
		After:        map[string]string{"status": req.Status},
	})

	return c.JSON(http.StatusOK, map[string]any{"ok": true, "block_id": bid, "status": req.Status})
}
