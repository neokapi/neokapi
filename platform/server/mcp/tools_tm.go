package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTMTools registers translation memory MCP tools.
// Phase 1: provides tool stubs. Phase 2: integrates with workspace TM stores.
func (s *MCPServer) registerTMTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "tm_search",
		Description: "Search translation memory for matches. Returns fuzzy and exact matches with scores.",
	}, s.handleTMSearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "tm_import",
		Description: "Import translation memory entries from provided data.",
	}, s.handleTMImport)
}

type tmSearchInput struct {
	WorkspaceID  string `json:"workspace_id" jsonschema:"the workspace ID"`
	Text         string `json:"text" jsonschema:"source text to search for"`
	SourceLocale string `json:"source_locale" jsonschema:"source language code"`
	TargetLocale string `json:"target_locale" jsonschema:"target language code"`
	Limit        int    `json:"limit,omitempty" jsonschema:"max results (default 5)"`
}
type tmSearchOutput struct {
	Matches []tmMatch `json:"matches"`
	Status  string    `json:"status"`
}
type tmMatch struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Score      float64 `json:"score"`
	MatchType  string  `json:"match_type"` // "exact" or "fuzzy"
}

func (s *MCPServer) handleTMSearch(ctx context.Context, req *mcp.CallToolRequest, input tmSearchInput) (*mcp.CallToolResult, tmSearchOutput, error) {
	// Phase 1: stub. Phase 2: integrates with workspace TM (sievepen) store.
	return nil, tmSearchOutput{
		Matches: []tmMatch{},
		Status:  "TM search integration will be available in Phase 2.",
	}, nil
}

type tmImportInput struct {
	WorkspaceID  string     `json:"workspace_id" jsonschema:"the workspace ID"`
	SourceLocale string     `json:"source_locale" jsonschema:"source language code"`
	TargetLocale string     `json:"target_locale" jsonschema:"target language code"`
	Entries      []tmEntry  `json:"entries" jsonschema:"TM entries to import"`
}
type tmEntry struct {
	Source string `json:"source"`
	Target string `json:"target"`
}
type tmImportOutput struct {
	Imported int    `json:"imported"`
	Status   string `json:"status"`
}

func (s *MCPServer) handleTMImport(ctx context.Context, req *mcp.CallToolRequest, input tmImportInput) (*mcp.CallToolResult, tmImportOutput, error) {
	// Phase 1: stub.
	return nil, tmImportOutput{
		Imported: 0,
		Status:   "TM import integration will be available in Phase 2.",
	}, nil
}
