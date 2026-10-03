package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// applyReverts restores each target a revert names to the value it held: a
// set_content of its prior runs, or the removal of a target the reverted batch
// created. Each operation names the revision the target holds now, which the
// revert read, and the change set is applied through the stream's change
// service as the person, labelled reason in the history it writes, so the
// revert is itself recorded and can be reverted. A person restoring prior
// wording lands it over the findings it brings back (gate report). A target
// that moved since the read keeps what it holds. It returns the number of
// targets reverted.
func (s *Server) applyReverts(ctx context.Context, c echo.Context, pid, stream, reason string, reverts []bstore.TargetRevert) (int, error) {
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return 0, err
	}
	set := change.Set{Gate: change.GateReport}
	read := map[string]*venue.StoredBlock{}
	for _, r := range reverts {
		sb, ok := read[r.BlockID]
		if !ok {
			sb, _ = s.ContentStore.GetBlock(ctx, pid, stream, r.BlockID)
			read[r.BlockID] = sb
		}
		if sb == nil {
			continue // block gone; skip
		}
		if op, ok := revertOp(sb, r); ok {
			set.Ops = append(set.Ops, op)
		}
	}
	if len(set.Ops) == 0 {
		return 0, nil
	}
	ctx = bstore.WithChangeContext(ctx, bstore.ChangeContext{Reason: reason})
	wsID, _ := c.Get("workspace_id").(string)
	sender := requestSender(c)
	sc := s.newStreamChange(ctx, c, proj, stream, wsID, c.Param("ws"), sender)
	res, _, _, err := sc.applyEach(ctx, set, change.Actor{Kind: change.ActorPerson, Name: sender.userID})
	if err != nil || res == nil || res.Status == change.SetRefused {
		return 0, err
	}
	n := 0
	for _, op := range res.Ops {
		if op.Status == change.OpApplied {
			n++
		}
	}
	return n, nil
}

// revertOp is the operation that restores one target, guarded by the revision
// the target holds in sb: its prior runs, its prior text, or its removal. A
// revert that restores what the target already holds is no operation.
func revertOp(sb *venue.StoredBlock, r bstore.TargetRevert) (change.Op, bool) {
	loc := model.LocaleID(r.Locale)
	at := change.Ref{Doc: sb.ItemName, Block: sb.Block.ID, Edition: model.EditionKey{Locale: loc}}
	rev := store.TargetRevision(sb, loc)
	if r.Clear {
		if rev == model.AbsentRevision {
			return change.Op{}, false
		}
		return change.Op{Kind: change.KindRemoveEdition, At: at, IfMatch: rev, Body: &change.RemoveEdition{}}, true
	}
	return change.Op{Kind: change.KindSetContent, At: at, IfMatch: rev, Body: &change.SetContent{Content: historyContent(r.Text, r.Coded)}}, true
}

// historyContent is a history entry's content: its runs, which keep the inline
// markup, or its plain text when it recorded none.
func historyContent(text, coded string) change.Content {
	if coded != "" {
		var runs []model.Run
		if json.Unmarshal([]byte(coded), &runs) == nil && len(runs) > 0 {
			return change.Content{Runs: runs}
		}
	}
	return change.Content{Text: &text}
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

	var entry *store.BlockHistoryEntry
	for i := range history {
		if history[i].Seq == req.ToSeq {
			entry = &history[i]
			break
		}
	}
	if entry == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "history entry not found for this block/locale"})
	}
	if !bstore.IsContentHistory(entry.ChangeType) {
		return c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: fmt.Sprintf(
			"history entry %d records a %s, not the translation's wording; roll back to an entry that records its content", req.ToSeq, entry.ChangeType)})
	}

	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, bid)
	if err != nil || sb == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
	}
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found"})
	}
	// The restore is a set_content of the entry's runs on the revision the
	// target holds now, or on the one the caller read; an entry that records
	// the translation's removal restores its removal. Labelled so its history
	// entry reads as a rollback; a person restoring prior wording lands it over
	// the findings it brings back.
	locale := model.LocaleID(req.Locale)
	rev := req.BaseRevision
	if rev == "" {
		rev = store.TargetRevision(sb, locale)
	}
	op := change.Op{
		Kind:    change.KindSetContent,
		At:      change.Ref{Doc: sb.ItemName, Block: bid, Edition: model.EditionKey{Locale: locale}},
		IfMatch: rev,
		Body:    &change.SetContent{Content: historyContent(entry.Text, entry.Coded)},
	}
	if entry.ChangeType == bstore.HistoryTargetRemoved {
		if rev == model.AbsentRevision {
			// The block holds no translation in the language: as it was.
			return c.JSON(http.StatusOK, map[string]any{"ok": true, "block_id": bid, "locale": req.Locale, "restored_seq": req.ToSeq})
		}
		op.Kind, op.Body = change.KindRemoveEdition, &change.RemoveEdition{}
	}
	set := change.Set{Gate: change.GateReport, Ops: []change.Op{op}}
	ctx = bstore.WithChangeContext(ctx, bstore.ChangeContext{Reason: "rollback:" + strconv.FormatInt(req.ToSeq, 10)})
	wsID, _ := c.Get("workspace_id").(string)
	sender := requestSender(c)
	sc := s.newStreamChange(ctx, c, proj, stream, wsID, c.Param("ws"), sender)
	res, err := sc.apply(ctx, set, change.Actor{Kind: change.ActorPerson, Name: sender.userID})
	if err != nil {
		return serverErr(c, err)
	}
	if res.Status == change.SetRefused {
		return c.JSON(resultStatus(res), res)
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

	n, err := s.applyReverts(ctx, c, pid, stream, "revert_batch:"+req.CorrelationID, reverts)
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

	n, err := s.applyReverts(ctx, c, pid, stream, "restore:"+label, reverts)
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
