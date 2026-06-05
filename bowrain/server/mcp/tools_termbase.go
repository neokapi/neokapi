package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// registerTermbaseTools registers terminology management MCP tools.
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
	Terms []termResult `json:"terms"`
	Total int          `json:"total"`
}
type termResult struct {
	Term       string `json:"term"`
	Definition string `json:"definition,omitempty"`
	Locale     string `json:"locale"`
	ConceptID  string `json:"concept_id"`
}

func (s *MCPServer) handleTermSearch(ctx context.Context, req *mcp.CallToolRequest, input termSearchInput) (*mcp.CallToolResult, termSearchOutput, error) {
	if s.tbResolver == nil {
		return nil, termSearchOutput{}, errors.New("terminology base not configured")
	}
	if input.Query == "" {
		return nil, termSearchOutput{}, errors.New("query is required")
	}
	if err := s.authorizeWorkspace(ctx, req, input.WorkspaceID); err != nil {
		return nil, termSearchOutput{}, err
	}

	tb, err := s.tbResolver.GetTB(input.WorkspaceID)
	if err != nil {
		return nil, termSearchOutput{}, fmt.Errorf("get termbase: %w", err)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	concepts, total, err := tb.Search(ctx, input.Query, model.LocaleID(input.Locale), "", 0, limit)
	if err != nil {
		return nil, termSearchOutput{}, fmt.Errorf("search termbase: %w", err)
	}

	var results []termResult
	for _, c := range concepts {
		for _, t := range c.Terms {
			if input.Locale != "" && string(t.Locale) != input.Locale {
				continue
			}
			results = append(results, termResult{
				Term:       t.Text,
				Definition: c.Definition,
				Locale:     string(t.Locale),
				ConceptID:  c.ID,
			})
		}
	}

	return nil, termSearchOutput{Terms: results, Total: total}, nil
}

type termAddInput struct {
	WorkspaceID string         `json:"workspace_id" jsonschema:"the workspace ID"`
	Terms       []termAddEntry `json:"terms" jsonschema:"terms to add"`
}
type termAddEntry struct {
	Term       string `json:"term" jsonschema:"the term text"`
	Definition string `json:"definition,omitempty" jsonschema:"term definition"`
	Locale     string `json:"locale" jsonschema:"language code for this term"`
}
type termAddOutput struct {
	Added int `json:"added"`
}

func (s *MCPServer) handleTermAdd(ctx context.Context, req *mcp.CallToolRequest, input termAddInput) (*mcp.CallToolResult, termAddOutput, error) {
	if s.tbResolver == nil {
		return nil, termAddOutput{}, errors.New("terminology base not configured")
	}
	if len(input.Terms) == 0 {
		return nil, termAddOutput{Added: 0}, nil
	}
	if err := s.authorizeWorkspace(ctx, req, input.WorkspaceID); err != nil {
		return nil, termAddOutput{}, err
	}

	tb, err := s.tbResolver.GetTB(input.WorkspaceID)
	if err != nil {
		return nil, termAddOutput{}, fmt.Errorf("get termbase: %w", err)
	}

	added := 0
	for _, t := range input.Terms {
		concept := termbase.Concept{
			Definition: t.Definition,
			Terms: []termbase.Term{
				{
					Text:   t.Term,
					Locale: model.LocaleID(t.Locale),
					Status: model.TermApproved,
				},
			},
		}
		if err := tb.AddConcept(ctx, concept); err != nil {
			return nil, termAddOutput{}, fmt.Errorf("add term: %w", err)
		}
		added++
	}

	return nil, termAddOutput{Added: added}, nil
}
