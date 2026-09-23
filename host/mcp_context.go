package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
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
		Description: "Ask what this project says about one word or phrase: what it is called here, whether " +
			"it is discouraged and what to say instead, and wording the project has already approved. " +
			"For everything that applies to a file, read context://<path> instead. " +
			"An empty answer means nothing is recorded about the word; if the project keeps to a spelling " +
			"for it, record that with context_observe. `attention` says what a person must act on, such as " +
			"context files nobody has imported.",
	}, a.handleContextSearch)

	registerContextResources(server, a)
}

// contextURIScheme is the address space the by-location primitive lives in.
const contextURIScheme = "context://"

// The two resource templates, by location and by profile name.
const (
	contextLocationTemplate = contextURIScheme + "{+path}{?format,project}"
	contextProfileTemplate  = contextURIScheme + contextProfilePrefix + "{name}{?format,project}"
)

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
	const description = "What applies when you write at one place: the voice, the words to use and to " +
		"avoid, and what has been suggested but not yet established. Read it before you change a file. " +
		"An answer with nothing recorded says so. Returns markdown; append `?format=json` for the " +
		"structured answer, which also names the point, the project and the revision that answered."

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "context-at-location",
		Title:       "Context at a location",
		URITemplate: contextLocationTemplate,
		MIMEType:    "text/markdown",
		Description: description + " The path is project-relative, e.g. `context://docs/guide.md`; " +
			"`?project=<path>` reads another project.",
	}, a.handleContextResource)

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "context-for-profile",
		Title:       "Context of a named profile",
		URITemplate: contextProfileTemplate,
		MIMEType:    "text/markdown",
		Description: description + " Names a profile instead of a file, e.g. `context://profile/marketing`.",
	}, a.handleContextResource)
}

// handleContextResource serves both `context://<path>` and
// `context://profile/<name>` from the same host resolution the CLI verb calls.
func (a *App) handleContextResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	request, asJSON, named, err := parseContextURI(uri)
	if err != nil {
		return nil, err
	}

	cmd, recipe, err := a.mcpCallCommand(ctx, "context", named)
	if err != nil {
		return nil, err
	}
	if recipe != "" && request.Path != "" && !filepath.IsAbs(request.Path) {
		request.Path = filepath.Join(filepath.Dir(recipe), request.Path)
	}
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

// parseContextURI reads a `context://` address into the request it names, the
// rendering it asked for, and the project it named (empty for the server's).
//
// The URI is split by hand rather than through url.Parse: under `context://`
// the first path segment would be read as an authority, so `context://docs/a.md`
// would arrive as host `docs` and path `/a.md` — and a host is lowercased,
// which silently renames a location on a case-sensitive filesystem.
func parseContextURI(uri string) (ContextPointRequest, bool, string, error) {
	rest, ok := strings.CutPrefix(uri, contextURIScheme)
	if !ok {
		return ContextPointRequest{}, false, "", mcp.ResourceNotFoundError(uri)
	}
	address, query, _ := strings.Cut(rest, "?")

	asJSON, project, err := contextParamsFromQuery(query)
	if err != nil {
		return ContextPointRequest{}, false, "", err
	}

	address, uerr := url.PathUnescape(address)
	if uerr != nil {
		return ContextPointRequest{}, false, "", fmt.Errorf("read %s: %w", uri, uerr)
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return ContextPointRequest{}, false, "", fmt.Errorf("read %s: name a location or `profile/<name>`", uri)
	}

	if name, isProfile := strings.CutPrefix(address, contextProfilePrefix); isProfile {
		if name == "" {
			return ContextPointRequest{}, false, "", fmt.Errorf("read %s: name a profile after `profile/`", uri)
		}
		return ContextPointRequest{Profile: name}, asJSON, project, nil
	}
	return ContextPointRequest{Path: address}, asJSON, project, nil
}

// contextParamsFromQuery reads the `format` and `project` parameters. An
// unrecognised rendering is an error rather than a silent fall back to
// markdown: a caller that asked for a shape it can parse must not be handed
// prose it cannot. `project` names the project the read acts on, the way the
// tools take a `project` argument; empty means the server's own.
func contextParamsFromQuery(query string) (bool, string, error) {
	if query == "" {
		return false, "", nil
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return false, "", fmt.Errorf("read context resource: %w", err)
	}
	project := strings.TrimSpace(values.Get("project"))
	switch format := values.Get("format"); format {
	case "", "markdown", "md", "text":
		return false, project, nil
	case "json":
		return true, project, nil
	default:
		return false, "", fmt.Errorf("unknown context rendering %q (want `markdown` or `json`)", format)
	}
}

// contextSearchInput is the MCP input for context_search.
//
// The store overrides mirror the older per-store tools this replaces. An MCP
// handler threads the server's bound recipe into the host command and reads
// the project's own store. An unbound server uses normal project discovery.
// A path here selects a STANDALONE store
// instead, for an agent pointed at a vocabulary or corpus outside the project.
type contextSearchInput struct {
	Query   string `json:"query" jsonschema:"the word or phrase to ask about"`
	Project string `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
	Locale  string `json:"locale,omitempty" jsonschema:"narrow results to one language (e.g. en, fr)"`
	Limit   int    `json:"limit,omitempty" jsonschema:"max results per group (default 10)"`
	Terms   string `json:"terms,omitempty" jsonschema:"path to a standalone terms store (default: the project's own store)"`
	Memory  string `json:"memory,omitempty" jsonschema:"path to a standalone content memory (default: the project's own store)"`
}

func (a *App) handleContextSearch(ctx context.Context, _ *mcp.CallToolRequest, in contextSearchInput) (*mcp.CallToolResult, *ContextSearchResult, error) {
	// One assembly shared with `kapi context search` (ContextSearchSourcesFor).
	// The call's project governs project resolution; a non-empty terms/memory
	// path selects a standalone store instead.
	cmd, _, err := a.mcpCallCommand(ctx, "context-search", in.Project)
	if err != nil {
		return nil, nil, err
	}
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
