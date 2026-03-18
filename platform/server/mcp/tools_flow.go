package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerFlowTools registers flow execution MCP tools.
// Phase 1: provides tool discovery (list_flows).
// Phase 2: integrates with FlowService for actual execution (run_flow, get_flow_status).
func (s *MCPServer) registerFlowTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_flows",
		Description: "List available flows and presets that can be executed on project content.",
	}, s.handleListFlows)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "run_flow",
		Description: "Execute a flow on project content. Returns a job ID for status tracking. Available in Phase 2.",
	}, s.handleRunFlow)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_flow_status",
		Description: "Check the execution status of a running or completed flow.",
	}, s.handleGetFlowStatus)
}

type listFlowsInput struct {
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"optional workspace filter"`
}
type listFlowsOutput struct {
	Flows []flowSummary `json:"flows"`
}
type flowSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"` // "builtin" or "custom"
}

func (s *MCPServer) handleListFlows(ctx context.Context, req *mcp.CallToolRequest, input listFlowsInput) (*mcp.CallToolResult, listFlowsOutput, error) {
	// Phase 1: return a static list of built-in flows.
	// Phase 2: will integrate with FlowService and workspace-specific flows.
	flows := []flowSummary{
		{Name: "pseudo-translate", Description: "Generate pseudo-translations for testing and QA", Type: "builtin"},
		{Name: "ai-translate", Description: "Translate content using AI (requires AI provider)", Type: "builtin"},
		{Name: "tm-translate", Description: "Translate content using translation memory matches", Type: "builtin"},
		{Name: "qa-check", Description: "Run quality assurance checks on translations", Type: "builtin"},
		{Name: "brand-voice-check", Description: "Check content against brand voice guidelines", Type: "builtin"},
	}
	return nil, listFlowsOutput{Flows: flows}, nil
}

type runFlowInput struct {
	ProjectID    string `json:"project_id" jsonschema:"the project to run the flow on"`
	FlowName     string `json:"flow_name" jsonschema:"name of the flow to execute"`
	Stream       string `json:"stream,omitempty" jsonschema:"stream name (defaults to main)"`
	TargetLocale string `json:"target_locale,omitempty" jsonschema:"target locale for translation flows"`
}
type runFlowOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *MCPServer) handleRunFlow(ctx context.Context, req *mcp.CallToolRequest, input runFlowInput) (*mcp.CallToolResult, runFlowOutput, error) {
	// Phase 1: return a stub response.
	// Phase 2: will integrate with FlowService for actual execution.
	return nil, runFlowOutput{
		Status:  "not_implemented",
		Message: "Flow execution will be available in Phase 2 with ZeroClaw integration.",
	}, nil
}

type getFlowStatusInput struct {
	JobID string `json:"job_id" jsonschema:"the flow execution job ID"`
}
type getFlowStatusOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *MCPServer) handleGetFlowStatus(ctx context.Context, req *mcp.CallToolRequest, input getFlowStatusInput) (*mcp.CallToolResult, getFlowStatusOutput, error) {
	// Phase 1: stub.
	return nil, getFlowStatusOutput{
		Status:  "not_implemented",
		Message: "Flow status tracking will be available in Phase 2.",
	}, nil
}
