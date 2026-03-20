package agenticmcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerFeedbackTools registers issue filing and memory commit tools.
func (s *Server) registerFeedbackTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "file_feedback_issue",
		Description: "Create a GitHub issue in the neokapi/agent-feedback repo when a platform problem is detected. The coordinator provides cross-workspace context for richer bug reports.",
	}, s.handleFileFeedbackIssue)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "commit_memory",
		Description: "Commit a file change to the fleet repo. Used to persist coordinator observations, update workspace status, or modify agent SOUL.md overrides.",
	}, s.handleCommitMemory)
}

// ── file_feedback_issue ──────────────────────────────────────────────────

type fileFeedbackIssueInput struct {
	Title  string   `json:"title" jsonschema:"issue title"`
	Body   string   `json:"body" jsonschema:"issue body with markdown formatting"`
	Labels []string `json:"labels,omitempty" jsonschema:"issue labels (e.g. bug, format-parser, cross-workspace)"`
}

type fileFeedbackIssueOutput struct {
	IssueURL    string `json:"issue_url"`
	IssueNumber int    `json:"issue_number"`
}

func (s *Server) handleFileFeedbackIssue(ctx context.Context, req *mcp.CallToolRequest, input fileFeedbackIssueInput) (*mcp.CallToolResult, fileFeedbackIssueOutput, error) {
	if s.issues == nil {
		return nil, fileFeedbackIssueOutput{}, fmt.Errorf("issue tracker not configured")
	}

	url, number, err := s.issues.FileIssue(ctx, input.Title, input.Body, input.Labels)
	if err != nil {
		return nil, fileFeedbackIssueOutput{}, fmt.Errorf("file issue: %w", err)
	}

	return nil, fileFeedbackIssueOutput{
		IssueURL:    url,
		IssueNumber: number,
	}, nil
}

// ── commit_memory ────────────────────────────────────────────────────────

type commitMemoryInput struct {
	Path    string `json:"path" jsonschema:"file path within the fleet repo"`
	Content string `json:"content" jsonschema:"file content to write"`
	Message string `json:"message" jsonschema:"git commit message"`
}

type commitMemoryOutput struct {
	CommitSHA string `json:"commit_sha"`
}

func (s *Server) handleCommitMemory(ctx context.Context, req *mcp.CallToolRequest, input commitMemoryInput) (*mcp.CallToolResult, commitMemoryOutput, error) {
	if s.fleetRepo == nil {
		return nil, commitMemoryOutput{}, fmt.Errorf("fleet repo not configured")
	}

	sha, err := s.fleetRepo.CommitFile(ctx, input.Path, input.Content, input.Message)
	if err != nil {
		return nil, commitMemoryOutput{}, fmt.Errorf("commit memory: %w", err)
	}

	return nil, commitMemoryOutput{CommitSHA: sha}, nil
}
