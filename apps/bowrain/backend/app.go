package backend

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/gokapi/gokapi/ai/provider"
	"github.com/gokapi/gokapi/ai/tools"
	"github.com/gokapi/gokapi/core/config"
	"github.com/gokapi/gokapi/core/locale"
	"github.com/gokapi/gokapi/core/version"
	"github.com/gokapi/gokapi/core/credentials"
	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/gokapi/gokapi/formats"
	libtools "github.com/gokapi/gokapi/lib/tools"
	"github.com/gokapi/gokapi/plugin/loader"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the Bowrain UI backend. It exposes methods that can be
// bound to a Wails frontend or called from tests.
type App struct {
	app          *application.App
	formatReg    *registry.FormatRegistry
	toolReg      *registry.ToolRegistry
	projects     *projectStore
	pluginMu     sync.Mutex
	pluginLoader *loader.PluginLoader
	credentials  *credentials.Store

	// pluginSearchRegistry overrides the registry URL for testing.
	pluginSearchRegistry string

	initialProjectMu   sync.Mutex
	initialProjectPath string
}

// NewApp creates a new Bowrain backend with all formats and plugins registered.
// This blocks until plugin loading completes (which may start a JVM subprocess).
// For GUI use, prefer NewAppWithoutPlugins followed by LoadPlugins in a goroutine.
func NewApp() *App {
	a := NewAppWithoutPlugins()
	a.LoadPlugins()
	return a
}

// NewAppWithoutPlugins creates a Bowrain backend with built-in formats and tools
// registered but without loading plugins. Call LoadPlugins separately (possibly
// in a background goroutine) to discover and register plugins.
func NewAppWithoutPlugins() *App {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)

	return &App{
		formatReg:   reg,
		toolReg:     toolReg,
		projects:    newProjectStore(),
		credentials: credentials.NewStore(credentials.DefaultPath()),
	}
}

// LoadPlugins discovers and loads plugins from the configured plugin directory.
// This may start JVM subprocesses for Java bridge plugins and can take several
// seconds. Safe to call from a goroutine.
func (a *App) LoadPlugins() {
	pluginDir := os.Getenv("KAPI_PLUGIN_DIR")
	if pluginDir == "" {
		pluginDir = config.NewAppConfig().PluginDirectory()
	}

	pl := loader.NewPluginLoader(pluginDir, nil)
	if err := pl.LoadAll(a.formatReg, nil); err != nil {
		log.Printf("bowrain: failed to load plugins: %v", err)
	}

	a.pluginMu.Lock()
	a.pluginLoader = pl
	a.pluginMu.Unlock()
}

// SetApplication stores the Wails v3 application reference for dialog and event access.
func (a *App) SetApplication(app *application.App) {
	a.app = app
}

// SetInitialProjectPath stores a .kaz project path to be opened on startup.
// Called from main before app.Run().
func (a *App) SetInitialProjectPath(path string) {
	a.initialProjectMu.Lock()
	defer a.initialProjectMu.Unlock()
	a.initialProjectPath = path
}

// GetInitialProject returns and clears the initial project path.
// The consume-once pattern prevents double opens in React StrictMode dev mode.
func (a *App) GetInitialProject() string {
	a.initialProjectMu.Lock()
	defer a.initialProjectMu.Unlock()
	path := a.initialProjectPath
	a.initialProjectPath = ""
	return path
}

// VersionInfo describes the application version.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

// GetVersion returns the application version information.
func (a *App) GetVersion() VersionInfo {
	return VersionInfo{
		Version:   version.Version,
		Commit:    version.Commit,
		BuildDate: version.BuildDate,
	}
}

// FormatInfo describes a registered data format.
type FormatInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	MimeTypes   []string `json:"mime_types,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
	HasReader   bool     `json:"has_reader"`
	HasWriter   bool     `json:"has_writer"`
	Source      string   `json:"source"`
}

// ToolInfo describes an available tool.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// FlowInfo describes an available flow.
type FlowInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PluginInfo describes a loaded plugin.
type PluginInfo struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Source  string   `json:"source"`
	Formats []string `json:"formats"`
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

// GetKnownLocales returns a curated list of well-known BCP-47 locales with display names.
func (a *App) GetKnownLocales() []locale.LocaleInfo {
	return locale.WellKnownLocales()
}

// GetLocaleDisplayName returns the English display name for a BCP-47 locale code.
func (a *App) GetLocaleDisplayName(code string) string {
	return locale.DisplayName(model.LocaleID(code))
}

// ListFormats returns all registered formats with metadata.
func (a *App) ListFormats() []FormatInfo {
	regInfos := a.formatReg.FormatInfos()
	result := make([]FormatInfo, len(regInfos))
	for i, ri := range regInfos {
		result[i] = FormatInfo{
			Name:        ri.Name,
			DisplayName: ri.DisplayName,
			MimeTypes:   ri.MimeTypes,
			Extensions:  ri.Extensions,
			HasReader:   ri.HasReader,
			HasWriter:   ri.HasWriter,
			Source:      ri.Source,
		}
	}
	return result
}

// toolCategory maps tool names to their categories.
var toolCategory = map[string]string{
	// utility tools
	"word-count":     "utility",
	"char-count":     "utility",
	"segment-count":  "utility",
	"encoding-detect": "utility",
	// transform tools
	"pseudo-translate": "transform",
	"search-replace":   "transform",
	"case-transform":   "transform",
	"xslt-transform":   "transform",
	"segmentation":     "transform",
	// validate tools
	"xml-validation": "validate",
	"qa-check":       "validate",
	"term-check":     "validate",
	// enrich tools
	"tag-protect":  "enrich",
	"tm-leverage":  "enrich",
	// AI tools
	"ai-translate":   "transform",
	"ai-qa":          "validate",
	"ai-terminology": "enrich",
	"ai-review":      "validate",
}

// ListTools returns all available tools.
func (a *App) ListTools() []ToolInfo {
	var result []ToolInfo

	// Add tools from the registry.
	if a.toolReg != nil {
		for _, name := range a.toolReg.Names() {
			t, err := a.toolReg.NewTool(name)
			if err != nil {
				continue
			}
			category := toolCategory[name]
			if category == "" {
				category = "utility"
			}
			result = append(result, ToolInfo{
				Name:        name,
				Description: t.Description(),
				Category:    category,
			})
		}
	}

	// AI tools (not in tool registry, managed separately).
	aiTools := []ToolInfo{
		{Name: "ai-translate", Description: "Translate content using AI/LLM", Category: "transform"},
		{Name: "ai-qa", Description: "Quality check translations using AI", Category: "validate"},
		{Name: "ai-terminology", Description: "Extract terminology using AI", Category: "enrich"},
		{Name: "ai-review", Description: "Review translations using AI", Category: "validate"},
	}
	result = append(result, aiTools...)

	return result
}

// ListFlows returns all available flows (summary info).
func (a *App) ListFlows() []FlowInfo {
	defs := a.ListFlowDefinitions()
	result := make([]FlowInfo, len(defs))
	for i, d := range defs {
		result[i] = FlowInfo{
			Name:        d.ID,
			Description: d.Description,
		}
	}
	return result
}

// ListPlugins returns all loaded plugins.
func (a *App) ListPlugins() []PluginInfo {
	a.pluginMu.Lock()
	pl := a.pluginLoader
	a.pluginMu.Unlock()
	if pl == nil {
		return []PluginInfo{}
	}
	raw := pl.Plugins()
	out := make([]PluginInfo, len(raw))
	for i, p := range raw {
		out[i] = PluginInfo{
			Name:    p.Name,
			Type:    p.Type,
			Source:  p.Source,
			Formats: p.Formats,
		}
	}
	return out
}

// PluginDir returns the configured plugin directory path.
func (a *App) PluginDir() string {
	a.pluginMu.Lock()
	pl := a.pluginLoader
	a.pluginMu.Unlock()
	if pl == nil {
		return ""
	}
	return pl.Dir()
}

// ServiceShutdown is called by Wails v3 when the application exits.
func (a *App) ServiceShutdown() error {
	// Close all project TMs.
	for _, info := range a.projects.all() {
		if p, err := a.projects.get(info.ID); err == nil && p.tm != nil {
			p.tm.Close()
		}
	}
	a.pluginMu.Lock()
	pl := a.pluginLoader
	a.pluginMu.Unlock()
	if pl != nil {
		pl.Shutdown()
	}
	return nil
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
		return []tool.Tool{
			libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
				TargetLocale: model.LocaleID(tgtLang),
			}),
		}, nil
	case "qa-check":
		return []tool.Tool{
			libtools.NewQACheckTool(libtools.NewQACheckConfig(model.LocaleID(tgtLang))),
		}, nil
	case "segmentation":
		return []tool.Tool{
			libtools.NewSegmentationTool(&libtools.SegmentationConfig{
				TargetLocale: model.LocaleID(tgtLang),
			}),
		}, nil
	case "tm-leverage":
		return []tool.Tool{
			libtools.NewTMLeverageTool(&libtools.TMLeverageConfig{
				SourceLocale:   model.LocaleID(srcLang),
				TargetLocale:   model.LocaleID(tgtLang),
				FuzzyThreshold: 70,
				Provider:       libtools.NullTMProvider{},
			}),
		}, nil
	default:
		return nil, fmt.Errorf("unknown flow: %q", flowName)
	}
}
