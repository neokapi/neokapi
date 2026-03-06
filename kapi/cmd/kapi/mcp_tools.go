package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/loader"
	pluginreg "github.com/gokapi/gokapi/core/plugin/registry"
	"github.com/gokapi/gokapi/core/preset"
	"github.com/gokapi/gokapi/core/tool"
	libtools "github.com/gokapi/gokapi/core/tools"
	"github.com/gokapi/gokapi/cli"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	Path       string `json:"path" jsonschema:"Input file path"`
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

	fmtName, err := a.FormatReg.Detector().DetectByExtension(ext)
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
			blk := result.Part.Resource.(*model.Block)
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
	targetLang := input.TargetLang
	if targetLang == "" {
		if input.FlowName == "pseudo-translate" {
			targetLang = "qps"
		} else {
			return nil, RunFlowOutput{}, fmt.Errorf("target_lang is required for flow %q", input.FlowName)
		}
	}

	sourceLang := input.SourceLang
	if sourceLang == "" {
		sourceLang = "en"
	}

	outputPath, err := executeFlow(ctx, a, input.FlowName, input.Path, sourceLang, targetLang, input.OutputPath)
	if err != nil {
		return nil, RunFlowOutput{}, err
	}

	return nil, RunFlowOutput{
		FlowName:   input.FlowName,
		InputPath:  input.Path,
		OutputPath: outputPath,
	}, nil
}

func handleListFlows() (*mcp.CallToolResult, ListFlowsOutput, error) {
	flows := []FlowEntry{
		{Name: "ai-translate", Description: "Translate content using AI/LLM"},
		{Name: "ai-translate-qa", Description: "Translate + quality check using AI/LLM"},
		{Name: "pseudo-translate", Description: "Generate pseudo-translations for testing"},
		{Name: "qa-check", Description: "Run rule-based quality checks on translations"},
		{Name: "tm-leverage", Description: "Pre-fill translations from translation memory"},
		{Name: "segmentation", Description: "Split source text into sentence segments"},
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
			blk := result.Part.Resource.(*model.Block)
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

	otherTools := []struct {
		name string
		desc string
	}{
		{"ai-translate", "Translate content using AI/LLM"},
		{"ai-qa", "Check translation quality using AI/LLM"},
		{"ai-terminology", "Extract terminology using AI/LLM"},
		{"ai-review", "Review translations using AI/LLM"},
		{"pseudo-translate", "Generate pseudo-translations for testing"},
	}
	for _, t := range otherTools {
		entries = append(entries, ToolEntry{
			Name:        t.name,
			Description: t.desc,
			Source:      "builtin",
		})
	}
	return nil, ListToolsOutput{Tools: entries, Total: len(entries)}, nil
}

func handlePseudoTranslate(ctx context.Context, a *cli.App, input PseudoTranslateInput) (*mcp.CallToolResult, RunFlowOutput, error) {
	targetLang := input.TargetLang
	if targetLang == "" {
		targetLang = "qps"
	}

	outputPath, err := executeFlow(ctx, a, "pseudo-translate", input.Path, "en", targetLang, input.OutputPath)
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
		detected, err := a.FormatReg.Detector().DetectByExtension(ext)
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
	resolvedName := ref.Name

	var mergedConfig map[string]any
	if ref.IsPreset() {
		presetReg := a.PluginLoader.Presets()
		preset.RegisterBuiltins(presetReg)
		resolver := preset.NewConfigResolver(presetReg, a.PluginLoader.Schemas())

		var err error
		mergedConfig, err = resolver.ResolveFormatConfig(resolvedName, ref.Preset, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("resolve format config: %w", err)
		}
	}

	reader, err := a.FormatReg.NewReader(resolvedName)
	if err != nil {
		reader, err = a.FormatReg.NewReader(fmtName)
		if err != nil {
			return nil, fmt.Errorf("no reader for format %q: %w", fmtName, err)
		}
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
func executeFlow(ctx context.Context, a *cli.App, flowName, inputPath, sourceLang, targetLang, outputPath string) (string, error) {
	ext := filepath.Ext(inputPath)
	fmtName, err := a.FormatReg.Detector().DetectByExtension(ext)
	if err != nil {
		return "", fmt.Errorf("unable to detect format: %w", err)
	}

	ref := pluginreg.ParseFormatRef(fmtName)
	resolvedFmtName := ref.Name

	reader, err := a.FormatReg.NewReader(resolvedFmtName)
	if err != nil {
		reader, err = a.FormatReg.NewReader(fmtName)
		if err != nil {
			return "", fmt.Errorf("no reader for format %q: %w", fmtName, err)
		}
	}

	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
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

	flowTools, err := buildFlowTools(flowName, sourceLang, targetLang)
	if err != nil {
		return "", err
	}

	fb := flow.NewFlow(flowName)
	for _, t := range flowTools {
		fb.AddTool(t)
	}
	f := fb.Build()

	executor := flow.NewFlowExecutor()
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

	writer, err := a.FormatReg.NewWriter(resolvedFmtName)
	if err != nil {
		writer, err = a.FormatReg.NewWriter(fmtName)
		if err != nil {
			return "", fmt.Errorf("no writer for format %q: %w", fmtName, err)
		}
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return "", fmt.Errorf("set output: %w", err)
	}

	// Prefer passing the file path over loading content bytes when the writer
	// supports it. This avoids duplicating the file in memory for gRPC transfer.
	if sps, ok := writer.(loader.SourcePathSetter); ok && filepath.IsAbs(inputPath) {
		sps.SetSourcePath(inputPath)
	} else if ocs, ok := writer.(loader.OriginalContentSetter); ok {
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

// buildFlowTools creates the tool chain for a named flow.
func buildFlowTools(flowName, sourceLang, targetLang string) ([]tool.Tool, error) {
	switch flowName {
	case "pseudo-translate":
		return []tool.Tool{
			libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
				TargetLocale: model.LocaleID(targetLang),
			}),
		}, nil
	case "qa-check":
		return []tool.Tool{
			libtools.NewQACheckTool(libtools.NewQACheckConfig(model.LocaleID(targetLang))),
		}, nil
	case "segmentation":
		return []tool.Tool{
			libtools.NewSegmentationTool(&libtools.SegmentationConfig{
				TargetLocale: model.LocaleID(targetLang),
			}),
		}, nil
	case "tm-leverage":
		return []tool.Tool{
			libtools.NewTMLeverageTool(&libtools.TMLeverageConfig{
				SourceLocale:   model.LocaleID(sourceLang),
				TargetLocale:   model.LocaleID(targetLang),
				FuzzyThreshold: 70,
				Provider:       libtools.NullTMProvider{},
			}),
		}, nil
	default:
		return nil, fmt.Errorf("unknown flow: %q (AI-powered flows require API keys and are not available via MCP)", flowName)
	}
}
