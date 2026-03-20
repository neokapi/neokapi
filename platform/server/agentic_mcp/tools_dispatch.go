package agentic_mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerDispatchTools registers agent dispatch and execution history tools.
func (s *Server) registerDispatchTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "trigger_agent_session",
		Description: "Dispatch a worker agent session by placing a task message on the appropriate queue. The worker agent picks it up via KEDA (Azure) or Redis (local). The task message is passed directly to the agent's -m flag.",
	}, s.handleTriggerAgentSession)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_agent_executions",
		Description: "List recent agent session history across the fleet. Use for scheduling decisions (avoiding over-triggering the same agent) and detecting cross-workspace patterns.",
	}, s.handleListAgentExecutions)
}

// ── trigger_agent_session ────────────────────────────────────────────────

type triggerAgentSessionInput struct {
	WorkspaceSlug string `json:"workspace_slug" jsonschema:"target workspace slug"`
	AgentRole     string `json:"agent_role" jsonschema:"agent role: developer, translator, brand_manager, qa, pm"`
	Persona       string `json:"persona" jsonschema:"agent persona name (e.g. sophie-translator)"`
	Task          string `json:"task" jsonschema:"natural language task description for the agent"`
	Locale        string `json:"locale,omitempty" jsonschema:"target locale for translators"`
	Priority      string `json:"priority,omitempty" jsonschema:"normal or high (high skips inter-session gap)"`
}

type triggerAgentSessionOutput struct {
	ExecutionID string `json:"execution_id"`
	QueuedAt    string `json:"queued_at"`
	Queue       string `json:"queue"`
}

func (s *Server) handleTriggerAgentSession(ctx context.Context, req *mcp.CallToolRequest, input triggerAgentSessionInput) (*mcp.CallToolResult, triggerAgentSessionOutput, error) {
	if s.dispatcher == nil {
		return nil, triggerAgentSessionOutput{}, fmt.Errorf("agent dispatcher not configured")
	}

	priority := input.Priority
	if priority == "" {
		priority = "normal"
	}

	result, err := s.dispatcher.Dispatch(ctx, DispatchRequest{
		WorkspaceSlug: input.WorkspaceSlug,
		AgentRole:     input.AgentRole,
		Persona:       input.Persona,
		Task:          input.Task,
		Locale:        input.Locale,
		Priority:      priority,
	})
	if err != nil {
		return nil, triggerAgentSessionOutput{}, fmt.Errorf("dispatch agent: %w", err)
	}

	return nil, triggerAgentSessionOutput{
		ExecutionID: result.ExecutionID,
		QueuedAt:    result.QueuedAt,
		Queue:       result.Queue,
	}, nil
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
