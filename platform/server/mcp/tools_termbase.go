package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTermbaseTools registers terminology management MCP tools.
// Phase 1: provides tool stubs. Phase 2: integrates with workspace termbase stores.
func (s *MCPServer) registerTermbaseTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "term_search",
		Description: "Search the workspace terminology base for matching terms.",
	}, s.handleTermSearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "term_add",
		Description: "Add new term entries to the workspace terminology base.",
	}, s.handleTermAdd)
}

type termSearchInput struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"the workspace ID"`
	Query       string `json:"query" jsonschema:"search query for terms"`
	Locale      string `json:"locale,omitempty" jsonschema:"optional locale filter"`
	Limit       int    `json:"limit,omitempty" jsonschema:"max results (default 10)"`
}
type termSearchOutput struct {
	Terms  []termResult `json:"terms"`
	Status string       `json:"status"`
}
type termResult struct {
	Term        string `json:"term"`
	Definition  string `json:"definition,omitempty"`
	Locale      string `json:"locale"`
	ConceptID   string `json:"concept_id"`
}

func (s *MCPServer) handleTermSearch(ctx context.Context, req *mcp.CallToolRequest, input termSearchInput) (*mcp.CallToolResult, termSearchOutput, error) {
	// Phase 1: stub.
	return nil, termSearchOutput{
		Terms:  []termResult{},
		Status: "Termbase search integration will be available in Phase 2.",
	}, nil
}

type termAddInput struct {
	WorkspaceID string          `json:"workspace_id" jsonschema:"the workspace ID"`
	Terms       []termAddEntry  `json:"terms" jsonschema:"terms to add"`
}
type termAddEntry struct {
	Term       string `json:"term" jsonschema:"the term text"`
	Definition string `json:"definition,omitempty" jsonschema:"term definition"`
	Locale     string `json:"locale" jsonschema:"language code for this term"`
}
type termAddOutput struct {
	Added  int    `json:"added"`
	Status string `json:"status"`
}

func (s *MCPServer) handleTermAdd(ctx context.Context, req *mcp.CallToolRequest, input termAddInput) (*mcp.CallToolResult, termAddOutput, error) {
	// Phase 1: stub.
	return nil, termAddOutput{
		Added:  0,
		Status: "Termbase add integration will be available in Phase 2.",
	}, nil
}
