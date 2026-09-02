package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// The context retrieval surface (AD-037), MCP half.
//
// Both primitives are wrappers over the same host functions the `kapi context`
// verbs call — SearchContext by content, ResolveContextAt by location. Keeping
// one implementation under both is the whole point: the agent skill drives the
// CLI, so a capability that exists on only one surface teaches an assistant a
// kapi that the other half does not have.
//
// The two are shaped differently on the wire because the questions are. Asking
// what a word means is a call with arguments, so it is a TOOL. Asking what
// applies at a location is reading something that already exists at an address,
// so it is a RESOURCE — which is also what lets the rendering be a property of
// the read (a mime type) rather than a second entry point.

func init() { RegisterMCPToolFactory(registerContextMCPTools) }

func registerContextMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "context_search",
		Description: "Ask what this project's content context says about a word or phrase: " +
			"what it is called here, whether it is discouraged and what to say instead, " +
			"and wording the project has already approved. One question across every store " +
			"the project binds; you do not need to know which one holds the answer. " +
			"Ask BEFORE writing: learning the same fact from a failing check afterwards is " +
			"the expensive route. Results are grouped by kind, and say what could not be reached.",
	}, a.handleContextSearch)

	registerContextResources(server, a)
}

// contextURIScheme is the address space the by-location primitive lives in.
const contextURIScheme = "context://"

// contextProfilePrefix reserves one path under the scheme for the by-name form.
// A location genuinely called `profile/…` is therefore not addressable by path;
// the two address forms are one primitive, and the reservation is what lets a
// single scheme carry both.
const contextProfilePrefix = "profile/"

// contextResourceTemplates are the two addresses the by-location primitive
// answers at, as a client discovers them. Both carry the same handler: which
// template the SDK matched decides nothing, because the URI itself says which
// form was asked for, and a dispatch that depended on template ordering would
// be a coin toss on `context://profile/x`, which both templates match.
func registerContextResources(server *mcp.Server, a *App) {
	const description = "What this project's context says applies at one place: the voice profile " +
		"in force with its full guidance, the terms bound there, and the governance windows " +
		"around them. Read this BEFORE writing or editing content at that location. " +
		"Returns markdown by default; append `?format=json` for the structured shape."

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "context-at-location",
		Title:       "Context at a location",
		URITemplate: contextURIScheme + "{+path}{?format}",
		MIMEType:    "text/markdown",
		Description: description + " The path is project-relative, e.g. `context://docs/guide.md`.",
	}, a.handleContextResource)

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "context-for-profile",
		Title:       "Context of a named profile",
		URITemplate: contextURIScheme + contextProfilePrefix + "{name}{?format}",
		MIMEType:    "text/markdown",
		Description: description + " Addresses a governance profile by name, for a caller with " +
			"no file in hand, e.g. `context://profile/marketing`.",
	}, a.handleContextResource)
}

// handleContextResource serves both `context://<path>` and
// `context://profile/<name>` from the same host resolution the CLI verb calls.
func (a *App) handleContextResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	request, asJSON, err := parseContextURI(uri)
	if err != nil {
		return nil, err
	}

	cmd := NewEnvCommand(ctx, "context")
	src, cleanup := a.ContextSourcesAt(cmd, request)
	defer cleanup()

	answer, err := ResolveContextAt(ctx, src, request)
	if err != nil {
		return nil, err
	}

	body, mime, err := renderContextAnswer(answer, asJSON)
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
		{URI: uri, MIMEType: mime, Text: body},
	}}, nil
}

// renderContextAnswer renders one answer in the form the read asked for. The
// mime type carries the rendering — prose for a model, or structure for a
// program — which is why this is a property of the response rather than a
// second address.
func renderContextAnswer(answer *ContextAnswer, asJSON bool) (string, string, error) {
	if asJSON {
		raw, err := json.MarshalIndent(answer, "", "  ")
		if err != nil {
			return "", "", fmt.Errorf("render context: %w", err)
		}
		return string(raw) + "\n", "application/json", nil
	}
	var buf bytes.Buffer
	if err := answer.FormatText(&buf); err != nil {
		return "", "", err
	}
	return buf.String(), "text/markdown", nil
}

// parseContextURI reads a `context://` address into the request it names and
// the rendering it asked for.
//
// The URI is split by hand rather than through url.Parse: under `context://`
// the first path segment would be read as an authority, so `context://docs/a.md`
// would arrive as host `docs` and path `/a.md` — and a host is lowercased,
// which silently renames a location on a case-sensitive filesystem.
func parseContextURI(uri string) (ContextPointRequest, bool, error) {
	rest, ok := strings.CutPrefix(uri, contextURIScheme)
	if !ok {
		return ContextPointRequest{}, false, mcp.ResourceNotFoundError(uri)
	}
	address, query, _ := strings.Cut(rest, "?")

	asJSON, err := contextRenderingFromQuery(query)
	if err != nil {
		return ContextPointRequest{}, false, err
	}

	address, uerr := url.PathUnescape(address)
	if uerr != nil {
		return ContextPointRequest{}, false, fmt.Errorf("read %s: %w", uri, uerr)
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return ContextPointRequest{}, false, fmt.Errorf("read %s: name a location or `profile/<name>`", uri)
	}

	if name, isProfile := strings.CutPrefix(address, contextProfilePrefix); isProfile {
		if name == "" {
			return ContextPointRequest{}, false, fmt.Errorf("read %s: name a profile after `profile/`", uri)
		}
		return ContextPointRequest{Profile: name}, asJSON, nil
	}
	return ContextPointRequest{Path: address}, asJSON, nil
}

// contextRenderingFromQuery reads the `format` parameter. An unrecognised value
// is an error rather than a silent fall back to markdown: a caller that asked
// for a shape it can parse must not be handed prose it cannot.
func contextRenderingFromQuery(query string) (bool, error) {
	if query == "" {
		return false, nil
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return false, fmt.Errorf("read context resource: %w", err)
	}
	switch format := values.Get("format"); format {
	case "", "markdown", "md", "text":
		return false, nil
	case "json":
		return true, nil
	default:
		return false, fmt.Errorf("unknown context rendering %q (want `markdown` or `json`)", format)
	}
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

	// A locale narrows the search, so it has to be the tag the store holds
	// rather than the one the caller happened to type.
	var scope model.LocaleID
	if in.Locale != "" {
		id, lerr := locale.Canonical(in.Locale)
		if lerr != nil {
			return nil, nil, fmt.Errorf("locale: %w", lerr)
		}
		scope = id
	}

	res, err := SearchContext(ctx, src, ContextSearchRequest{
		Query:  in.Query,
		Locale: scope,
		Limit:  in.Limit,
	})
	if err != nil {
		return nil, nil, err
	}
	return nil, res, nil
}
