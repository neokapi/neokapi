package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
)

func init() {
	cli.RegisterMCPToolFactory(registerKapiTools)
}

// registerKapiTools registers all kapi MCP tools on the given server.
func registerKapiTools(server *mcp.Server, a *cli.App) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_formats",
		Description: "List all supported file formats with their extensions, MIME types, and read/write capabilities",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, ListFormatsOutput, error) {
		return handleListFormats(a)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "detect_format",
		Description: "Detect the file format from a file path based on its extension",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DetectFormatInput) (*mcp.CallToolResult, DetectFormatOutput, error) {
		return handleDetectFormat(a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "extract_content",
		Description: "Parse a file into translatable content blocks — each block's id, content_hash, source text (inline codes rendered as <x id=\"…\"/> placeholders), and word count. The read leg of the edit loop: edit a block's text keeping the placeholders, then send it back via apply_edits (or kapi apply).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExtractContentInput) (*mcp.CallToolResult, ExtractContentOutput, error) {
		return handleExtractContent(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_flow",
		Description: "Execute a processing flow (e.g. pseudo-translate, qa) on a file",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RunFlowInput) (*mcp.CallToolResult, RunFlowOutput, error) {
		return handleRunFlow(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_flows",
		Description: "List all available processing flows",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, ListFlowsOutput, error) {
		return handleListFlows()
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "word_count",
		Description: "Count translatable words in a file",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input WordCountInput) (*mcp.CallToolResult, WordCountOutput, error) {
		return handleWordCount(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tools",
		Description: "List all available processing tools",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, ListToolsOutput, error) {
		return handleListTools(a)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pseudo_translate",
		Description: "Pseudo-translate a file for localization QA testing",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PseudoTranslateInput) (*mcp.CallToolResult, RunFlowOutput, error) {
		return handlePseudoTranslate(ctx, a, input)
	})
}

// --- Input/Output types ---

type DetectFormatInput struct {
	Path string `json:"path" jsonschema:"File path to detect format from"`
}

type DetectFormatOutput struct {
	Format     string   `json:"format"`
	Extensions []string `json:"extensions,omitempty"`
	HasReader  bool     `json:"has_reader"`
	HasWriter  bool     `json:"has_writer"`
}

type ExtractContentInput struct {
	Path       string `json:"path" jsonschema:"File path to extract content from"`
	Format     string `json:"format,omitempty" jsonschema:"Override format detection"`
	SourceLang string `json:"source_lang,omitempty" jsonschema:"Source language (default: en)"`
	Project    string `json:"project,omitempty" jsonschema:"Path to .kapi project file for scoped format detection"`
}

type BlockEntry struct {
	ID string `json:"id"`
	// ContentHash is the canonical block identity (SHA-256 of the plain,
	// normalized source text) — the drift anchor to send back in an apply_edits
	// content entry.
	ContentHash string `json:"content_hash"`
	// SourceText renders inline codes as <x id="…"/> placeholders so an edit can
	// round-trip without dropping a link, span, or placeholder.
	SourceText string `json:"source_text"`
	WordCount  int    `json:"word_count"`
}

type ExtractContentOutput struct {
	Blocks    []BlockEntry `json:"blocks"`
	Format    string       `json:"format"`
	WordCount int          `json:"word_count"`
}

type RunFlowInput struct {
	FlowName   string `json:"flow_name" jsonschema:"Name of the flow to run (e.g. pseudo-translate or qa)"`
	Path       string `json:"path,omitempty" jsonschema:"Input file path (optional when project has content patterns)"`
	Project    string `json:"project,omitempty" jsonschema:"Path to a .kapi project file for project-scoped execution"`
	SourceLang string `json:"source_lang,omitempty" jsonschema:"Source language (default: en)"`
	TargetLang string `json:"target_lang,omitempty" jsonschema:"Target language"`
	OutputPath string `json:"output_path,omitempty" jsonschema:"Output file path (default: auto-generated)"`
}

type RunFlowOutput struct {
	FlowName   string `json:"flow_name"`
	InputPath  string `json:"input_path"`
	OutputPath string `json:"output_path"`
}

type WordCountInput struct {
	Path       string `json:"path" jsonschema:"File path to count words in"`
	Format     string `json:"format,omitempty" jsonschema:"Override format detection"`
	SourceLang string `json:"source_lang,omitempty" jsonschema:"Source language (default: en)"`
	Project    string `json:"project,omitempty" jsonschema:"Path to .kapi project file for scoped format detection"`
}

type WordCountOutput struct {
	WordCount  int    `json:"word_count"`
	BlockCount int    `json:"block_count"`
	Format     string `json:"format"`
}

type PseudoTranslateInput struct {
	Path       string `json:"path" jsonschema:"File path to pseudo-translate"`
	TargetLang string `json:"target_lang,omitempty" jsonschema:"Target language (default: qps)"`
	OutputPath string `json:"output_path,omitempty" jsonschema:"Output file path (default: auto-generated)"`
}

type FormatEntry struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
	MimeTypes   []string `json:"mime_types,omitempty"`
	HasReader   bool     `json:"has_reader"`
	HasWriter   bool     `json:"has_writer"`
	Source      string   `json:"source"`
}

type ListFormatsOutput struct {
	Formats []FormatEntry `json:"formats"`
	Total   int           `json:"total"`
}

type FlowEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ListFlowsOutput struct {
	Flows []FlowEntry `json:"flows"`
	Total int         `json:"total"`
}

type ToolEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

type ListToolsOutput struct {
	Tools []ToolEntry `json:"tools"`
	Total int         `json:"total"`
}

// --- Handlers ---

func handleListFormats(a *cli.App) (*mcp.CallToolResult, ListFormatsOutput, error) {
	infos := a.FormatReg.FormatInfos()
	entries := make([]FormatEntry, len(infos))
	for i, info := range infos {
		entries[i] = FormatEntry{
			Name:        string(info.Name),
			DisplayName: info.DisplayName,
			Extensions:  info.Extensions,
			MimeTypes:   info.MimeTypes,
			HasReader:   info.HasReader,
			HasWriter:   info.HasWriter,
			Source:      info.Source,
		}
	}
	return nil, ListFormatsOutput{Formats: entries, Total: len(entries)}, nil
}

func handleDetectFormat(a *cli.App, input DetectFormatInput) (*mcp.CallToolResult, DetectFormatOutput, error) {
	ext := filepath.Ext(input.Path)
	if ext == "" {
		return nil, DetectFormatOutput{}, fmt.Errorf("no file extension in path %q", input.Path)
	}

	fmtName, err := a.FormatReg.DetectByExtension(ext)
	if err != nil {
		return nil, DetectFormatOutput{}, fmt.Errorf("unable to detect format: %w", err)
	}

	out := DetectFormatOutput{Format: string(fmtName)}
	if info := a.FormatReg.FormatInfo(fmtName); info != nil {
		out.Extensions = info.Extensions
		out.HasReader = info.HasReader
		out.HasWriter = info.HasWriter
	}
	return nil, out, nil
}

func handleExtractContent(ctx context.Context, a *cli.App, input ExtractContentInput) (*mcp.CallToolResult, ExtractContentOutput, error) {
	fmtName, reader, err := openReader(ctx, a, input.Path, input.Format, input.SourceLang, input.Project)
	if err != nil {
		return nil, ExtractContentOutput{}, err
	}
	defer reader.Close()

	var blocks []BlockEntry
	var totalWords int
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return nil, ExtractContentOutput{}, fmt.Errorf("read error: %w", result.Error)
		}
		if result.Part.Type == model.PartBlock {
			blk, ok := result.Part.Resource.(*model.Block)
			if !ok {
				continue
			}
			if !blk.Translatable {
				continue
			}
			wc := blk.WordCount()
			blocks = append(blocks, BlockEntry{
				ID:          blk.ID,
				ContentHash: model.ComputeContentHash(blk.SourceText()),
				SourceText:  model.RunsPlaceholderText(blk.Source),
				WordCount:   wc,
			})
			totalWords += wc
		}
	}

	return nil, ExtractContentOutput{
		Blocks:    blocks,
		Format:    fmtName,
		WordCount: totalWords,
	}, nil
}

func handleRunFlow(ctx context.Context, a *cli.App, input RunFlowInput) (*mcp.CallToolResult, RunFlowOutput, error) {
	// When a project file is specified, use ProjectContext for defaults and content resolution.
	if input.Project != "" {
		return handleRunFlowWithProject(ctx, a, input)
	}

	if input.Path == "" {
		return nil, RunFlowOutput{}, errors.New("path is required (or specify a project file)")
	}

	targetLang := input.TargetLang
	if targetLang == "" {
		if info := a.ToolReg.ToolInfo(registry.ToolID(input.FlowName)); info != nil && info.DefaultLocale != "" {
			targetLang = string(info.DefaultLocale)
		} else {
			return nil, RunFlowOutput{}, fmt.Errorf("target_lang is required for flow %q", input.FlowName)
		}
	}

	sourceLang := input.SourceLang
	if sourceLang == "" {
		sourceLang = "en"
	}

	outputPath, err := executeFlow(ctx, a, input.FlowName, input.Path, sourceLang, targetLang, input.OutputPath, nil)
	if err != nil {
		return nil, RunFlowOutput{}, err
	}

	return nil, RunFlowOutput{
		FlowName:   input.FlowName,
		InputPath:  input.Path,
		OutputPath: outputPath,
	}, nil
}

func handleRunFlowWithProject(ctx context.Context, a *cli.App, input RunFlowInput) (*mcp.CallToolResult, RunFlowOutput, error) {
	proj, err := a.LoadProjectInteractive(ctx, input.Project, cli.LoadProjectInteractiveOptions{
		AssumeYes: a.AssumeYes,
	})
	if err != nil {
		return nil, RunFlowOutput{}, fmt.Errorf("load project: %w", err)
	}

	pctx := project.NewProjectContext(proj, input.Project)

	// Apply project defaults for languages.
	sourceLang := input.SourceLang
	if sourceLang == "" {
		sourceLang = string(pctx.SourceLocale)
	}
	if sourceLang == "" {
		sourceLang = "en"
	}

	targetLang := input.TargetLang
	if targetLang == "" && len(pctx.TargetLocales) > 0 {
		targetLang = string(pctx.TargetLocales[0])
	}
	if targetLang == "" {
		if info := a.ToolReg.ToolInfo(registry.ToolID(input.FlowName)); info != nil && info.DefaultLocale != "" {
			targetLang = string(info.DefaultLocale)
		} else {
			return nil, RunFlowOutput{}, fmt.Errorf("target_lang is required for flow %q", input.FlowName)
		}
	}

	// Resolve flow: check project flows first, then built-in.
	flowName := input.FlowName
	if spec := proj.Flow(flowName); spec != nil {
		// Project flow — build tools from steps.
		config := map[string]any{
			"source_locale": sourceLang,
			"target_locale": targetLang,
		}
		var flowTools []tool.Tool
		for _, step := range spec.Steps {
			// Merge step config with defaults.
			merged := config
			if len(step.Config) > 0 {
				merged = make(map[string]any)
				maps.Copy(merged, config)
				maps.Copy(merged, step.Config)
			}
			t, err := a.ToolReg.NewToolWithConfig(registry.ToolID(step.Tool), merged, targetLang)
			if err != nil {
				return nil, RunFlowOutput{}, fmt.Errorf("create tool %q: %w", step.Tool, err)
			}
			flowTools = append(flowTools, t)
		}
		// Use the first input file (from flag or content resolution).
		inputPath := input.Path
		if inputPath == "" {
			resolved, err := pctx.ResolveContent(a.FormatReg)
			if err != nil {
				return nil, RunFlowOutput{}, fmt.Errorf("resolve content: %w", err)
			}
			if len(resolved) == 0 {
				return nil, RunFlowOutput{}, errors.New("no input files — specify path or add content patterns")
			}
			inputPath = resolved[0].Path
		}
		outputPath, err := executeFlowWithTools(ctx, a, flowName, inputPath, sourceLang, targetLang, input.OutputPath, flowTools, pctx)
		if err != nil {
			return nil, RunFlowOutput{}, err
		}
		return nil, RunFlowOutput{FlowName: flowName, InputPath: inputPath, OutputPath: outputPath}, nil
	}

	// Built-in flow.
	inputPath := input.Path
	if inputPath == "" {
		resolved, err := pctx.ResolveContent(a.FormatReg)
		if err != nil {
			return nil, RunFlowOutput{}, fmt.Errorf("resolve content: %w", err)
		}
		if len(resolved) == 0 {
			return nil, RunFlowOutput{}, errors.New("no input files — specify path or add content patterns")
		}
		inputPath = resolved[0].Path
	}

	outputPath, err := executeFlow(ctx, a, flowName, inputPath, sourceLang, targetLang, input.OutputPath, pctx)
	if err != nil {
		return nil, RunFlowOutput{}, err
	}
	return nil, RunFlowOutput{FlowName: flowName, InputPath: inputPath, OutputPath: outputPath}, nil
}

func handleListFlows() (*mcp.CallToolResult, ListFlowsOutput, error) {
	var flows []FlowEntry
	for _, def := range flow.BuiltInFlows() {
		flows = append(flows, FlowEntry{
			Name:        def.ID,
			Description: def.Description,
		})
	}
	return nil, ListFlowsOutput{Flows: flows, Total: len(flows)}, nil
}

func handleWordCount(ctx context.Context, a *cli.App, input WordCountInput) (*mcp.CallToolResult, WordCountOutput, error) {
	fmtName, reader, err := openReader(ctx, a, input.Path, input.Format, input.SourceLang, input.Project)
	if err != nil {
		return nil, WordCountOutput{}, err
	}
	defer reader.Close()

	var wordCount, blockCount int
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return nil, WordCountOutput{}, fmt.Errorf("read error: %w", result.Error)
		}
		if result.Part.Type == model.PartBlock {
			blk, ok := result.Part.Resource.(*model.Block)
			if !ok {
				continue
			}
			if !blk.Translatable {
				continue
			}
			wordCount += blk.WordCount()
			blockCount++
		}
	}

	return nil, WordCountOutput{
		WordCount:  wordCount,
		BlockCount: blockCount,
		Format:     fmtName,
	}, nil
}

func handleListTools(a *cli.App) (*mcp.CallToolResult, ListToolsOutput, error) {
	var entries []ToolEntry
	if a.ToolReg != nil {
		for _, entry := range a.ToolReg.CLITools() {
			desc := entry.Info.Description
			if desc == "" {
				desc = entry.Info.DisplayName
			}
			entries = append(entries, ToolEntry{
				Name:        string(entry.Info.Name),
				Description: desc,
				Source:      entry.Info.Source,
			})
		}
	}
	return nil, ListToolsOutput{Tools: entries, Total: len(entries)}, nil
}

func handlePseudoTranslate(ctx context.Context, a *cli.App, input PseudoTranslateInput) (*mcp.CallToolResult, RunFlowOutput, error) {
	targetLang := input.TargetLang
	if targetLang == "" {
		if info := a.ToolReg.ToolInfo(registry.ToolID("pseudo-translate")); info != nil && info.DefaultLocale != "" {
			targetLang = string(info.DefaultLocale)
		} else {
			targetLang = "qps"
		}
	}

	outputPath, err := executeFlow(ctx, a, "pseudo-translate", input.Path, "en", targetLang, input.OutputPath, nil)
	if err != nil {
		return nil, RunFlowOutput{}, err
	}

	return nil, RunFlowOutput{
		FlowName:   "pseudo-translate",
		InputPath:  input.Path,
		OutputPath: outputPath,
	}, nil
}

// --- Shared helpers ---

// openReader detects the format, creates a reader, and opens the document.
// Caller must call reader.Close().
func openReader(ctx context.Context, a *cli.App, path, formatOverride, sourceLang, projectPath string) (string, format.DataFormatReader, error) {
	fmtName := formatOverride
	if fmtName == "" {
		// Use project-scoped detection when a project file is specified.
		if projectPath != "" {
			proj, err := a.LoadProjectInteractive(ctx, projectPath, cli.LoadProjectInteractiveOptions{
				AssumeYes: a.AssumeYes,
			})
			if err == nil {
				pctx := project.NewProjectContext(proj, projectPath)
				fmtName = pctx.DetectFormat(a.FormatReg, path)
			}
		}
		if fmtName == "" {
			ext := filepath.Ext(path)
			detected, err := a.FormatReg.DetectByExtension(ext)
			if err != nil {
				return "", nil, fmt.Errorf("unable to detect format: %w", err)
			}
			fmtName = string(detected)
		}
	}

	reader, err := createReader(a, fmtName)
	if err != nil {
		return "", nil, err
	}

	srcLang := sourceLang
	if srcLang == "" {
		srcLang = "en"
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read input: %w", err)
	}

	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleID(srcLang),
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		return "", nil, fmt.Errorf("open document: %w", err)
	}

	return fmtName, reader, nil
}

// createReader creates a format reader, handling preset syntax.
func createReader(a *cli.App, fmtName string) (format.DataFormatReader, error) {
	ref := preset.ParseFormatRef(fmtName)
	registryName := ref.RegistryName()

	var mergedConfig map[string]any
	if ref.IsPreset() {
		presetReg := preset.NewPresetRegistry()
		preset.RegisterBuiltins(presetReg)
		resolver := preset.NewConfigResolver(presetReg, a.SchemaReg)

		var err error
		mergedConfig, err = resolver.ResolveFormatConfig(ref.Name, ref.Preset, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("resolve format config: %w", err)
		}
	}

	reader, err := a.FormatReg.NewReader(registry.FormatID(registryName))
	if err != nil {
		return nil, fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}

	if len(mergedConfig) > 0 {
		if cfg := reader.Config(); cfg != nil {
			if err := cfg.ApplyMap(mergedConfig); err != nil {
				return nil, fmt.Errorf("apply format config: %w", err)
			}
		}
	}

	return reader, nil
}

// executeFlow runs a named flow on a file and writes the result.
func executeFlow(ctx context.Context, a *cli.App, flowName, inputPath, sourceLang, targetLang, outputPath string, pctx *project.ProjectContext) (string, error) {
	flowTools, err := buildFlowTools(a, flowName, sourceLang, targetLang)
	if err != nil {
		return "", err
	}
	return executeFlowWithTools(ctx, a, flowName, inputPath, sourceLang, targetLang, outputPath, flowTools, pctx)
}

func executeFlowWithTools(ctx context.Context, a *cli.App, flowName, inputPath, sourceLang, targetLang, outputPath string, flowTools []tool.Tool, pctx *project.ProjectContext) (string, error) {
	// Compute output path if not specified.
	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		base := inputPath[:len(inputPath)-len(ext)]
		outputPath = fmt.Sprintf("%s_%s%s", base, targetLang, ext)
	}

	encoding := "UTF-8"
	if pctx != nil && pctx.Encoding != "" {
		encoding = pctx.Encoding
	}

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    a.FormatReg,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     encoding,
		ConfigureReader: func(reader format.DataFormatReader, fmtName registry.FormatID) error {
			if pctx != nil {
				return pctx.ConfigureReader(reader, string(fmtName))
			}
			return nil
		},
		ConfigureWriter: func(writer format.DataFormatWriter) {
			if pctx != nil {
				pctx.ConfigureWriter(writer)
			}
		},
	})

	if err := runner.RunFile(ctx, flowName, flowTools, inputPath, outputPath, targetLang); err != nil {
		return "", err
	}

	return outputPath, nil
}

// buildFlowTools creates the tool chain for a named flow using the built-in
// flow registry and tool command definitions.
func buildFlowTools(a *cli.App, flowName, sourceLang, targetLang string) ([]tool.Tool, error) {
	// Look up the flow definition.
	var flowDef *flow.FlowDefinition
	for _, def := range flow.BuiltInFlows() {
		if def.ID == flowName {
			d := def
			flowDef = &d
			break
		}
	}
	if flowDef == nil {
		return nil, fmt.Errorf("unknown flow: %q", flowName)
	}

	// Extract tool nodes sorted by position.
	type tn struct {
		name string
		x    float64
	}
	var toolNodes []tn
	for _, n := range flowDef.Nodes {
		if n.Type == flow.NodeTool {
			toolNodes = append(toolNodes, tn{name: n.Name, x: n.Position.X})
		}
	}
	// Sort by X position.
	for i := 1; i < len(toolNodes); i++ {
		for j := i; j > 0 && toolNodes[j].x < toolNodes[j-1].x; j-- {
			toolNodes[j], toolNodes[j-1] = toolNodes[j-1], toolNodes[j]
		}
	}

	config := map[string]any{
		"source_locale": sourceLang,
		"target_locale": targetLang,
	}

	var tools []tool.Tool
	for _, node := range toolNodes {
		t, err := a.ToolReg.NewToolWithConfig(registry.ToolID(node.name), config, targetLang)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", node.name, err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}
