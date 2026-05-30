package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/model"
)

// RollbackBlockRequest restores a block's target for a locale to a prior
// version recorded in block_history.
type RollbackBlockRequest struct {
	Locale string `json:"locale"`
	ToSeq  int64  `json:"to_seq"` // block_history entry id to restore to
}

// HandleRollbackBlock restores a block's target (for one locale) to a prior
// version from its history. The restore is non-destructive: it writes the
// historical content as a NEW edit (which itself appends a history entry), so a
// rollback can itself be rolled back. Requires PermRollbackChanges, and is
// language-scoped.
//
// POST /:ws/:id/blocks/:ref/:bid/rollback  { "locale": "fr", "to_seq": 12 }
func (s *Server) HandleRollbackBlock(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermRollbackChanges); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")
	stream := streamParam(c)

	var req RollbackBlockRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Locale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale is required"})
	}
	// Rolling back a translation is a language-scoped action.
	if err := s.requireLanguagePermission(c, platauth.PermRollbackChanges, req.Locale); err != nil {
		return err
	}

	ctx := c.Request().Context()
	history, err := s.ContentStore.GetBlockHistory(ctx, pid, stream, bid, req.Locale, 200)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	var entry *struct {
		Text  string
		Coded string
	}
	for _, h := range history {
		if h.Seq == req.ToSeq {
			entry = &struct {
				Text  string
				Coded string
			}{Text: h.Text, Coded: h.Coded}
			break
		}
	}
	if entry == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "history entry not found for this block/locale"})
	}

	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, bid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
	}

	locale := model.LocaleID(req.Locale)
	// Prefer restoring the full Run sequence (preserves inline markup); fall
	// back to plain text.
	if entry.Coded != "" {
		var runs []model.Run
		if json.Unmarshal([]byte(entry.Coded), &runs) == nil && len(runs) > 0 {
			sb.Block.SetTargetRuns(locale, runs)
		} else {
			sb.Block.SetTargetText(locale, entry.Text)
		}
	} else {
		sb.Block.SetTargetText(locale, entry.Text)
	}

	if err := s.ContentStore.StoreBlocks(ctx, pid, stream, []*model.Block{sb.Block}); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	s.emitAudit(c, auditEvent{
		Type:         platev.EventRollbackPerformed,
		ProjectID:    pid,
		ResourceType: "block",
		ResourceID:   bid,
		Data: map[string]string{
			"locale": req.Locale,
			"to_seq": strconv.FormatInt(req.ToSeq, 10),
			"stream": stream,
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"ok":           true,
		"block_id":     bid,
		"locale":       req.Locale,
		"restored_seq": req.ToSeq,
	})
}
