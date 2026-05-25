package server

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

// QAIssueResponse is a single QA finding returned by the API.
type QAIssueResponse struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
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

	issues := runQAOnBlock(sb.Block, model.LocaleID(locale))
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
	results := make([]FileQAResultResponse, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		issues := runQAOnBlock(sb.Block, targetLocale)
		results = append(results, FileQAResultResponse{
			BlockID: sb.Block.ID,
			Issues:  issues,
		})
	}

	return c.JSON(http.StatusOK, results)
}

// runQAOnBlock runs the QA check tool on a single block and returns issues.
func runQAOnBlock(block *model.Block, locale model.LocaleID) []QAIssueResponse {
	cfg := tools.NewQACheckConfig(locale)
	qaTool := tools.NewQACheckTool(cfg)

	part := &model.Part{
		Type:     model.PartBlock,
		Resource: block,
	}

	// Process through the tool (ignoring error since the tool is deterministic).
	_, _ = qaTool.Apply(part)

	// Read issues from block properties.
	issuesJSON, ok := block.Properties[tools.PropQAIssues]
	if !ok || issuesJSON == "" || issuesJSON == "[]" {
		return []QAIssueResponse{}
	}

	var qaIssues []tools.QAIssue
	if err := json.Unmarshal([]byte(issuesJSON), &qaIssues); err != nil {
		return []QAIssueResponse{}
	}

	result := make([]QAIssueResponse, len(qaIssues))
	for i, issue := range qaIssues {
		result[i] = QAIssueResponse{
			Type:     issue.Type,
			Severity: string(issue.Severity),
			Message:  issue.Message,
		}
	}
	return result
}
