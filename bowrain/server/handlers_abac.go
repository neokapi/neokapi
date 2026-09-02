package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// requireEditableStatus is the ABAC predicate gating edits by a block's access
// state (and ownership). It runs in addition to the base translate permission:
//
//   - open       → no extra requirement (normal perms apply)
//   - restricted → requires PermReview for the locale, unless the actor owns
//     the block (an owner may keep working their own held content)
//   - published  → editing requires PermManageProject (re-opening published
//     content is privileged)
//
// Returns a non-nil error (after writing the 403) when the edit is not allowed.
func (s *Server) requireEditableStatus(c echo.Context, projectID, blockID, locale string) error {
	as, ok := s.ContentStore.(platstore.BlockAccessStore)
	if !ok {
		return nil // access ABAC only enforced on stores that keep the ladder
	}
	access, owner, err := as.GetBlockAccess(c.Request().Context(), projectID, refParam(c), blockID)
	if err != nil {
		return nil // don't block edits on an access-lookup error
	}
	switch access {
	case bstore.BlockAccessPublished:
		return s.requirePermission(c, platauth.PermManageProject)
	case bstore.BlockAccessRestricted:
		if actor, _ := c.Get("user_id").(string); owner != "" && owner == actor {
			return nil
		}
		return s.requireLanguagePermission(c, platauth.PermReview, locale)
	default:
		return nil
	}
}

// blockEditAllowed is requireEditableStatus as a predicate: the same ABAC
// gate, answering without writing a 403. A batch route uses it so one
// protected block is skipped in its own result instead of refusing the whole
// request.
func (s *Server) blockEditAllowed(c echo.Context, projectID, blockID, locale string) bool {
	as, ok := s.ContentStore.(platstore.BlockAccessStore)
	if !ok {
		return true
	}
	access, owner, err := as.GetBlockAccess(c.Request().Context(), projectID, refParam(c), blockID)
	if err != nil {
		return true
	}
	switch access {
	case bstore.BlockAccessPublished:
		return hasPermission(c, platauth.PermManageProject)
	case bstore.BlockAccessRestricted:
		if actor, _ := c.Get("user_id").(string); owner != "" && owner == actor {
			return true
		}
		return allowsLanguage(c, platauth.PermReview, locale)
	default:
		return true
	}
}

// BlockStatusRequest sets a block's access state and optional owner. Reason
// captures a structured rejection/send-back note when moving content back to a
// lower state. The JSON field keeps its historical name — "status" is the
// endpoint's name — but the VALUES are the access vocabulary (open |
// restricted | published); the retired draft/in_review are normalized on the
// way in rather than rejected.
//
// Locale names the translation a publish is blessing. It picks the target the
// four-eyes gate asks about, so publishing the German wording is judged on who
// wrote the German wording. A request that names none is publishing the block
// for every language it holds, and the gate asks about each of them.
type BlockStatusRequest struct {
	Status  string `json:"status"`
	OwnerID string `json:"owner_id,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Locale  string `json:"locale,omitempty"`
}

// HandleSetBlockStatus changes a block's access state. Restricting or
// publishing requires PermReview; un-publishing (published → open/restricted)
// requires PermManageProject.
//
// PUT /:ws/:id/blocks/:ref/:bid/status  { "status": "published", "locale": "de" }
func (s *Server) HandleSetBlockStatus(c echo.Context) error {
	as, ok := s.ContentStore.(platstore.BlockAccessStore)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "access ladder requires a store that keeps it"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")
	var req BlockStatusRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	req.Status = bstore.NormalizeBlockAccess(req.Status)
	if !bstore.ValidBlockAccess[req.Status] {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "status must be open, restricted, or published"})
	}

	ctx := c.Request().Context()
	// The current state decides which permission is required, so a failed read
	// must not be answered with the zero value: "" is not published, which
	// silently swaps the privileged un-publish gate below for the review one.
	// The store already separates the cases — a missing row comes back as
	// open with a nil error — so an error here is a real fault, and the gate
	// fails closed on it.
	cur, _, err := as.GetBlockAccess(ctx, pid, refParam(c), bid)
	if err != nil {
		return serverErr(c, fmt.Errorf("read block access for the permission gate: %w", err))
	}

	// Un-publishing is privileged; other access transitions are review actions.
	if cur == bstore.BlockAccessPublished && req.Status != bstore.BlockAccessPublished {
		if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
			return err
		}
	} else {
		if err := s.requirePermission(c, platauth.PermReview); err != nil {
			return err
		}
	}

	// Separation of duties: approving (publishing) is a four-eyes step, and the
	// approver must be somebody other than the translator who wrote the wording.
	if req.Status == bstore.BlockAccessPublished {
		if err := s.enforcePublishSoD(c, pid, refParam(c), bid, req.Locale); err != nil {
			return err
		}
	}

	if err := as.SetBlockAccess(ctx, pid, refParam(c), bid, req.Status, req.OwnerID); err != nil {
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

// enforcePublishSoD applies the workspace separation-of-duties policy to
// publishing a block, and answers with a 403 (plus a non-nil error, so the
// caller aborts) when the policy blocks.
//
// It asks the store the question the review approval routes ask: who last wrote
// this translation by hand, for this locale. The same block_history column also
// carries the decider of every recorded decision and the literal "system" a
// settled projection writes, so reading the newest attributed row of any kind
// names the last decider as the translator, and the person who wrote the
// wording can publish it as soon as somebody else has decided on it.
//
// A store that keeps no target authorship knows of no author and every publish
// passes. A read that fails refuses the request: judgeSoD reads an empty author
// as "no conflict of interest", so a discarded error disables the four-eyes
// check rather than tightening it.
func (s *Server) enforcePublishSoD(c echo.Context, projectID, stream, blockID, locale string) error {
	actor, _ := c.Get("user_id").(string)
	ts, ok := s.ContentStore.(platstore.TargetAuthorStore)
	if actor == "" || !ok {
		return nil
	}
	ctx := c.Request().Context()
	locales, err := s.publishSoDLocales(ctx, projectID, stream, blockID, locale)
	if err != nil {
		return serverErr(c, fmt.Errorf("read the block's targets for separation of duties: %w", err))
	}
	if len(locales) == 0 {
		return nil
	}
	authors, err := ts.LastTargetAuthors(ctx, projectID, stream, []string{blockID}, locales)
	if err != nil {
		return serverErr(c, fmt.Errorf("read target authorship for separation of duties: %w", err))
	}
	for _, l := range locales {
		author := authors[platstore.TargetRef{BlockID: blockID, Locale: l}]
		// An unattributed target is machine-authored: a run wrote it outside
		// any request, so there is nobody for the approver to conflict with.
		if author != actor {
			continue
		}
		// One record per publish, naming the language that conflicts, so a
		// block held in a dozen languages files one violation and not a dozen.
		return s.enforceSoD(c, actor, author, "publish_block:"+blockID+":"+l)
	}
	return nil
}

// publishSoDLocales names the targets the four-eyes gate judges one publish on.
// A request that names a locale is blessing that translation and is judged on
// it alone. A request that names none moves the whole block to published, which
// covers every language it holds, so each of those targets is judged. The order
// is stable, so the locale a refusal reports is the same on every attempt.
//
// The lookup is GetBlocks rather than GetBlock because a block this project
// does not hold comes back as no rows here, which leaves the missing-block
// answer the 404 SetBlockAccess already writes.
func (s *Server) publishSoDLocales(ctx context.Context, projectID, stream, blockID, locale string) ([]string, error) {
	if locale != "" {
		return []string{locale}, nil
	}
	blocks, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: projectID, Stream: stream, IDs: []string{blockID},
	})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil {
			continue
		}
		for _, l := range sb.Block.TargetLocales() {
			out = append(out, string(l))
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}
