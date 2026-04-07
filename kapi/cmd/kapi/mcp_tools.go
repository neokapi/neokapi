package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	pluginreg "github.com/neokapi/neokapi/core/plugin/registry"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
)

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
		Description: "Parse a file and extract translatable content blocks with source text and word counts",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExtractContentInput) (*mcp.CallToolResult, ExtractContentOutput, error) {
		return handleExtractContent(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_flow",
		Description: "Execute a processing flow (e.g. pseudo-translate, qa-check) on a file",
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
		return handleListTools()
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
}

type BlockEntry struct {
	ID         string `json:"id"`
	SourceText string `json:"source_text"`
	WordCount  int    `json:"word_count"`
}

type ExtractContentOutput struct {
	Blocks    []BlockEntry `json:"blocks"`
	Format    string       `json:"format"`
	WordCount int          `json:"word_count"`
}

type RunFlowInput struct {
	FlowName   string `json:"flow_name" jsonschema:"Name of the flow to run (e.g. pseudo-translate or qa-check)"`
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
			Name:        info.Name,
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

	out := DetectFormatOutput{Format: fmtName}
	if info := a.FormatReg.FormatInfo(fmtName); info != nil {
		out.Extensions = info.Extensions
		out.HasReader = info.HasReader
		out.HasWriter = info.HasWriter
	}
	return nil, out, nil
}

func handleExtractContent(ctx context.Context, a *cli.App, input ExtractContentInput) (*mcp.CallToolResult, ExtractContentOutput, error) {
	fmtName, reader, err := openReader(ctx, a, input.Path, input.Format, input.SourceLang)
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
				ID:         blk.ID,
				SourceText: blk.SourceText(),
				WordCount:  wc,
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
		return nil, RunFlowOutput{}, fmt.Errorf("path is required (or specify a project file)")
	}

	targetLang := input.TargetLang
	if targetLang == "" {
		if def := cli.LookupToolCommand(input.FlowName); def != nil && def.DefaultTargetLang != "" {
			targetLang = def.DefaultTargetLang
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
	proj, err := project.Load(input.Project)
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
		if def := cli.LookupToolCommand(input.FlowName); def != nil && def.DefaultTargetLang != "" {
			targetLang = def.DefaultTargetLang
		} else {
			return nil, RunFlowOutput{}, fmt.Errorf("target_lang is required for flow %q", input.FlowName)
		}
	}

	// Resolve flow: check project flows first, then built-in.
	flowName := input.FlowName
	if spec := proj.GetFlow(flowName); spec != nil {
		// Project flow — build tools from steps.
		config := map[string]any{
			"source_locale": sourceLang,
			"target_locale": targetLang,
		}
		var flowTools []tool.Tool
		for _, step := range spec.Steps {
			def := cli.LookupToolCommand(step.Tool)
			if def == nil || def.NewToolFromConfig == nil {
				return nil, RunFlowOutput{}, fmt.Errorf("tool %q not available", step.Tool)
			}
			// Merge step config with defaults.
			merged := config
			if len(step.Config) > 0 {
				merged = make(map[string]any)
				for k, v := range config {
					merged[k] = v
				}
				for k, v := range step.Config {
					merged[k] = v
				}
			}
			t, err := def.NewToolFromConfig(merged, targetLang)
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
				return nil, RunFlowOutput{}, fmt.Errorf("no input files — specify path or add content patterns")
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
			return nil, RunFlowOutput{}, fmt.Errorf("no input files — specify path or add content patterns")
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
	fmtName, reader, err := openReader(ctx, a, input.Path, input.Format, input.SourceLang)
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

func handleListTools() (*mcp.CallToolResult, ListToolsOutput, error) {
	var entries []ToolEntry
	for _, def := range cli.BuiltinToolCommands {
		entries = append(entries, ToolEntry{
			Name:        def.Use,
			Description: def.Short,
			Source:      "builtin",
		})
	}
	return nil, ListToolsOutput{Tools: entries, Total: len(entries)}, nil
}

func handlePseudoTranslate(ctx context.Context, a *cli.App, input PseudoTranslateInput) (*mcp.CallToolResult, RunFlowOutput, error) {
	targetLang := input.TargetLang
	if targetLang == "" {
		if def := cli.LookupToolCommand("pseudo-translate"); def != nil && def.DefaultTargetLang != "" {
			targetLang = def.DefaultTargetLang
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
func openReader(ctx context.Context, a *cli.App, path, formatOverride, sourceLang string) (string, format.DataFormatReader, error) {
	fmtName := formatOverride
	if fmtName == "" {
		ext := filepath.Ext(path)
		detected, err := a.FormatReg.DetectByExtension(ext)
		if err != nil {
			return "", nil, fmt.Errorf("unable to detect format: %w", err)
		}
		fmtName = detected
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
	ref := pluginreg.ParseFormatRef(fmtName)
	registryName := ref.RegistryName()

	var mergedConfig map[string]any
	if ref.IsPreset() {
		presetReg := a.PluginLoader.Presets()
		preset.RegisterBuiltins(presetReg)
		resolver := preset.NewConfigResolver(presetReg, a.PluginLoader.Schemas())

		var err error
		mergedConfig, err = resolver.ResolveFormatConfig(ref.Name, ref.Preset, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("resolve format config: %w", err)
		}
	}

	reader, err := a.FormatReg.NewReader(registryName)
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
	flowTools, err := buildFlowTools(flowName, sourceLang, targetLang)
	if err != nil {
		return "", err
	}
	return executeFlowWithTools(ctx, a, flowName, inputPath, sourceLang, targetLang, outputPath, flowTools, pctx)
}

func executeFlowWithTools(ctx context.Context, a *cli.App, flowName, inputPath, sourceLang, targetLang, outputPath string, flowTools []tool.Tool, pctx *project.ProjectContext) (string, error) {
	// Detect format — project-scoped when a project context is available.
	ext := filepath.Ext(inputPath)
	var fmtName string
	var err error
	if pctx != nil {
		fmtName = pctx.DetectFormat(a.FormatReg, inputPath)
		if fmtName == "" {
			return "", fmt.Errorf("unable to detect format for %q", inputPath)
		}
	} else {
		fmtName, err = a.FormatReg.DetectByExtension(ext)
		if err != nil {
			return "", fmt.Errorf("unable to detect format: %w", err)
		}
	}

	ref := pluginreg.ParseFormatRef(fmtName)
	registryName := ref.RegistryName()

	reader, err := a.FormatReg.NewReader(registryName)
	if err != nil {
		return "", fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}

	// Apply project format defaults to reader.
	if pctx != nil {
		if cfgErr := pctx.ConfigureReader(reader, fmtName); cfgErr != nil {
			return "", fmt.Errorf("apply project format config: %w", cfgErr)
		}
	}

	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}

	// Create writer early so we can wire skeleton store before reading.
	writer, err := a.FormatReg.NewWriter(registryName)
	if err != nil {
		return "", fmt.Errorf("no writer for format %q: %w", fmtName, err)
	}

	// Apply project encoding to writer.
	if pctx != nil {
		pctx.ConfigureWriter(writer)
	}

	// Wire skeleton store if both reader and writer support it.
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
			store, err := format.NewSkeletonStore()
			if err == nil {
				defer store.Close()
				emitter.SetSkeletonStore(store)
				consumer.SetSkeletonStore(store)
			}
		}
	}

	doc := &model.RawDocument{
		URI:          inputPath,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(inputContent)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		return "", fmt.Errorf("open document: %w", err)
	}
	defer reader.Close()

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return "", fmt.Errorf("read error: %w", result.Error)
		}
		parts = append(parts, result.Part)
	}

	fb := flow.NewFlow(flowName)
	for _, t := range flowTools {
		fb.AddTool(t)
	}
	f, err := fb.Build()
	if err != nil {
		return "", fmt.Errorf("build flow: %w", err)
	}

	executor := flow.NewExecutor()
	inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f)

	go func() {
		for _, p := range parts {
			inCh <- p
		}
		close(inCh)
	}()

	var outputParts []*model.Part
	for p := range outCh {
		outputParts = append(outputParts, p)
	}

	if err := wait(); err != nil {
		return "", fmt.Errorf("flow execution error: %w", err)
	}

	if outputPath == "" {
		base := inputPath[:len(inputPath)-len(ext)]
		outputPath = fmt.Sprintf("%s_%s%s", base, targetLang, ext)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return "", fmt.Errorf("set output: %w", err)
	}

	// Prefer passing the file path over loading content bytes when the writer
	// supports it. This avoids duplicating the file in memory for gRPC transfer.
	if sps, ok := writer.(format.SourcePathSetter); ok && filepath.IsAbs(inputPath) {
		sps.SetSourcePath(inputPath)
	} else if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent(inputContent)
	}

	writer.SetLocale(model.LocaleID(targetLang))

	ch := make(chan *model.Part, len(outputParts))
	for _, p := range outputParts {
		ch <- p
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		return "", fmt.Errorf("write output: %w", err)
	}
	writer.Close()

	return outputPath, nil
}

// buildFlowTools creates the tool chain for a named flow using the built-in
// flow registry and tool command definitions.
func buildFlowTools(flowName, sourceLang, targetLang string) ([]tool.Tool, error) {
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
		if n.Type == "tool" {
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
		def := cli.LookupToolCommand(node.name)
		if def == nil {
			return nil, fmt.Errorf("tool %q not found in registry", node.name)
		}
		if def.NewToolFromConfig == nil {
			return nil, fmt.Errorf("tool %q has no config factory (AI-powered tools require API keys)", node.name)
		}
		t, err := def.NewToolFromConfig(config, targetLang)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", node.name, err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}
