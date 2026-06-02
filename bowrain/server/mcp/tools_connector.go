package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/core/connector"
)

// registerConnectorTools registers connector management MCP tools.
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
	WorkspaceID string `json:"workspace_id" jsonschema:"the workspace the connector belongs to"`
	ProjectID   string `json:"project_id" jsonschema:"the project ID"`
	ConnectorID string `json:"connector_id" jsonschema:"the connector ID to pull from"`
}
type connectorPullOutput struct {
	Status    string `json:"status"`
	ItemCount int    `json:"item_count"`
}

func (s *MCPServer) handleConnectorPull(ctx context.Context, req *mcp.CallToolRequest, input connectorPullInput) (*mcp.CallToolResult, connectorPullOutput, error) {
	if s.connResolver == nil {
		return nil, connectorPullOutput{}, errors.New("connectors not configured")
	}
	if input.ConnectorID == "" {
		return nil, connectorPullOutput{}, errors.New("connector_id is required")
	}
	if input.ProjectID == "" {
		return nil, connectorPullOutput{}, errors.New("project_id is required")
	}

	items, err := s.connResolver.Fetch(ctx, input.WorkspaceID, input.ConnectorID, input.ProjectID, connector.FetchOptions{})
	if err != nil {
		return nil, connectorPullOutput{}, fmt.Errorf("connector pull: %w", err)
	}

	return nil, connectorPullOutput{
		Status:    "completed",
		ItemCount: len(items),
	}, nil
}

type connectorPushInput struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"the workspace the connector belongs to"`
	ProjectID   string `json:"project_id" jsonschema:"the project ID"`
	ConnectorID string `json:"connector_id" jsonschema:"the connector ID to push to"`
}
type connectorPushOutput struct {
	Status string `json:"status"`
}

func (s *MCPServer) handleConnectorPush(ctx context.Context, req *mcp.CallToolRequest, input connectorPushInput) (*mcp.CallToolResult, connectorPushOutput, error) {
	if s.connResolver == nil {
		return nil, connectorPushOutput{}, errors.New("connectors not configured")
	}
	if input.ConnectorID == "" {
		return nil, connectorPushOutput{}, errors.New("connector_id is required")
	}
	if input.ProjectID == "" {
		return nil, connectorPushOutput{}, errors.New("project_id is required")
	}

	if err := s.connResolver.Publish(ctx, input.WorkspaceID, input.ConnectorID, input.ProjectID, connector.PublishOptions{}); err != nil {
		return nil, connectorPushOutput{}, fmt.Errorf("connector push: %w", err)
	}

	return nil, connectorPushOutput{Status: "completed"}, nil
}

type connectorStatusInput struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"the workspace the connector belongs to"`
	ConnectorID string `json:"connector_id" jsonschema:"the connector ID to check"`
}
type connectorStatusOutput struct {
	ConnectorID string   `json:"connector_id"`
	LastSync    string   `json:"last_sync"`
	ItemCount   int      `json:"item_count"`
	PendingPull int      `json:"pending_pull"`
	PendingPush int      `json:"pending_push"`
	Errors      []string `json:"errors,omitempty"`
}

func (s *MCPServer) handleConnectorStatus(ctx context.Context, req *mcp.CallToolRequest, input connectorStatusInput) (*mcp.CallToolResult, connectorStatusOutput, error) {
	if s.connResolver == nil {
		return nil, connectorStatusOutput{}, errors.New("connectors not configured")
	}
	if input.ConnectorID == "" {
		return nil, connectorStatusOutput{}, errors.New("connector_id is required")
	}

	status, err := s.connResolver.ConnectorStatus(ctx, input.WorkspaceID, input.ConnectorID)
	if err != nil {
		return nil, connectorStatusOutput{}, fmt.Errorf("connector status: %w", err)
	}

	return nil, connectorStatusOutput{
		ConnectorID: status.ConnectorID,
		LastSync:    status.LastSync.Format("2006-01-02T15:04:05Z"),
		ItemCount:   status.ItemCount,
		PendingPull: status.PendingPull,
		PendingPush: status.PendingPush,
		Errors:      status.Errors,
	}, nil
}
