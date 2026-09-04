package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/flowdef"
)

// registerFlowTools registers flow execution MCP tools.
func (s *MCPServer) registerFlowTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_flows",
		Description: "List the flows that can be run on project content: the built-in catalog, plus the project's own flows when project_id is given.",
	}, s.handleListFlows)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "run_flow",
		Description: "Run a flow over a project's stored content and write the results back. Returns a summary of what the run touched.",
	}, s.handleRunFlow)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_flow_status",
		Description: "Check the execution status of a running or completed flow.",
	}, s.handleGetFlowStatus)
}

type listFlowsInput struct {
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"optional workspace filter"`
	ProjectID   string `json:"project_id,omitempty" jsonschema:"include the project's own flows beside the built-in catalog"`
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
	defs, err := s.listFlows(ctx, input.ProjectID)
	if err != nil {
		return nil, listFlowsOutput{}, err
	}
	flows := make([]flowSummary, 0, len(defs))
	for _, f := range defs {
		kind := "custom"
		if f.Source == registry.SourceBuiltIn {
			kind = "builtin"
		}
		flows = append(flows, flowSummary{
			Name:        f.ID,
			Description: f.Description,
			Type:        kind,
		})
	}
	return nil, listFlowsOutput{Flows: flows}, nil
}

// listFlows lists the flows a project can run. Without a catalog only the
// built-in flows are known.
func (s *MCPServer) listFlows(ctx context.Context, projectID string) ([]flow.FlowDefinition, error) {
	if s.flowCatalog == nil || projectID == "" {
		return flowdef.BuiltInFlows(), nil
	}
	return s.flowCatalog.List(ctx, projectID)
}

// resolveFlow resolves a flow id under a project through the catalog, the
// same lookup an automation rule's run_flow action makes. Without a catalog
// only the built-in flows resolve.
func (s *MCPServer) resolveFlow(ctx context.Context, projectID, flowID string) (*flow.FlowDefinition, error) {
	if s.flowCatalog != nil {
		return s.flowCatalog.Get(ctx, projectID, flowID)
	}
	for _, f := range flowdef.BuiltInFlows() {
		if f.ID == flowID {
			def := f
			return &def, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", service.ErrFlowNotFound, flowID)
}

type runFlowInput struct {
	ProjectID    string `json:"project_id" jsonschema:"the project to run the flow on"`
	FlowName     string `json:"flow_name" jsonschema:"id of the flow to run, built-in or one of the project's own"`
	Stream       string `json:"stream,omitempty" jsonschema:"stream name (defaults to main)"`
	TargetLocale string `json:"target_locale,omitempty" jsonschema:"target locale for bilingual tools such as translate or checks"`
}
type runFlowOutput struct {
	Status        string `json:"status"`
	FlowName      string `json:"flow_name"`
	BlocksUpdated int    `json:"blocks_updated"`
	Message       string `json:"message,omitempty"`
}

// handleRunFlow runs a flow over a project's stored content through the same
// catalog and runner an automation rule's run_flow action uses, so an agent
// and a rule naming the same flow id run the same flow the same way.
func (s *MCPServer) handleRunFlow(ctx context.Context, req *mcp.CallToolRequest, input runFlowInput) (*mcp.CallToolResult, runFlowOutput, error) {
	if input.FlowName == "" {
		return nil, runFlowOutput{}, errors.New("flow_name is required")
	}
	if input.ProjectID == "" {
		return nil, runFlowOutput{}, errors.New("project_id is required")
	}
	if s.flowRunner == nil {
		return nil, runFlowOutput{}, errors.New("flow runner not configured")
	}

	def, err := s.resolveFlow(ctx, input.ProjectID, input.FlowName)
	if err != nil {
		return nil, runFlowOutput{}, err
	}

	run := service.FlowRun{
		Definition: def,
		ProjectID:  input.ProjectID,
		Stream:     input.Stream,
		Source:     "mcp",
	}
	if input.TargetLocale != "" {
		run.TargetLocales = []string{input.TargetLocale}
	}
	if req != nil {
		run.Actor = extractUserID(req)
	}

	res, err := s.flowRunner.RunFlow(ctx, run)
	if err != nil {
		return nil, runFlowOutput{}, fmt.Errorf("run flow %q: %w", input.FlowName, err)
	}
	out := runFlowOutput{
		Status:        "completed",
		FlowName:      input.FlowName,
		BlocksUpdated: res.Blocks,
	}
	if res.Items == 0 {
		out.Message = "No blocks to process."
	}
	return nil, out, nil
}

type getFlowStatusInput struct {
	JobID string `json:"job_id" jsonschema:"the flow execution job ID"`
}
type getFlowStatusOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *MCPServer) handleGetFlowStatus(ctx context.Context, req *mcp.CallToolRequest, input getFlowStatusInput) (*mcp.CallToolResult, getFlowStatusOutput, error) {
	// Flows run to completion inside run_flow, so there is no job to poll.
	return nil, getFlowStatusOutput{
		Status:  "not_applicable",
		Message: "Flows run to completion inside run_flow; its result carries the outcome.",
	}, nil
}
