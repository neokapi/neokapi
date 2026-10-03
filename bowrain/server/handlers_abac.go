package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/review"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/venue"
)

// blockEditAllowed is the access state a person's edit of a block meets (ABAC),
// as a predicate: the rule the stream's change policy applies to every
// content operation (streamPolicy.access), asked by an action that skips one
// protected block in its own result instead of refusing the whole request.
//
//   - open       → no extra requirement (normal perms apply)
//   - restricted → requires PermReview for the locale, unless the actor owns
//     the block (an owner may keep working their own held content)
//   - published  → requires PermManageProject (re-opening published content is
//     privileged)
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

// BlockAccessRequest moves a block along the access ladder (open, restricted,
// published) and optionally names its owner. Reason records why, for instance
// when content goes back to a lower state. Locale names the translation a
// publish is blessing, which the separation-of-duties gate judges; a request
// that names none publishes the block for every language it holds, and the gate
// judges each of them. The retired values draft and in_review are read as open
// and restricted.
type BlockAccessRequest struct {
	Access  string `json:"access"`
	OwnerID string `json:"owner_id,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Locale  string `json:"locale,omitempty"`
}

// HandleSetBlockAccess sets a block's access state, which governs who may
// change its content (streamPolicy.access). The access state is not content: a
// change set never moves it. Restricting or publishing takes PermReview;
// un-publishing (published to open or restricted) takes PermManageProject. A
// publish is held to the workspace's separation-of-duties policy, and every
// move is recorded as a content.access_changed audit event.
//
// PUT /:ws/:id/blocks/:ref/:bid/access  { "access": "published", "locale": "de" }
func (s *Server) HandleSetBlockAccess(c echo.Context) error {
	as, ok := s.ContentStore.(platstore.BlockAccessStore)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "the access ladder requires a store that keeps it"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")
	stream := refParam(c)
	var req BlockAccessRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	req.Access = bstore.NormalizeBlockAccess(req.Access)
	if !bstore.ValidBlockAccess[req.Access] {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "access must be open, restricted, or published"})
	}

	ctx := c.Request().Context()
	// The current state decides which permission is required, so a failed read
	// is a fault and the gate fails closed on it: "" is not published, and
	// reading it as such would swap the privileged un-publish gate for the
	// review one. A missing row reads as open with no error.
	cur, _, err := as.GetBlockAccess(ctx, pid, stream, bid)
	if err != nil {
		return serverErr(c, fmt.Errorf("read block access for the permission gate: %w", err))
	}
	if cur == bstore.BlockAccessPublished && req.Access != bstore.BlockAccessPublished {
		if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
			return err
		}
	} else if err := s.requirePermission(c, platauth.PermReview); err != nil {
		return err
	}

	// Publishing is a four-eyes step: whoever wrote the wording may not be the
	// one who publishes it.
	if req.Access == bstore.BlockAccessPublished {
		if err := s.enforcePublishSoD(c, pid, stream, bid, req.Locale); err != nil {
			return err
		}
	}

	if err := as.SetBlockAccess(ctx, pid, stream, bid, req.Access, req.OwnerID); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	data := map[string]string{}
	if req.Reason != "" {
		data["reason"] = req.Reason
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventContentAccessChanged,
		ProjectID:    pid,
		ResourceType: "block",
		ResourceID:   bid,
		Data:         data,
		Before:       map[string]string{"access": cur},
		After:        map[string]string{"access": req.Access},
	})
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "block_id": bid, "access": req.Access})
}

// enforcePublishSoD applies the workspace separation-of-duties policy to
// publishing a block, and answers with a 403 (and a non-nil error, so the
// caller stops) when the policy blocks. It asks the gate every review decision
// asks (review.Gate): who last wrote each translation being published by hand.
// A violation is recorded once per publish, naming the language that conflicts.
//
// A read that fails refuses the request: the gate reads an unknown author as no
// conflict, so a discarded error would disable the four-eyes check.
func (s *Server) enforcePublishSoD(c echo.Context, projectID, stream, blockID, locale string) error {
	actor, _ := c.Get("user_id").(string)
	if actor == "" {
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
	wsID, _ := c.Get("workspace_id").(string)
	cfg := review.Config{
		Actor: actor, WorkspaceID: wsID, ProjectID: projectID, Stream: stream,
		BlockIDs: []string{blockID}, Locales: locales,
		// The route asked for the review permission already.
		Permits: func(string) bool { return true },
		Silent:  true,
	}
	if s.AuthStore != nil {
		cfg.Policy = s.AuthStore
	}
	if ts, ok := s.ContentStore.(platstore.TargetAuthorStore); ok {
		cfg.Authors = ts
	}
	gate, err := review.Open(ctx, cfg)
	if err != nil {
		return serverErr(c, err)
	}
	for _, l := range locales {
		err := gate.Allow(blockID, l)
		if gate.Violations() == 0 {
			continue
		}
		s.recordSoDViolation(c, actor, "publish_block:"+blockID+":"+l, gate.Mode(), 1)
		if refusal, ok := errors.AsType[review.Refusal](err); ok && refusal.Reason == venue.RefusedSeparationOfDuties {
			return deny(c, sodRefusal)
		}
		return nil // warn: recorded, and allowed
	}
	return nil
}

// publishSoDLocales names the translations the four-eyes gate judges one
// publish on: the locale a request names, or every language the block holds,
// in a stable order so a refusal names the same language on every attempt. A
// block the project does not hold has none, which leaves the 404 to
// SetBlockAccess.
func (s *Server) publishSoDLocales(ctx context.Context, projectID, stream, blockID, locale string) ([]string, error) {
	if locale != "" {
		return []string{locale}, nil
	}
	blocks, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: projectID, Stream: stream, IDs: []string{blockID}})
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
