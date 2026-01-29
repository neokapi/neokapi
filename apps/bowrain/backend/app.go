package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/asgeirf/gokapi/ai/provider"
	"github.com/asgeirf/gokapi/ai/tools"
	"github.com/asgeirf/gokapi/core/flow"
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/registry"
	"github.com/asgeirf/gokapi/core/tool"
	"github.com/asgeirf/gokapi/formats"
)

// App is the Bowrain UI backend. It exposes methods that can be
// bound to a Wails frontend or called from tests.
type App struct {
	ctx       context.Context
	formatReg *registry.FormatRegistry
}

// NewApp creates a new Bowrain backend with all formats registered.
func NewApp() *App {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return &App{formatReg: reg}
}

// Startup is called by Wails when the application starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// FormatInfo describes a registered data format.
type FormatInfo struct {
	Name      string `json:"name"`
	HasReader bool   `json:"has_reader"`
	HasWriter bool   `json:"has_writer"`
}

// ToolInfo describes an available tool.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// FlowInfo describes an available flow.
type FlowInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ConvertRequest holds parameters for format conversion.
type ConvertRequest struct {
	InputPath    string `json:"input_path"`
	OutputPath   string `json:"output_path"`
	InputFormat  string `json:"input_format"`
	OutputFormat string `json:"output_format"`
	SourceLang   string `json:"source_lang"`
	TargetLang   string `json:"target_lang"`
	Encoding     string `json:"encoding"`
}

// TranslateRequest holds parameters for AI translation.
type TranslateRequest struct {
	InputPath  string `json:"input_path"`
	OutputPath string `json:"output_path"`
	Format     string `json:"format"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
	Provider   string `json:"provider"`
	APIKey     string `json:"api_key"`
	Model      string `json:"model"`
	Encoding   string `json:"encoding"`
}

// FlowRequest holds parameters for flow execution.
type FlowRequest struct {
	FlowName   string `json:"flow_name"`
	InputPath  string `json:"input_path"`
	OutputPath string `json:"output_path"`
	Format     string `json:"format"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
	Provider   string `json:"provider"`
	APIKey     string `json:"api_key"`
	Model      string `json:"model"`
	Encoding   string `json:"encoding"`
}

// ConvertResult holds the result of a conversion.
type ConvertResult struct {
	OutputPath string `json:"output_path"`
	PartCount  int    `json:"part_count"`
}

// TranslateResult holds the result of a translation.
type TranslateResult struct {
	OutputPath string `json:"output_path"`
	BlockCount int    `json:"block_count"`
}

// ListFormats returns all registered formats.
func (a *App) ListFormats() []FormatInfo {
	readers := a.formatReg.ReaderNames()
	writers := a.formatReg.WriterNames()

	nameSet := make(map[string]bool)
	for _, n := range readers {
		nameSet[n] = true
	}
	for _, n := range writers {
		nameSet[n] = true
	}

	var result []FormatInfo
	for name := range nameSet {
		result = append(result, FormatInfo{
			Name:      name,
			HasReader: a.formatReg.HasReader(name),
			HasWriter: a.formatReg.HasWriter(name),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// ListTools returns all available tools.
func (a *App) ListTools() []ToolInfo {
	return []ToolInfo{
		{Name: "ai-translate", Description: "Translate content using AI/LLM"},
		{Name: "ai-qa-check", Description: "Quality check translations using AI"},
		{Name: "ai-terminology", Description: "Extract terminology using AI"},
		{Name: "ai-review", Description: "Review translations using AI"},
		{Name: "pseudo-translate", Description: "Generate pseudo-translations for testing"},
		{Name: "word-count", Description: "Count words in source and target text"},
		{Name: "search-replace", Description: "Search and replace text in blocks"},
	}
}

// ListFlows returns all available flows.
func (a *App) ListFlows() []FlowInfo {
	return []FlowInfo{
		{Name: "ai-translate", Description: "Translate content using AI/LLM"},
		{Name: "ai-translate-qa", Description: "Translate + quality check using AI/LLM"},
		{Name: "pseudo-translate", Description: "Generate pseudo-translations for testing"},
	}
}

// DetectFormat detects the format of a file by its extension.
func (a *App) DetectFormat(filePath string) (string, error) {
	ext := filepath.Ext(filePath)
	return a.formatReg.Detector().DetectByExtension(ext)
}

// Convert converts a file between formats.
func (a *App) Convert(req ConvertRequest) (*ConvertResult, error) {
	ctx := context.Background()

	if req.InputPath == "" {
		return nil, fmt.Errorf("input path is required")
	}
	if req.OutputPath == "" {
		return nil, fmt.Errorf("output path is required")
	}

	// Detect formats if not specified
	inputFormat := req.InputFormat
	if inputFormat == "" {
		detected, err := a.DetectFormat(req.InputPath)
		if err != nil {
			return nil, fmt.Errorf("unable to detect input format: %w", err)
		}
		inputFormat = detected
	}
	outputFormat := req.OutputFormat
	if outputFormat == "" {
		detected, err := a.DetectFormat(req.OutputPath)
		if err != nil {
			return nil, fmt.Errorf("unable to detect output format: %w", err)
		}
		outputFormat = detected
	}

	// Read
	reader, err := a.formatReg.NewReader(inputFormat)
	if err != nil {
		return nil, fmt.Errorf("no reader for %q: %w", inputFormat, err)
	}

	f, err := os.Open(req.InputPath)
	if err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}

	enc := req.Encoding
	if enc == "" {
		enc = "UTF-8"
	}
	srcLang := req.SourceLang
	if srcLang == "" {
		srcLang = "en"
	}

	doc := &model.RawDocument{
		URI:          req.InputPath,
		SourceLocale: model.LocaleID(srcLang),
		Encoding:     enc,
		Reader:       f,
	}

	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("open document: %w", err)
	}
	defer reader.Close()

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return nil, fmt.Errorf("read: %w", result.Error)
		}
		parts = append(parts, result.Part)
	}

	// Write
	writer, err := a.formatReg.NewWriter(outputFormat)
	if err != nil {
		return nil, fmt.Errorf("no writer for %q: %w", outputFormat, err)
	}

	if err := writer.SetOutput(req.OutputPath); err != nil {
		return nil, fmt.Errorf("set output: %w", err)
	}

	locale := model.LocaleID(srcLang)
	if req.TargetLang != "" {
		locale = model.LocaleID(req.TargetLang)
	}
	writer.SetLocale(locale)

	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	writer.Close()

	return &ConvertResult{
		OutputPath: req.OutputPath,
		PartCount:  len(parts),
	}, nil
}

// Translate translates a file using AI.
func (a *App) Translate(req TranslateRequest) (*TranslateResult, error) {
	ctx := context.Background()

	if req.InputPath == "" {
		return nil, fmt.Errorf("input path is required")
	}
	if req.TargetLang == "" {
		return nil, fmt.Errorf("target language is required")
	}

	// Detect format
	fmtName := req.Format
	if fmtName == "" {
		detected, err := a.DetectFormat(req.InputPath)
		if err != nil {
			return nil, fmt.Errorf("unable to detect format: %w", err)
		}
		fmtName = detected
	}

	// Default output path
	outputPath := req.OutputPath
	if outputPath == "" {
		ext := filepath.Ext(req.InputPath)
		base := req.InputPath[:len(req.InputPath)-len(ext)]
		outputPath = fmt.Sprintf("%s_%s%s", base, req.TargetLang, ext)
	}

	enc := req.Encoding
	if enc == "" {
		enc = "UTF-8"
	}
	srcLang := req.SourceLang
	if srcLang == "" {
		srcLang = "en"
	}

	// Read input
	reader, err := a.formatReg.NewReader(fmtName)
	if err != nil {
		return nil, fmt.Errorf("no reader for %q: %w", fmtName, err)
	}

	f, err := os.Open(req.InputPath)
	if err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}

	doc := &model.RawDocument{
		URI:          req.InputPath,
		SourceLocale: model.LocaleID(srcLang),
		Encoding:     enc,
		Reader:       f,
	}

	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("open document: %w", err)
	}
	defer reader.Close()

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return nil, fmt.Errorf("read: %w", result.Error)
		}
		parts = append(parts, result.Part)
	}

	// Translate
	p := createProvider(req.Provider, req.APIKey, req.Model)
	translateTool := tools.NewAITranslateTool(p, tools.AITranslateConfig{
		SourceLocale: model.LocaleID(srcLang),
		TargetLocale: model.LocaleID(req.TargetLang),
	})

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, pt := range parts {
		in <- pt
	}
	close(in)

	if err := translateTool.Process(ctx, in, out); err != nil {
		return nil, fmt.Errorf("translation: %w", err)
	}
	close(out)

	var translated []*model.Part
	for pt := range out {
		translated = append(translated, pt)
	}

	// Write output
	writer, err := a.formatReg.NewWriter(fmtName)
	if err != nil {
		return nil, fmt.Errorf("no writer for %q: %w", fmtName, err)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return nil, fmt.Errorf("set output: %w", err)
	}
	writer.SetLocale(model.LocaleID(req.TargetLang))

	ch := make(chan *model.Part, len(translated))
	for _, pt := range translated {
		ch <- pt
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	writer.Close()

	blockCount := 0
	for _, pt := range translated {
		if pt.Type == model.PartBlock {
			blockCount++
		}
	}

	return &TranslateResult{
		OutputPath: outputPath,
		BlockCount: blockCount,
	}, nil
}

// ExecuteFlow runs a named flow on an input file.
func (a *App) ExecuteFlow(req FlowRequest) (*TranslateResult, error) {
	ctx := context.Background()

	if req.FlowName == "" {
		return nil, fmt.Errorf("flow name is required")
	}
	if req.InputPath == "" {
		return nil, fmt.Errorf("input path is required")
	}
	if req.TargetLang == "" {
		return nil, fmt.Errorf("target language is required")
	}

	enc := req.Encoding
	if enc == "" {
		enc = "UTF-8"
	}
	srcLang := req.SourceLang
	if srcLang == "" {
		srcLang = "en"
	}

	// Validate flow name early
	flowTools, err := buildFlowTools(req.FlowName, req.Provider, req.APIKey, req.Model, srcLang, req.TargetLang)
	if err != nil {
		return nil, err
	}

	// Detect format
	fmtName := req.Format
	if fmtName == "" {
		detected, err := a.DetectFormat(req.InputPath)
		if err != nil {
			return nil, fmt.Errorf("unable to detect format: %w", err)
		}
		fmtName = detected
	}

	// Default output path
	outputPath := req.OutputPath
	if outputPath == "" {
		ext := filepath.Ext(req.InputPath)
		base := req.InputPath[:len(req.InputPath)-len(ext)]
		outputPath = fmt.Sprintf("%s_%s%s", base, req.TargetLang, ext)
	}

	// Read input
	reader, err := a.formatReg.NewReader(fmtName)
	if err != nil {
		return nil, fmt.Errorf("no reader for %q: %w", fmtName, err)
	}

	f, err := os.Open(req.InputPath)
	if err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}

	doc := &model.RawDocument{
		URI:          req.InputPath,
		SourceLocale: model.LocaleID(srcLang),
		Encoding:     enc,
		Reader:       f,
	}

	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("open document: %w", err)
	}
	defer reader.Close()

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return nil, fmt.Errorf("read: %w", result.Error)
		}
		parts = append(parts, result.Part)
	}

	// Build flow
	fb := flow.NewFlow(req.FlowName)
	for _, t := range flowTools {
		fb.AddTool(t)
	}
	f2 := fb.Build()

	// Execute
	executor := flow.NewFlowExecutor()
	inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f2)

	go func() {
		for _, pt := range parts {
			inCh <- pt
		}
		close(inCh)
	}()

	var outputParts []*model.Part
	for pt := range outCh {
		outputParts = append(outputParts, pt)
	}

	if err := wait(); err != nil {
		return nil, fmt.Errorf("flow execution: %w", err)
	}

	// Write output
	writer, err := a.formatReg.NewWriter(fmtName)
	if err != nil {
		return nil, fmt.Errorf("no writer for %q: %w", fmtName, err)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return nil, fmt.Errorf("set output: %w", err)
	}
	writer.SetLocale(model.LocaleID(req.TargetLang))

	ch := make(chan *model.Part, len(outputParts))
	for _, pt := range outputParts {
		ch <- pt
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	writer.Close()

	blockCount := 0
	for _, pt := range outputParts {
		if pt.Type == model.PartBlock {
			blockCount++
		}
	}

	return &TranslateResult{
		OutputPath: outputPath,
		BlockCount: blockCount,
	}, nil
}

func createProvider(name, apiKey, modelName string) provider.LLMProvider {
	cfg := provider.Config{
		APIKey: apiKey,
		Model:  modelName,
	}
	switch name {
	case "anthropic":
		return provider.NewAnthropicProvider(cfg)
	case "openai":
		return provider.NewOpenAIProvider(cfg)
	case "ollama":
		return provider.NewOllamaProvider(cfg)
	default:
		return provider.NewMockProvider()
	}
}

func buildFlowTools(flowName, providerName, apiKey, modelName, srcLang, tgtLang string) ([]tool.Tool, error) {
	switch flowName {
	case "ai-translate":
		p := createProvider(providerName, apiKey, modelName)
		return []tool.Tool{
			tools.NewAITranslateTool(p, tools.AITranslateConfig{
				SourceLocale: model.LocaleID(srcLang),
				TargetLocale: model.LocaleID(tgtLang),
			}),
		}, nil
	case "ai-translate-qa":
		p := createProvider(providerName, apiKey, modelName)
		return []tool.Tool{
			tools.NewAITranslateTool(p, tools.AITranslateConfig{
				SourceLocale: model.LocaleID(srcLang),
				TargetLocale: model.LocaleID(tgtLang),
			}),
			tools.NewAIQACheckTool(p, tools.AIQAConfig{
				SourceLocale: model.LocaleID(srcLang),
				TargetLocale: model.LocaleID(tgtLang),
			}),
		}, nil
	case "pseudo-translate":
		t := &tool.BaseTool{
			ToolName:        "pseudo-translate",
			ToolDescription: "Generates pseudo-translations for testing",
		}
		t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
			block, ok := part.Resource.(*model.Block)
			if !ok || !block.Translatable {
				return part, nil
			}
			pseudo := "[" + block.SourceText() + "]"
			block.SetTargetText(model.LocaleID(tgtLang), pseudo)
			return part, nil
		}
		return []tool.Tool{t}, nil
	default:
		return nil, fmt.Errorf("unknown flow: %q", flowName)
	}
}
