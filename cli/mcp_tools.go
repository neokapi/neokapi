package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/spf13/cobra"
)

// init exposes every CLI-visible framework tool over MCP as a schema-derived
// text tool. Unlike the hand-authored MCP tools (which operate on file paths),
// these run a single tool over a snippet of text the caller supplies, with the
// tool's own parameters projected straight from its schema. The set is scoped
// by mode (see registerFrameworkMCPTools): in a kapi project, only the tools the
// project allows; ad-hoc, the full set.
func init() {
	RegisterMCPToolFactory(registerFrameworkMCPTools)
}

// resolveMCPProject resolves the active project (git-style upward walk, honoring
// KAPI_NO_PROJECT and an explicit -p) and, when one is found, loads it into
// a.projectContext so the MCP factories can scope themselves to it. Failure to
// resolve or load is non-fatal — the server simply runs in ad-hoc mode.
func (a *App) resolveMCPProject(cmd *cobra.Command) {
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return
	}
	proj, err := project.Load(path)
	if err != nil {
		return
	}
	a.projectContext = project.NewProjectContext(proj, path)
}

// registerFrameworkMCPTools registers one MCP tool per CLI-visible framework
// tool. In project mode (a.projectContext set) the set is filtered to the
// project's allowed sources and the project's first target language becomes the
// default; in ad-hoc mode the full CLI tool set is exposed with no language
// default. This mirrors the desktop's ListTools vs ListProjectTools split.
func registerFrameworkMCPTools(server *mcp.Server, a *App) {
	if a.ToolReg == nil {
		return
	}
	entries, defaultTargetLang := scopeFrameworkTools(a.ToolReg.CLITools(), a.projectContext)

	t := a.T()
	for _, entry := range entries {
		name := string(entry.Info.Name)
		scope := "tools." + name
		desc := t.T(i18n.Scope(scope+".description"), entry.Info.Description)
		if desc == "" {
			desc = t.T(i18n.Scope(scope+".displayName"), entry.Info.DisplayName)
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
	if len(b.Annotations) > 0 {
		out.Annotations = make(map[string]json.RawMessage, len(b.Annotations))
		for k, v := range b.Annotations {
			if raw, err := json.Marshal(v); err == nil {
				out.Annotations[k] = raw
			}
		}
	}
	return out
}
