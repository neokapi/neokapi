package host

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"path/filepath"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
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
// The store paths mirror the older per-store tools this replaces: an MCP
// handler has no cobra Command, so it cannot use the project-resolving openers
// the CLI does. Both default to the conventional project filenames.
type contextSearchInput struct {
	Query  string `json:"query" jsonschema:"the word or phrase to ask about"`
	Locale string `json:"locale,omitempty" jsonschema:"narrow results to one language (e.g. en, fr)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max results per group (default 10)"`
	Terms  string `json:"terms,omitempty" jsonschema:"path to the terms store (default .kapi/termbase.db)"`
	Memory string `json:"memory,omitempty" jsonschema:"path to the content memory (default .kapi/tm.db)"`
	// The default paths spell the retained identifiers, because they name files
	// that exist on disk today. The vocabulary in the descriptions is the
	// settled one — "terms store", "content memory" — and the filenames are
	// quoted verbatim, which is the rule for a retained identifier.
}

func (a *App) handleContextSearch(ctx context.Context, _ *mcp.CallToolRequest, in contextSearchInput) (*mcp.CallToolResult, *ContextSearchResult, error) {
	src := ContextSearchSources{Scope: ScopeProject}

	// A store that is absent is ordinary, not an error: a project with
	// terminology but no content memory is a normal project. SearchContext
	// reports the gap in its notes, so a caller can tell "no answer" from
	// "nowhere to look" instead of reading an empty result as a verdict.
	if tb, err := terms.NewSQLiteStore(firstNonEmpty(in.Terms, filepath.Join(project.StateDirName, project.TermsFileName))); err == nil {
		defer tb.Close()
		src.Terms = tb
	}
	if tm, err := memory.NewSQLiteStore(firstNonEmpty(in.Memory, filepath.Join(project.StateDirName, project.MemoryFileName))); err == nil {
		defer tm.Close()
		src.Memory = tm
	}

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
