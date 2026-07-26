package mcp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// registerMemoryTools registers content memory MCP tools.
func (s *MCPServer) registerMemoryTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "tm_search",
		Description: "Search content memory for matches. Returns fuzzy and exact matches with scores.",
	}, s.handleMemorySearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "tm_import",
		Description: "Import content memory entries from provided data.",
	}, s.handleMemoryImport)
}

type memorySearchInput struct {
	WorkspaceID  string `json:"workspace_id" jsonschema:"the workspace ID"`
	Text         string `json:"text" jsonschema:"source text to search for"`
	SourceLocale string `json:"source_locale" jsonschema:"source language code"`
	TargetLocale string `json:"target_locale" jsonschema:"target language code"`
	Limit        int    `json:"limit,omitempty" jsonschema:"max results (default 5)"`
}
type memorySearchOutput struct {
	Matches []memoryMatch `json:"matches"`
	Total   int           `json:"total"`
}
type memoryMatch struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"`
}

func (s *MCPServer) handleMemorySearch(ctx context.Context, req *mcp.CallToolRequest, input memorySearchInput) (*mcp.CallToolResult, memorySearchOutput, error) {
	if s.memoryResolver == nil {
		return nil, memorySearchOutput{}, errors.New("content memory not configured")
	}
	if input.Text == "" {
		return nil, memorySearchOutput{}, errors.New("text is required")
	}
	if err := s.authorizeWorkspace(ctx, req, input.WorkspaceID); err != nil {
		return nil, memorySearchOutput{}, err
	}

	tm, err := s.memoryResolver.GetMemory(input.WorkspaceID)
	if err != nil {
		return nil, memorySearchOutput{}, fmt.Errorf("get content-memory store: %w", err)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}

	matches, err := tm.LookupText(
		ctx,
		input.Text,
		model.LocaleID(input.SourceLocale),
		model.LocaleID(input.TargetLocale),
		memory.LookupOptions{
			MaxResults: limit,
			MinScore:   0.5,
		},
	)
	if err != nil {
		return nil, memorySearchOutput{}, fmt.Errorf("content-memory lookup: %w", err)
	}

	srcLoc := model.LocaleID(input.SourceLocale)
	tgtLoc := model.LocaleID(input.TargetLocale)
	result := make([]memoryMatch, 0, len(matches))
	for _, m := range matches {
		matchType := "fuzzy"
		if m.Score >= 1.0 {
			matchType = "exact"
		}
		result = append(result, memoryMatch{
			Source:    m.Entry.VariantText(srcLoc),
			Target:    m.Entry.VariantText(tgtLoc),
			Score:     m.Score,
			MatchType: matchType,
		})
	}

	return nil, memorySearchOutput{Matches: result, Total: len(result)}, nil
}

type memoryImportInput struct {
	WorkspaceID  string        `json:"workspace_id" jsonschema:"the workspace ID"`
	SourceLocale string        `json:"source_locale" jsonschema:"source language code"`
	TargetLocale string        `json:"target_locale" jsonschema:"target language code"`
	Entries      []memoryEntry `json:"entries" jsonschema:"content-memory entries to import"`
}
type memoryEntry struct {
	Source string `json:"source"`
	Target string `json:"target"`
}
type memoryImportOutput struct {
	Imported int `json:"imported"`
}

func (s *MCPServer) handleMemoryImport(ctx context.Context, req *mcp.CallToolRequest, input memoryImportInput) (*mcp.CallToolResult, memoryImportOutput, error) {
	if s.memoryResolver == nil {
		return nil, memoryImportOutput{}, errors.New("content memory not configured")
	}
	if len(input.Entries) == 0 {
		return nil, memoryImportOutput{Imported: 0}, nil
	}
	if err := s.authorizeWorkspace(ctx, req, input.WorkspaceID); err != nil {
		return nil, memoryImportOutput{}, err
	}

	tm, err := s.memoryResolver.GetMemory(input.WorkspaceID)
	if err != nil {
		return nil, memoryImportOutput{}, fmt.Errorf("get content-memory store: %w", err)
	}

	srcLoc := model.LocaleID(input.SourceLocale)
	tgtLoc := model.LocaleID(input.TargetLocale)
	now := time.Now()
	imported := 0
	for i, e := range input.Entries {
		entry := memory.Entry{
			ID: fmt.Sprintf("mcp-import-%d-%d", now.UnixNano(), i),
			Variants: map[model.LocaleID][]model.Run{
				srcLoc: {{Text: &model.TextRun{Text: e.Source}}},
				tgtLoc: {{Text: &model.TextRun{Text: e.Target}}},
			},
			HintSrcLang: srcLoc,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := tm.Add(ctx, entry); err != nil {
			return nil, memoryImportOutput{}, fmt.Errorf("add content-memory entry: %w", err)
		}
		imported++
	}

	return nil, memoryImportOutput{Imported: imported}, nil
}
