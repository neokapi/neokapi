package host

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// init exposes a CURATED set of framework tools over MCP as schema-derived text
// tools. Unlike the hand-authored MCP tools (which operate on file paths), these
// run a single tool over a snippet of text the caller supplies, with the tool's
// own parameters projected straight from its schema.
//
// It used to expose every CLI-visible tool, which is how the agent surface grew
// to 51 tools nobody chose. See agentFacingTools.
func init() {
	RegisterMCPToolFactory(registerFrameworkMCPTools)
}

// ResolveMCPProject resolves the active project (git-style upward walk, honoring
// KAPI_NO_PROJECT and an explicit -p) and, when one is found, loads it into
// a.ProjectContext so the MCP factories can scope themselves to it. Failure to
// resolve or load is non-fatal — the server simply runs in ad-hoc mode.
func (a *App) ResolveMCPProject(cmd Command) {
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return
	}
	proj, err := project.Load(path)
	if err != nil {
		return
	}
	a.ProjectContext = project.NewProjectContext(proj, path)
}

// registerFrameworkMCPTools registers one MCP tool per curated framework tool.
// Project scoping still applies on top: in project mode the set is further
// filtered to the project's allowed sources and the project's first target
// language becomes the default.
//
// agentFacingTools is the curated set of registry tools worth offering to an
// assistant (AD-037). Exposure is a decision with a name attached, not a
// consequence of a tool being CLI-visible.
//
// The unfiltered surface was 51 tools in ad-hoc mode, of which about 12 were
// deliberately authored. The rest arrived because someone added a pipeline step
// — `whitespace-correct`, `encoding-detect`, `inline-codes-remove`,
// `xml-validation` — which no caller should be assembling by hand. Worse, the
// loop's own job was to invite hand-cranking the loop: `recycle` and
// `diff-leverage` are what `up` does automatically, and content-memory
// recycling is invisible by design.
//
// `kapi mcp --all-tools` restores the full generated surface for debugging.
// Only registry tools are listed here — the hand-authored porcelain
// (check_file, apply_edits, up, context_search, …) registers itself elsewhere
// and is unaffected. Anything with a porcelain equivalent is deliberately
// absent: two names for one job means the caller picks wrong half the time.
// `qa` is check_text/check_file, and `pseudo-translate` is pseudo_translate.
var agentFacingTools = map[string]bool{
	// Producing content the caller cannot produce itself.
	"translate": true,
	// Checks with no porcelain equivalent.
	"term-check": true,
	// Redaction is a deliberate act on content, not a pipeline step.
	"redact": true,
}

// neverAgentFacing are tools withheld even under KAPI_MCP_ALL_TOOLS.
//
// "Show me every tool" and "let a caller execute arbitrary commands and
// JavaScript" are different classes of decision. Bundling them would mean
// enabling the first silently grants the second to any MCP client. Neither is
// removed from the CLI: `kapi exec` still runs both.
var neverAgentFacing = map[string]bool{
	"external-command": true, // "Executes an external command on block text"
	"script":           true, // "Run a JavaScript processing script on each part"
}

// MCPSurface widens the exposed tool set. It is set from flags on `kapi mcp`
// (--all-tools / --all-flows / --all) rather than an environment variable: the
// surface an assistant sees is a property of how the server was started, so it
// belongs on the command that starts it, where `--help` lists it.
type MCPSurface struct {
	// AllTools exposes every CLI-visible registry tool instead of the curated
	// set — pipeline steps, format internals, one-off transforms.
	AllTools bool
	// AllFlows exposes the flow-running verbs.
	AllFlows bool
}

func registerFrameworkMCPTools(server *mcp.Server, a *App) {
	if a.ToolReg == nil {
		return
	}
	entries, defaultTargetLang := scopeFrameworkTools(a.ToolReg.CLITools(), a.ProjectContext)
	exposeAll := a.MCPSurface.AllTools

	t := a.T()
	for _, entry := range entries {
		name := string(entry.Info.Name)
		if neverAgentFacing[name] {
			continue
		}
		if !exposeAll && !agentFacingTools[name] {
			continue
		}
		scope := "tools." + name
		desc := t.T(i18n.Scope(scope+".description"), entry.Info.Description)
		if desc == "" {
			desc = t.T(i18n.Scope(scope+".DisplayName"), entry.Info.DisplayName)
		}

		inputSchema, err := frameworkToolInputSchema(entry.Schema)
		if err != nil {
			continue // a tool whose schema can't be projected is simply not exposed
		}

		server.AddTool(&mcp.Tool{
			Name:        name,
			Description: desc,
			InputSchema: inputSchema,
		}, a.frameworkMCPHandler(registry.ToolID(name), defaultTargetLang))
	}
}

// scopeFrameworkTools applies project vs ad-hoc scoping to the CLI tool set.
// With no project context it returns every entry and no language default. In a
// project it keeps only tools whose source the project declares (built-ins are
// always allowed) and surfaces the project's first target language as the
// default for translate-like tools.
func scopeFrameworkTools(entries []registry.CLIToolEntry, ctx *project.ProjectContext) ([]registry.CLIToolEntry, string) {
	if ctx == nil {
		return entries, ""
	}
	allowed := make(map[string]bool, len(ctx.AllowedSources))
	for _, s := range ctx.AllowedSources {
		allowed[s] = true
	}
	scoped := make([]registry.CLIToolEntry, 0, len(entries))
	for _, e := range entries {
		src := e.Info.Source
		if src == "" {
			src = registry.SourceBuiltIn
		}
		if allowed[src] {
			scoped = append(scoped, e)
		}
	}
	var defaultTargetLang string
	if len(ctx.TargetLocales) > 0 {
		defaultTargetLang = string(ctx.TargetLocales[0])
	}
	return scoped, defaultTargetLang
}

// frameworkToolInputSchema projects a tool's ComponentSchema into the MCP input
// schema: the tool's own parameters, plus a required `text` field (the content
// to process) and an optional `target_lang`. MCP requires a top-level object
// schema, which ComponentSchema already is.
func frameworkToolInputSchema(s *schema.ComponentSchema) (json.RawMessage, error) {
	base := map[string]any{"type": "object", "properties": map[string]any{}}
	if s != nil {
		raw, err := json.Marshal(s)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &base); err != nil {
			return nil, err
		}
	}
	props, ok := base["properties"].(map[string]any)
	if !ok || props == nil {
		props = map[string]any{}
		base["properties"] = props
	}
	props["text"] = map[string]any{
		"type":        "string",
		"description": "The content (text) to run the tool over.",
	}
	if _, exists := props["target_lang"]; !exists {
		props["target_lang"] = map[string]any{
			"type":        "string",
			"description": "BCP-47 target language (e.g. fr, de). Defaults to the project's target when run inside a project.",
		}
	}
	base["type"] = "object"
	base["required"] = []any{"text"}
	return json.Marshal(base)
}

// frameworkMCPHandler builds the untyped MCP handler for one framework tool: it
// splits `text`/`target_lang` from the remaining arguments (the tool config),
// instantiates the tool via the registry (running the credential preprocessor),
// runs it over the text, and returns the serialized result block.
func (a *App) frameworkMCPHandler(name registry.ToolID, defaultTargetLang string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, fmt.Errorf("decode arguments: %w", err)
			}
		}
		text, _ := args["text"].(string)
		if text == "" {
			return nil, fmt.Errorf("%q requires a non-empty 'text' argument", name)
		}
		targetLang, _ := args["target_lang"].(string)
		if targetLang == "" {
			targetLang = defaultTargetLang
		}
		// An agent may spell a locale any way; the tool it drives is given the
		// canonical tag, and a target_lang that names no language is refused
		// rather than translated into.
		if targetLang != "" {
			id, err := locale.Canonical(targetLang)
			if err != nil {
				return nil, fmt.Errorf("target_lang: %w", err)
			}
			targetLang = string(id)
		}

		config := make(map[string]any, len(args))
		for k, v := range args {
			if k == "text" || k == "target_lang" {
				continue
			}
			config[k] = v
		}

		tl, err := a.ToolReg.NewToolWithConfig(name, config, targetLang)
		if err != nil {
			return nil, err
		}
		out, err := runToolOverText(ctx, tl, text)
		if err != nil {
			return nil, err
		}
		out.Tool = string(name)

		payload, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: string(payload)}},
			StructuredContent: out,
		}, nil
	}
}

// frameworkToolOutput is the serialized result of running a tool over one block.
// It captures every channel a tool can write to: target translations, rewritten
// source, properties, run-anchored overlays, and block annotations.
type frameworkToolOutput struct {
	Tool        string                     `json:"tool"`
	Source      string                     `json:"source,omitempty"`
	Targets     map[string]string          `json:"targets,omitempty"`
	Properties  map[string]string          `json:"properties,omitempty"`
	Overlays    json.RawMessage            `json:"overlays,omitempty"`
	Annotations map[string]json.RawMessage `json:"annotations,omitempty"`
}

// runToolOverText runs a single block tool over text and serializes the result.
// It mirrors the streaming contract used everywhere else: feed one block part,
// drain the output, then read the (in-place mutated) result block.
func runToolOverText(ctx context.Context, t tool.Tool, text string) (*frameworkToolOutput, error) {
	block := model.NewBlock("mcp", text)
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- t.Process(ctx, in, out)
	}()

	result := block
	for p := range out {
		if b, ok := p.Resource.(*model.Block); ok {
			result = b
		}
	}
	if err := <-errc; err != nil {
		return nil, err
	}
	return serializeBlock(result), nil
}

// serializeBlock renders a processed block into the JSON-friendly output shape.
func serializeBlock(b *model.Block) *frameworkToolOutput {
	out := &frameworkToolOutput{Source: model.RunsText(b.Source)}
	if len(b.Targets) > 0 {
		out.Targets = make(map[string]string, len(b.Targets))
		for k, tgt := range b.Targets {
			key, _ := k.MarshalText()
			out.Targets[string(key)] = model.RunsText(tgt.Runs)
		}
	}
	if len(b.Properties) > 0 {
		out.Properties = b.Properties
	}
	if len(b.Overlays) > 0 {
		if raw, err := json.Marshal(b.Overlays); err == nil {
			out.Overlays = raw
		}
	}
	if am := b.AnnoMap(); len(am) > 0 {
		out.Annotations = make(map[string]json.RawMessage, len(am))
		for k, v := range am {
			if raw, err := json.Marshal(v); err == nil {
				out.Annotations[k] = raw
			}
		}
	}
	return out
}
