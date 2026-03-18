package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerConnectorTools registers connector management MCP tools.
// Phase 1: provides tool stubs. Phase 2: integrates with ConnectorService.
func (s *MCPServer) registerConnectorTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "connector_pull",
		Description: "Pull content from an external source (Git, CMS, etc.) into a project.",
	}, s.handleConnectorPull)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "connector_push",
		Description: "Push translated content from a project to an external target.",
	}, s.handleConnectorPush)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "connector_status",
		Description: "Check the sync status of a connector.",
	}, s.handleConnectorStatus)
}

type connectorPullInput struct {
	ProjectID   string `json:"project_id" jsonschema:"the project ID"`
	ConnectorID string `json:"connector_id" jsonschema:"the connector ID to pull from"`
}
type connectorPullOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *MCPServer) handleConnectorPull(ctx context.Context, req *mcp.CallToolRequest, input connectorPullInput) (*mcp.CallToolResult, connectorPullOutput, error) {
	// Phase 1: stub. Phase 2: integrates with ConnectorService.
	return nil, connectorPullOutput{
		Status:  "not_implemented",
		Message: "Connector pull will be available in Phase 2.",
	}, nil
}

type connectorPushInput struct {
	ProjectID   string `json:"project_id" jsonschema:"the project ID"`
	ConnectorID string `json:"connector_id" jsonschema:"the connector ID to push to"`
}
type connectorPushOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *MCPServer) handleConnectorPush(ctx context.Context, req *mcp.CallToolRequest, input connectorPushInput) (*mcp.CallToolResult, connectorPushOutput, error) {
	// Phase 1: stub.
	return nil, connectorPushOutput{
		Status:  "not_implemented",
		Message: "Connector push will be available in Phase 2.",
	}, nil
}

type connectorStatusInput struct {
	ConnectorID string `json:"connector_id" jsonschema:"the connector ID to check"`
}
type connectorStatusOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *MCPServer) handleConnectorStatus(ctx context.Context, req *mcp.CallToolRequest, input connectorStatusInput) (*mcp.CallToolResult, connectorStatusOutput, error) {
	// Phase 1: stub.
	return nil, connectorStatusOutput{
		Status:  "not_implemented",
		Message: "Connector status will be available in Phase 2.",
	}, nil
}
