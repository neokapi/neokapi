package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// registerFlowTools registers flow execution MCP tools.
func (s *MCPServer) registerFlowTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_flows",
		Description: "List available flows and presets that can be executed on project content.",
	}, s.handleListFlows)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "run_flow",
		Description: "Execute a flow on project content. Returns a summary of the execution result.",
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
	builtIn := flow.BuiltInFlows()
	flows := make([]flowSummary, 0, len(builtIn))
	for _, f := range builtIn {
		flows = append(flows, flowSummary{
			Name:        f.ID,
			Description: f.Description,
			Type:        "builtin",
		})
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
	Status        string `json:"status"`
	FlowName      string `json:"flow_name"`
	BlocksUpdated int    `json:"blocks_updated"`
	Message       string `json:"message,omitempty"`
}

func (s *MCPServer) handleRunFlow(ctx context.Context, req *mcp.CallToolRequest, input runFlowInput) (*mcp.CallToolResult, runFlowOutput, error) {
	if input.FlowName == "" {
		return nil, runFlowOutput{}, errors.New("flow_name is required")
	}
	if input.ProjectID == "" {
		return nil, runFlowOutput{}, errors.New("project_id is required")
	}

	// Find the flow definition.
	var flowDef *flow.FlowDefinition
	for _, f := range flow.BuiltInFlows() {
		if f.ID == input.FlowName {
			flowDef = &f
			break
		}
	}
	if flowDef == nil {
		return nil, runFlowOutput{}, fmt.Errorf("flow %q not found", input.FlowName)
	}

	if s.toolReg == nil {
		return nil, runFlowOutput{}, errors.New("tool registry not configured")
	}

	stream := input.Stream
	if stream == "" {
		stream = "main"
	}

	// Get blocks from the content store.
	blocks, err := s.contentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: input.ProjectID,
		Stream:    stream,
	})
	if err != nil {
		return nil, runFlowOutput{}, fmt.Errorf("get blocks: %w", err)
	}
	if len(blocks) == 0 {
		return nil, runFlowOutput{
			Status:   "completed",
			FlowName: input.FlowName,
			Message:  "No blocks to process.",
		}, nil
	}

	// Build the tool chain from the flow definition's tool nodes.
	builder := flow.NewFlow(flowDef.ID)
	for _, node := range flowDef.Nodes {
		if node.Type != "tool" {
			continue
		}
		t, err := s.toolReg.NewTool(registry.ToolID(node.Name))
		if err != nil {
			return nil, runFlowOutput{}, fmt.Errorf("resolve tool %q: %w", node.Name, err)
		}
		builder.AddTool(t)
	}
	f, err := builder.Build()
	if err != nil {
		return nil, runFlowOutput{}, fmt.Errorf("build flow: %w", err)
	}

	// Build flow items from blocks.
	items := make([]*flow.Item, 0, len(blocks))
	for _, sb := range blocks {
		item := &flow.Item{}
		item.OutputBlocks = append(item.OutputBlocks, sb.Block)
		if input.TargetLocale != "" {
			item.TargetLocale = model.LocaleID(input.TargetLocale)
		}
		items = append(items, item)
	}

	// Execute.
	executor := flow.NewExecutor(flow.WithFailFast(true))
	if err := executor.Execute(ctx, f, items); err != nil {
		return nil, runFlowOutput{}, fmt.Errorf("execute flow: %w", err)
	}

	// Persist output blocks.
	var allBlocks []*model.Block
	for _, item := range items {
		allBlocks = append(allBlocks, item.OutputBlocks...)
	}
	if len(allBlocks) > 0 {
		if err := s.contentStore.StoreBlocks(ctx, input.ProjectID, stream, allBlocks); err != nil {
			return nil, runFlowOutput{}, fmt.Errorf("persist output: %w", err)
		}
	}

	return nil, runFlowOutput{
		Status:        "completed",
		FlowName:      input.FlowName,
		BlocksUpdated: len(allBlocks),
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
	// Flows execute synchronously via run_flow — no async job tracking needed.
	return nil, getFlowStatusOutput{
		Status:  "not_applicable",
		Message: "Flows execute synchronously. Use run_flow to execute and get results directly.",
	}, nil
}
