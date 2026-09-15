package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// applyReverts loads each affected block, applies the target reverts (restoring
// prior content, or blanking targets the batch created), and stores them under a
// labeled change context so the revert/restore is itself recorded in history.
// Returns the number of targets reverted.
func (s *Server) applyReverts(ctx context.Context, pid, stream, reason string, reverts []bstore.TargetRevert) (int, error) {
	byBlock := map[string][]bstore.TargetRevert{}
	order := []string{}
	for _, r := range reverts {
		if _, seen := byBlock[r.BlockID]; !seen {
			order = append(order, r.BlockID)
		}
		byBlock[r.BlockID] = append(byBlock[r.BlockID], r)
	}
	reads := make([]*venue.StoredBlock, 0, len(order))
	for _, bid := range order {
		sb, err := s.ContentStore.GetBlock(ctx, pid, stream, bid)
		if err != nil {
			continue // block gone; skip
		}
		for _, r := range byBlock[bid] {
			locale := model.LocaleID(r.Locale)
			switch {
			case r.Clear:
				sb.Block.SetTargetText(locale, "")
			case r.Coded != "":
				var runs []model.Run
				if json.Unmarshal([]byte(r.Coded), &runs) == nil && len(runs) > 0 {
					sb.Block.SetTargetRuns(locale, runs)
				} else {
					sb.Block.SetTargetText(locale, r.Text)
				}
			default:
				sb.Block.SetTargetText(locale, r.Text)
			}
		}
		reads = append(reads, sb)
	}
	if len(reads) == 0 {
		return 0, nil
	}
	// Written back to the rows read above: a block a push removed or rewrote since
	// then keeps what the push left, and its reverts are not counted.
	ctx = bstore.WithChangeContext(ctx, bstore.ChangeContext{Reason: reason})
	res, err := s.ContentStore.WriteBackBlocks(ctx, pid, stream, reads)
	if err != nil {
		return 0, err
	}
	skipped := make(map[string]bool, len(res.Skipped))
	for _, id := range res.Skipped {
		skipped[id] = true
	}
	n := 0
	for _, sb := range reads {
		if !skipped[sb.Block.ID] {
			n += len(byBlock[sb.Block.ID])
		}
	}
	return n, nil
}

// RollbackBlockRequest restores a block's target for a locale to a prior
// version recorded in block_history.
type RollbackBlockRequest struct {
	Locale string `json:"locale"`
	ToSeq  int64  `json:"to_seq"` // block_history entry id to restore to
	// BaseRevision is the target revision the caller read. A rollback that names
	// one is refused with the current block when the target has moved since.
	BaseRevision string `json:"base_revision,omitempty"`
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
		return serverErr(c, err)
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

	locale := model.LocaleID(req.Locale)
	// Label the restoring write so its history entry reads as a rollback. The
	// restore lands on the block as the write holds it, so a block removed since
	// the history was read stays removed.
	ctx = bstore.WithChangeContext(ctx, bstore.ChangeContext{Reason: "rollback:" + strconv.FormatInt(req.ToSeq, 10)})
	updated, uerr := s.ContentStore.UpdateBlock(ctx, pid, stream, bid, func(sb *venue.StoredBlock) error {
		if err := checkBaseRevision(sb, locale, req.BaseRevision); err != nil {
			return err
		}
		// Prefer restoring the full Run sequence (preserves inline markup); fall
		// back to plain text.
		if entry.Coded != "" {
			var runs []model.Run
			if json.Unmarshal([]byte(entry.Coded), &runs) == nil && len(runs) > 0 {
				sb.Block.SetTargetRuns(locale, runs)
				return nil
			}
		}
		sb.Block.SetTargetText(locale, entry.Text)
		return nil
	})
	if uerr != nil {
		if changed, ok := asBlockChanged(uerr); ok {
			return s.answerBlockChanged(c, pid, changed, req.Locale)
		}
		if updated == nil {
			return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
		}
		return serverErr(c, uerr)
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

// RevertBatchRequest reverts every target changed under a correlation id (one
// push / import / batch operation) to its pre-batch value.
type RevertBatchRequest struct {
	CorrelationID string `json:"correlation_id"`
	Stream        string `json:"stream,omitempty"`
}

// HandleRevertBatch reverts a whole batch of content changes (grouped by
// correlation id — e.g. a sync push, an AI-translate-file, or an import) back to
// the state before the batch. Each affected target is restored from history (or
// blanked if the batch first created it). Non-destructive (the revert is
// recorded). Requires PermRollbackChanges.
//
// POST /:ws/:id/revert  { "correlation_id": "...", "stream": "main" }
func (s *Server) HandleRevertBatch(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermRollbackChanges); err != nil {
		return err
	}
	pg, ok := s.ContentStore.(*bstore.PostgresStore)
	if !ok || pg == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "batch revert requires the PostgreSQL store"})
	}

	pid := projectParam(c)
	var req RevertBatchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.CorrelationID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "correlation_id is required"})
	}
	stream := req.Stream
	if stream == "" {
		stream = "main"
	}

	ctx := c.Request().Context()
	reverts, err := pg.ComputeBatchReverts(ctx, pid, stream, req.CorrelationID)
	if err != nil {
		return serverErr(c, err)
	}
	if len(reverts) == 0 {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "no changes found for this correlation id"})
	}

	n, err := s.applyReverts(ctx, pid, stream, "revert_batch:"+req.CorrelationID, reverts)
	if err != nil {
		return serverErr(c, err)
	}

	s.emitAudit(c, auditEvent{
		Type:         platev.EventRollbackPerformed,
		ProjectID:    pid,
		ResourceType: "batch",
		ResourceID:   req.CorrelationID,
		Data: map[string]string{
			"correlation_id": req.CorrelationID,
			"stream":         stream,
			"reverted":       strconv.Itoa(n),
		},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"ok":            true,
		"reverted":      n,
		"correlationId": req.CorrelationID,
	})
}

// RestoreToPointRequest restores a whole stream to a past point, identified by a
// change-log cursor, a named version, or an explicit time.
type RestoreToPointRequest struct {
	ToCursor  *int64 `json:"to_cursor,omitempty"`
	ToVersion string `json:"to_version,omitempty"`
	ToTime    string `json:"to_time,omitempty"` // RFC3339
	Stream    string `json:"stream,omitempty"`
}

// HandleRestoreToPoint restores every target in a stream to the value it held at
// a past point in time (cursor / version / timestamp). Targets unchanged since
// then are left alone; targets created after are blanked. Non-destructive
// (recorded as new edits). Requires PermRollbackChanges.
//
// POST /:ws/:id/restore  { "to_version": "v1" }  | { "to_cursor": 42 } | { "to_time": "..." }
func (s *Server) HandleRestoreToPoint(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermRollbackChanges); err != nil {
		return err
	}
	pg, ok := s.ContentStore.(*bstore.PostgresStore)
	if !ok || pg == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "restore requires the PostgreSQL store"})
	}

	pid := projectParam(c)
	var req RestoreToPointRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	stream := req.Stream
	if stream == "" {
		stream = "main"
	}

	ctx := c.Request().Context()
	var cutoff time.Time
	var label string
	switch {
	case req.ToVersion != "":
		t, err := pg.VersionTime(ctx, pid, req.ToVersion)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		}
		cutoff, label = t, "version:"+req.ToVersion
	case req.ToCursor != nil:
		t, err := pg.CursorTime(ctx, pid, stream, *req.ToCursor)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		}
		cutoff, label = t, "cursor:"+strconv.FormatInt(*req.ToCursor, 10)
	case req.ToTime != "":
		t, err := time.Parse(time.RFC3339, req.ToTime)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "to_time must be RFC3339"})
		}
		cutoff, label = t, "time:"+req.ToTime
	default:
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "one of to_version, to_cursor, or to_time is required"})
	}

	reverts, err := pg.ComputePointInTimeReverts(ctx, pid, stream, cutoff)
	if err != nil {
		return serverErr(c, err)
	}

	n, err := s.applyReverts(ctx, pid, stream, "restore:"+label, reverts)
	if err != nil {
		return serverErr(c, err)
	}

	s.emitAudit(c, auditEvent{
		Type:         platev.EventRollbackPerformed,
		ProjectID:    pid,
		ResourceType: "stream",
		ResourceID:   stream,
		Data:         map[string]string{"restore_to": label, "reverted": strconv.Itoa(n)},
	})

	return c.JSON(http.StatusOK, map[string]any{"ok": true, "restored": n, "to": label})
}
