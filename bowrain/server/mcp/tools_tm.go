package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

// registerTMTools registers translation memory MCP tools.
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
	Total   int       `json:"total"`
}
type tmMatch struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"`
}

func (s *MCPServer) handleTMSearch(ctx context.Context, req *mcp.CallToolRequest, input tmSearchInput) (*mcp.CallToolResult, tmSearchOutput, error) {
	if s.tmResolver == nil {
		return nil, tmSearchOutput{}, fmt.Errorf("translation memory not configured")
	}
	if input.Text == "" {
		return nil, tmSearchOutput{}, fmt.Errorf("text is required")
	}

	tm, err := s.tmResolver.GetTM(input.WorkspaceID)
	if err != nil {
		return nil, tmSearchOutput{}, fmt.Errorf("get TM store: %w", err)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}

	matches, err := tm.LookupText(
		input.Text,
		model.LocaleID(input.SourceLocale),
		model.LocaleID(input.TargetLocale),
		sievepen.LookupOptions{
			MaxResults: limit,
			MinScore:   0.5,
		},
	)
	if err != nil {
		return nil, tmSearchOutput{}, fmt.Errorf("TM lookup: %w", err)
	}

	result := make([]tmMatch, 0, len(matches))
	for _, m := range matches {
		matchType := "fuzzy"
		if m.Score >= 1.0 {
			matchType = "exact"
		}
		result = append(result, tmMatch{
			Source:    m.Entry.Source.Text(),
			Target:    m.Entry.Target.Text(),
			Score:     m.Score,
			MatchType: matchType,
		})
	}

	return nil, tmSearchOutput{Matches: result, Total: len(result)}, nil
}

type tmImportInput struct {
	WorkspaceID  string    `json:"workspace_id" jsonschema:"the workspace ID"`
	SourceLocale string    `json:"source_locale" jsonschema:"source language code"`
	TargetLocale string    `json:"target_locale" jsonschema:"target language code"`
	Entries      []tmEntry `json:"entries" jsonschema:"TM entries to import"`
}
type tmEntry struct {
	Source string `json:"source"`
	Target string `json:"target"`
}
type tmImportOutput struct {
	Imported int `json:"imported"`
}

func (s *MCPServer) handleTMImport(ctx context.Context, req *mcp.CallToolRequest, input tmImportInput) (*mcp.CallToolResult, tmImportOutput, error) {
	if s.tmResolver == nil {
		return nil, tmImportOutput{}, fmt.Errorf("translation memory not configured")
	}
	if len(input.Entries) == 0 {
		return nil, tmImportOutput{Imported: 0}, nil
	}

	tm, err := s.tmResolver.GetTM(input.WorkspaceID)
	if err != nil {
		return nil, tmImportOutput{}, fmt.Errorf("get TM store: %w", err)
	}

	imported := 0
	for _, e := range input.Entries {
		entry := sievepen.TMEntry{
			Source:       model.NewFragment(e.Source),
			Target:       model.NewFragment(e.Target),
			SourceLocale: model.LocaleID(input.SourceLocale),
			TargetLocale: model.LocaleID(input.TargetLocale),
		}
		if err := tm.Add(entry); err != nil {
			return nil, tmImportOutput{}, fmt.Errorf("add TM entry: %w", err)
		}
		imported++
	}

	return nil, tmImportOutput{Imported: imported}, nil
}
