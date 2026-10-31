package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// CheckIssueResponse is a single finding returned by the API.
//
// {type, severity ("error"|"warning"), message} is the shape the editor's
// Problems panel has always read, and it is unchanged. The check tools emit
// core/check.Finding, which also locates the finding; dropping that at the
// boundary left the caller with an issue it could name but not point at, so a
// run-native consumer could only record it as a block-level annotation
// (preview/toContentTree). Position, suggestion and the offending snippet now
// ride along.
//
// Position is a pointer with omitempty rather than a value, because
// model.Anchor's zero value is a legitimate reading — "the checker located
// nothing" — and a zero range serialized as {0,0,0,0} is indistinguishable
// from a real span at the start of the first run.
type CheckIssueResponse struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	// Position anchors the finding to the checked runs; absent when the checker
	// judged the whole block.
	Position     *model.Anchor `json:"position,omitempty"`
	Suggestion   string        `json:"suggestion,omitempty"`
	OriginalText string        `json:"original_text,omitempty"`
}

// FileCheckResultResponse holds the findings for a single block.
type FileCheckResultResponse struct {
	BlockID string               `json:"blockId"`
	Issues  []CheckIssueResponse `json:"issues"`
}

// CheckRequest asks for the checks on one block or one item. The editor
// sends it as the request body; the fields also read from the query string, so
// a hand-driven call keeps working.
type CheckRequest struct {
	BlockID string `json:"block_id,omitempty"`
	Item    string `json:"item,omitempty"`
	Locale  string `json:"locale"`
}

// bindCheckRequest reads the request body, falling back to the query string
// for each field it leaves empty.
func bindCheckRequest(c echo.Context) CheckRequest {
	var req CheckRequest
	_ = c.Bind(&req)
	if req.BlockID == "" {
		req.BlockID = firstNonEmpty(c.QueryParam("block_id"), c.Param("bid"))
	}
	if req.Item == "" {
		req.Item = fileParam(c)
	}
	if req.Locale == "" {
		req.Locale = c.QueryParam("locale")
	}
	return req
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// HandleCheckBlock runs the checks in force on a single block.
// POST /:ws/:id/actions/:ref/qa-check-block  {block_id, locale}
func (s *Server) HandleCheckBlock(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ctx := c.Request().Context()
	pid := projectParam(c)
	stream := streamParam(c)
	req := bindCheckRequest(c)
	if req.Locale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale is required"})
	}
	if req.BlockID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "block_id is required"})
	}

	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, req.BlockID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	checks, err := s.checksAtPoint(ctx, pid, stream, sb.ItemName, wsID, c.Param("ws"), model.LocaleID(req.Locale))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, fmt.Errorf("resolve block checks: %w", err))
	}
	issues, err := runChecksOnBlock(ctx, sb.Block, checks)
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, fmt.Errorf("run block checks: %w", err))
	}
	return c.JSON(http.StatusOK, issues)
}

// HandleCheckFile runs the checks in force on every block in an item.
// POST /:ws/:id/actions/:ref/qa-check  {item, locale}
func (s *Server) HandleCheckFile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ctx := c.Request().Context()
	pid := projectParam(c)
	stream := streamParam(c)
	req := bindCheckRequest(c)
	if req.Locale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale is required"})
	}

	storedBlocks, err := s.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: pid,
		Stream:    stream,
		ItemName:  req.Item,
	})
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	// Resolved once for the item: every block in it sits at the same point, and
	// the resolution reads the collection, the voice profile and the terms.
	wsID, _ := c.Get("workspace_id").(string)
	checks, err := s.checksAtPoint(ctx, pid, stream, req.Item, wsID, c.Param("ws"), model.LocaleID(req.Locale))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, fmt.Errorf("resolve item checks: %w", err))
	}

	results := make([]FileCheckResultResponse, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		issues, err := runChecksOnBlock(ctx, sb.Block, checks)
		if err != nil {
			return serverErrStatus(c, http.StatusServiceUnavailable, fmt.Errorf("run item block checks: %w", err))
		}
		results = append(results, FileCheckResultResponse{
			BlockID: sb.Block.ID,
			Issues:  issues,
		})
	}

	return c.JSON(http.StatusOK, results)
}

// pointChecks is the set of checks in force at one point (jobs.PointChecks).
type pointChecks = jobs.PointChecks

// checkBinding is where the server reads the checks at a point from: its
// content store, voice store, workspace default and the workspace's terms.
func (s *Server) checkBinding(workspaceSlug string) jobs.PointCheckBinding {
	voiceCtx := s.editorVoiceContext()
	b := jobs.PointCheckBinding{Store: s.ContentStore, Voice: voiceCtx.Voice, WorkspaceDefault: voiceCtx.WorkspaceDefault}
	if voiceCtx.Stores != nil && workspaceSlug != "" {
		b.Terms = func(ctx context.Context) (terms.Terminology, error) {
			return voiceCtx.Stores.getTerms(ctx, workspaceSlug)
		}
	}
	return b
}

// checksAtPoint resolves the checks in force where an item sits
// (jobs.ResolvePointChecks).
func (s *Server) checksAtPoint(ctx context.Context, projectID, stream, itemName, workspaceID, workspaceSlug string, locale model.LocaleID) (pointChecks, error) {
	if s.ContentStore == nil {
		return pointChecks{TargetLocale: locale}, errors.New("check context: content store unavailable")
	}
	return jobs.ResolvePointChecks(ctx, s.checkBinding(workspaceSlug), projectID, stream, itemName, workspaceID, locale)
}

// runChecksOnBlock runs the checks in force on a single block and returns the
// findings for exactly this block+locale.
//
// The tools run against a scratch copy with a private annotation surface:
// findings ACCUMULATE on the block's unified FindingsAnnotation by design
// (check.Annotate), so running the checks for several locales over the same
// in-memory block — the dashboard ship-state/compliant pass and convergence's
// countFailingBlocks both do — would otherwise leak one locale's findings into
// every later locale's read (findings carry no locale), and mutate a shared
// block as a side effect of a read.
//
// A zero pointChecks (locale only) is the standard set, which is what the
// dashboard passes: they judge a whole project a block at a time and resolve no
// point of their own.
func runChecksOnBlock(ctx context.Context, block *model.Block, checks pointChecks) ([]CheckIssueResponse, error) {
	standard, err := standardFindings(ctx, block, checks)
	if err != nil {
		return nil, err
	}
	issues := checkIssuesFromFindings(standard)
	voice, err := voiceFindings(ctx, block, checks)
	if err != nil {
		return nil, err
	}
	return append(issues, checkIssuesFromFindings(voice)...), nil
}

// standardFindings runs the standard per-locale checks on a scratch copy of
// block (jobs.StandardFindings).
func standardFindings(ctx context.Context, block *model.Block, checks pointChecks) ([]check.Finding, error) {
	return jobs.StandardFindings(ctx, block, checks)
}

// voiceFindings runs the vocabulary governing the point on a scratch copy of
// block (jobs.VoiceFindings).
func voiceFindings(ctx context.Context, block *model.Block, checks pointChecks) ([]check.Finding, error) {
	return jobs.VoiceFindings(ctx, block, checks)
}

// checkIssuesFromFindings maps core/check.Finding onto the CheckIssueResponse wire shape.
//
// Everything the finding locates or suggests rides along; only the severity is
// narrowed, to the two values the Problems panel has always styled. The result
// is an empty slice, never nil, so a clean block encodes as [].
func checkIssuesFromFindings(findings []check.Finding) []CheckIssueResponse {
	result := make([]CheckIssueResponse, 0, len(findings))
	for _, f := range findings {
		issue := CheckIssueResponse{
			Type:         f.Category,
			Severity:     checkWireSeverity(f.Fails && !f.Suggested),
			Message:      f.Message,
			Suggestion:   f.Suggestion,
			OriginalText: f.OriginalText,
		}
		if !f.Position.IsZero() {
			pos := f.Position
			issue.Position = &pos
		}
		result = append(result, issue)
	}
	return result
}

// checkWireSeverity maps whether a finding fails onto the two-valued severity
// the endpoint returns: a failing finding is "error", one that reports
// "warning".
func checkWireSeverity(fails bool) string {
	if fails {
		return "error"
	}
	return "warning"
}
