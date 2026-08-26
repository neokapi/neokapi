package server

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// QAIssueResponse is a single QA finding returned by the API.
//
// {type, severity ("error"|"warning"), message} is the shape the editor's
// Problems panel has always read, and it is unchanged. The QA tools emit
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
type QAIssueResponse struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	// Position anchors the finding to the checked runs; absent when the checker
	// judged the whole block.
	Position     *model.Anchor `json:"position,omitempty"`
	Suggestion   string        `json:"suggestion,omitempty"`
	OriginalText string        `json:"original_text,omitempty"`
}

// FileQAResultResponse holds QA results for a single block.
type FileQAResultResponse struct {
	BlockID string            `json:"blockId"`
	Issues  []QAIssueResponse `json:"issues"`
}

// HandleQACheckBlock runs QA checks on a single block.
// POST /editor/projects/:pid/blocks/:bid/qa-check?locale=xx
func (s *Server) HandleQACheckBlock(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")
	locale := c.QueryParam("locale")
	if locale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale query parameter is required"})
	}

	sb, err := s.ContentStore.GetBlock(c.Request().Context(), pid, "main", bid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	issues := runQAOnBlock(c.Request().Context(), sb.Block, model.LocaleID(locale))
	return c.JSON(http.StatusOK, issues)
}

// HandleQACheckFile runs QA checks on all blocks in a file.
// POST /editor/projects/:pid/file-qa-check/*?locale=xx
func (s *Server) HandleQACheckFile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	fname := fileParam(c)
	locale := c.QueryParam("locale")
	if locale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale query parameter is required"})
	}

	storedBlocks, err := s.ContentStore.GetBlocks(c.Request().Context(), store.BlockQuery{
		ProjectID: pid,
		Stream:    "main",
		ItemName:  fname,
	})
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	targetLocale := model.LocaleID(locale)
	ctx := c.Request().Context()
	results := make([]FileQAResultResponse, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		issues := runQAOnBlock(ctx, sb.Block, targetLocale)
		results = append(results, FileQAResultResponse{
			BlockID: sb.Block.ID,
			Issues:  issues,
		})
	}

	return c.JSON(http.StatusOK, results)
}

// runQAOnBlock runs the QA check tool on a single block and returns the issues
// for exactly this block+locale.
//
// The tool runs against a scratch copy with a private annotation surface:
// findings ACCUMULATE on the block's unified FindingsAnnotation by design
// (check.Annotate), so running the checks for several locales over the same
// in-memory block — the dashboard ship-state/compliant pass and convergence's
// countFailingBlocks both do — would otherwise leak one locale's findings into
// every later locale's read (findings carry no locale), and mutate a shared
// block as a side effect of a read.
func runQAOnBlock(ctx context.Context, block *model.Block, locale model.LocaleID) []QAIssueResponse {
	cfg := tools.NewQACheckConfig(locale)
	qaTool := tools.NewQACheckTool(cfg)

	scratch := *block
	scratch.Annotations = nil // fresh findings surface; SetAnno re-creates lazily
	part := &model.Part{
		Type:     model.PartBlock,
		Resource: &scratch,
	}

	// Process through the tool (ignoring error since the tool is deterministic).
	_, _ = qaTool.ApplyContext(ctx, part)

	// Read the findings the tool recorded on the scratch copy and map them onto
	// the wire shape the editor's Problems panel expects.
	return qaIssuesFromFindings(check.Findings(tool.NewBlockViewWithContext(ctx, &scratch)))
}

// qaIssuesFromFindings maps core/check.Finding onto the QA wire shape.
//
// Everything the finding locates or suggests rides along; only the severity is
// narrowed, to the two values the Problems panel has always styled. The result
// is an empty slice, never nil, so a clean block encodes as [].
func qaIssuesFromFindings(findings []check.Finding) []QAIssueResponse {
	result := make([]QAIssueResponse, 0, len(findings))
	for _, f := range findings {
		issue := QAIssueResponse{
			Type:         f.Category,
			Severity:     qaWireSeverity(f.Severity),
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

// qaWireSeverity maps a core/check.Severity onto the two-valued severity the QA
// API has always returned: critical/major are "error", minor/neutral "warning".
func qaWireSeverity(s check.Severity) string {
	switch s {
	case check.SeverityCritical, check.SeverityMajor:
		return "error"
	default:
		return "warning"
	}
}
