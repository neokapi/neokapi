package host

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/model"
)

// The context retrieval surface (AD-037), MCP half.
//
// This is a wrapper over the same host.SearchContext the `kapi context search`
// verb calls. Keeping one implementation under both is the whole point: the
// agent skill drives the CLI, so a capability that exists on only one surface
// teaches an assistant a kapi that the other half does not have.

func init() { RegisterMCPToolFactory(registerContextMCPTools) }

func registerContextMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "context_search",
		Description: "Ask what this project's content context says about a word or phrase — " +
			"what it is called here, whether it is discouraged and what to say instead, " +
			"and wording the project has already approved. One question across every store " +
			"the project binds; you do not need to know which one holds the answer. " +
			"Ask BEFORE writing: learning the same fact from a failing check afterwards is " +
			"the expensive route. Results are grouped by kind, and say what could not be reached.",
	}, a.handleContextSearch)
}

// contextSearchInput is the MCP input for context_search.
//
// The store overrides mirror the older per-store tools this replaces. An MCP
// handler has no cobra Command, so it resolves the project the way every
// flagless caller does — KAPI_PROJECT, else the upward walk from cwd — and
// reads the project's own store. A path here selects a STANDALONE store
// instead, for an agent pointed at a vocabulary or corpus outside the project.
type contextSearchInput struct {
	Query  string `json:"query" jsonschema:"the word or phrase to ask about"`
	Locale string `json:"locale,omitempty" jsonschema:"narrow results to one language (e.g. en, fr)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max results per group (default 10)"`
	Terms  string `json:"terms,omitempty" jsonschema:"path to a standalone terms store (default: the project's own store)"`
	Memory string `json:"memory,omitempty" jsonschema:"path to a standalone content memory (default: the project's own store)"`
}

func (a *App) handleContextSearch(ctx context.Context, _ *mcp.CallToolRequest, in contextSearchInput) (*mcp.CallToolResult, *ContextSearchResult, error) {
	// One assembly shared with `kapi context search` (ContextSearchSourcesFor).
	// A bare command carries the context for the project resolution the flagless
	// openers do (KAPI_PROJECT, else the upward walk from cwd); a non-empty
	// terms/memory path selects a standalone store instead.
	cmd := NewEnvCommand(ctx, "context-search")
	src, cleanup := a.ContextSearchSourcesFor(cmd, in.Terms, in.Memory)
	defer cleanup()

	res, err := SearchContext(ctx, src, ContextSearchRequest{
		Query:  in.Query,
		Locale: model.LocaleID(in.Locale),
		Limit:  in.Limit,
	})
	if err != nil {
		return nil, nil, err
	}
	return nil, res, nil
}
