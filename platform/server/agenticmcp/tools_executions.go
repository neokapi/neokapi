package agenticmcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerExecutionTools registers agent execution history tools.
func (s *Server) registerExecutionTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_agent_executions",
		Description: "List recent agent session history across the fleet. Use for scheduling decisions (avoiding over-triggering the same agent) and detecting cross-workspace patterns.",
	}, s.handleListAgentExecutions)
}

// ── list_agent_executions ────────────────────────────────────────────────

type listAgentExecutionsInput struct {
	WorkspaceSlug string `json:"workspace_slug,omitempty" jsonschema:"filter by workspace slug"`
	Agent         string `json:"agent,omitempty" jsonschema:"filter by agent persona name"`
	Since         string `json:"since,omitempty" jsonschema:"ISO timestamp to filter executions after"`
	Limit         int    `json:"limit,omitempty" jsonschema:"max results (default 50)"`
}

type listAgentExecutionsOutput struct {
	Executions []Execution `json:"executions"`
}

func (s *Server) handleListAgentExecutions(ctx context.Context, req *mcp.CallToolRequest, input listAgentExecutionsInput) (*mcp.CallToolResult, listAgentExecutionsOutput, error) {
	if s.execStore == nil {
		return nil, listAgentExecutionsOutput{}, fmt.Errorf("execution store not configured")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	execs, err := s.execStore.ListExecutions(ctx, ExecutionFilter{
		WorkspaceSlug: input.WorkspaceSlug,
		Agent:         input.Agent,
		Since:         input.Since,
		Limit:         limit,
	})
	if err != nil {
		return nil, listAgentExecutionsOutput{}, fmt.Errorf("list executions: %w", err)
	}

	return nil, listAgentExecutionsOutput{Executions: execs}, nil
}
