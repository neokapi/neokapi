package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// registerTermsTools registers terminology management MCP tools.
func (s *MCPServer) registerTermsTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "term_search",
		Description: "Search the workspace terms store for matching terms.",
	}, s.handleTermSearch)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "term_add",
		Description: "Add new term entries to the workspace terms store.",
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
		return nil, termSearchOutput{}, fmt.Errorf("get terms: %w", err)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	concepts, total, err := tb.Search(ctx, input.Query, model.LocaleID(input.Locale), "", 0, limit)
	if err != nil {
		return nil, termSearchOutput{}, fmt.Errorf("search terms: %w", err)
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
	// DoNotTranslate marks a term that stays the same string in every language.
	// Creating such a concept is governed, so it is proposed for review.
	DoNotTranslate bool `json:"do_not_translate,omitempty" jsonschema:"keep the term verbatim in every language; the term is proposed for review in a change-set rather than added"`
}
type termAddOutput struct {
	Added int `json:"added"`
	// Proposed counts the do-not-translate terms proposed for review, and
	// ChangeSetIDs names the change-sets that hold them.
	Proposed     int      `json:"proposed,omitempty"`
	ChangeSetIDs []string `json:"change_set_ids,omitempty"`
}

// ChangeSetProposer opens a submitted change-set for a governed change a tool
// cannot apply directly, such as a term kept verbatim in every language.
type ChangeSetProposer interface {
	ProposeConcept(ctx context.Context, workspaceID, actor string, concept terms.Concept) (changeSetID string, err error)
}

// WithChangeSetProposer lets the terms tools propose governed changes for
// review. Without it, a governed change is refused.
func WithChangeSetProposer(p ChangeSetProposer) Option {
	return func(s *MCPServer) { s.proposer = p }
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
		return nil, termAddOutput{}, fmt.Errorf("get terms: %w", err)
	}

	var out termAddOutput
	for _, t := range input.Terms {
		concept := terms.Concept{
			ID:         id.New(),
			Definition: t.Definition,
			Terms: []terms.Term{
				{
					Text:   t.Term,
					Locale: model.LocaleID(t.Locale),
					Status: model.TermApproved,
				},
			},
		}
		if t.DoNotTranslate {
			// A term kept verbatim in every language is a governed creation, so
			// it is proposed for review rather than added.
			if s.proposer == nil {
				return nil, out, fmt.Errorf("term %q: a do-not-translate term is proposed in a change-set, and this server cannot open one", t.Term)
			}
			concept.DoNotTranslate = true
			csID, err := s.proposer.ProposeConcept(ctx, input.WorkspaceID, callerID(req), concept)
			if err != nil {
				return nil, out, fmt.Errorf("propose term %q: %w", t.Term, err)
			}
			out.Proposed++
			out.ChangeSetIDs = append(out.ChangeSetIDs, csID)
			continue
		}
		if err := tb.AddConcept(ctx, concept); err != nil {
			return nil, out, fmt.Errorf("add term: %w", err)
		}
		out.Added++
	}

	return nil, out, nil
}
