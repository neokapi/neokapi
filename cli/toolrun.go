package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/gokapi/gokapi/cli/output"
	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/loader"
	pluginreg "github.com/gokapi/gokapi/core/plugin/registry"
	"github.com/gokapi/gokapi/core/preset"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/mattn/go-isatty"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/sync/errgroup"
)

// FormatMapping maps a glob pattern to a format reference string.
type FormatMapping struct {
	Pattern string // glob pattern (matched against filename)
	Format  string // format reference (e.g. "okf_openxml:wellFormed")
}

// ParseFormatMappings parses "-m" flag values in "pattern=format" form.
func ParseFormatMappings(values []string) ([]FormatMapping, error) {
	mappings := make([]FormatMapping, 0, len(values))
	for _, v := range values {
		i := strings.LastIndex(v, "=")
		if i <= 0 {
			return nil, fmt.Errorf("invalid format mapping %q: expected pattern=format", v)
		}
		mappings = append(mappings, FormatMapping{
			Pattern: v[:i],
			Format:  v[i+1:],
		})
	}
	return mappings, nil
}

// matchFormatMapping returns the format for the first mapping whose pattern
// matches the file's base name. Returns "" if no mapping matches.
func matchFormatMapping(filePath string, mappings []FormatMapping) string {
	base := filepath.Base(filePath)
	for _, m := range mappings {
		if matched, _ := filepath.Match(m.Pattern, base); matched {
			return m.Format
		}
	}
	return ""
}

// ToolRunConfig configures RunToolOnFiles.
type ToolRunConfig struct {
	ToolName       string
	Files          []string
	FormatMappings []FormatMapping
	Concurrency    int
	JSONOutput     bool
	FailOnUnknown  bool
	NoWarn         bool
	Progress       bool
	OutputTemplate string
	TargetLang     string
	TracePath      string // write flow trace JSON to this file
	ParallelBlocks int    // fan out block processing across N goroutines (0 = off)
	NewTool        func() (tool.Tool, error)
	NewCollector   func() flow.Collector
}

// RunToolOnFiles processes each file through a single-tool flow and
// aggregates results via the collector. Files are processed in parallel.
func (a *App) RunToolOnFiles(ctx context.Context, cfg ToolRunConfig) error {
	files, err := resolveFiles(cfg.Files)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no files to process")
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	var collector flow.Collector
	if cfg.NewCollector != nil {
		collector = cfg.NewCollector()
	}

	var commonDir string
	if cfg.OutputTemplate != "" {
		commonDir = commonDirPrefix(files)
	}

	// Auto-enable progress bar for multi-file runs on a TTY (unless JSON output).
	showProgress := cfg.Progress
	if !showProgress && !cfg.JSONOutput && len(files) > 1 && isatty.IsTerminal(os.Stderr.Fd()) {
		showProgress = true
	}

	var bar *mpb.Bar
	var progress *mpb.Progress
	var active atomic.Int64
	if showProgress {
		progress = mpb.New(mpb.WithOutput(os.Stderr))
		bar = progress.New(int64(len(files)),
			mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Padding(" ").Rbound("]"),
			mpb.PrependDecorators(decor.Elapsed(decor.ET_STYLE_GO)),
			mpb.AppendDecorators(
				decor.CountersNoUnit("[%d/%d]"),
				decor.Any(func(s decor.Statistics) string {
					n := active.Load()
					if n > 0 {
						return fmt.Sprintf(" (%d active)", n)
					}
					return ""
				}),
			),
		)
	}

	g, ctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, concurrency)

	for _, file := range files {
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			active.Add(1)
			err := a.processOneFile(ctx, cfg, file, collector, commonDir, progress)
			active.Add(-1)
			if bar != nil {
				bar.Increment()
			}
			return err
		})
	}

	if err := g.Wait(); err != nil {
		if progress != nil {
			progress.Wait()
		}
		return err
	}

	if progress != nil {
		progress.Wait()
	}

	if collector == nil {
		return nil
	}

	result, err := collector.Result()
	if err != nil {
		return fmt.Errorf("collector result: %w", err)
	}
	return output.FormatCollectorResult(cfg.JSONOutput, result.Data)
}

func (a *App) processOneFile(ctx context.Context, cfg ToolRunConfig, filePath string, collector flow.Collector, commonDir string, progress *mpb.Progress) error {
	// Resolve format: mapping > global -f flag > auto-detect by extension.
	fmtName := matchFormatMapping(filePath, cfg.FormatMappings)
	if fmtName == "" {
		fmtName = a.FormatFlag
	}
	if fmtName == "" {
		ext := filepath.Ext(filePath)
		detected, err := a.FormatReg.DetectByExtension(ext)
		if err != nil {
			if !cfg.FailOnUnknown {
				if !cfg.NoWarn {
					warnf(progress, "Warning: skipping %q: %v\n", filePath, err)
				}
				return nil
			}
			return fmt.Errorf("unable to detect format for %q: %w", filePath, err)
		}
		fmtName = detected
	}

	ref := pluginreg.ParseFormatRef(fmtName)
	registryName := ref.RegistryName()

	reader, err := a.FormatReg.NewReader(registryName)
	if err != nil {
		if !cfg.FailOnUnknown {
			if !cfg.NoWarn {
				warnf(progress, "Warning: skipping %q: %v\n", filePath, err)
			}
			return nil
		}
		return fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}

	if ref.IsPreset() {
		presetReg := a.PluginLoader.Presets()
		preset.RegisterBuiltins(presetReg)
		resolver := preset.NewConfigResolver(presetReg, a.SchemaReg)

		mergedConfig, err := resolver.ResolveFormatConfig(ref.Name, ref.Preset, nil, nil)
		if err != nil {
			return fmt.Errorf("resolve format config: %w", err)
		}
		if len(mergedConfig) > 0 {
			if cfg := reader.Config(); cfg != nil {
				if err := cfg.ApplyMap(mergedConfig); err != nil {
					return fmt.Errorf("apply format config: %w", err)
				}
			}
		}
	}

	// Create writer early so we can wire skeleton store before reading.
	var writer format.DataFormatWriter
	if cfg.OutputTemplate != "" {
		var err error
		writer, err = a.FormatReg.NewWriter(registryName)
		if err != nil {
			if !cfg.FailOnUnknown {
				if !cfg.NoWarn {
					warnf(progress, "Warning: skipping %q: %v\n", filePath, err)
				}
				return nil
			}
			return fmt.Errorf("no writer for format %q: %w", fmtName, err)
		}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	// Wire skeleton store if both reader and writer support it.
	if writer != nil {
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
	}

	doc := &model.RawDocument{
		URI:          filePath,
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		if !cfg.FailOnUnknown {
			if !cfg.NoWarn {
				warnf(progress, "Warning: skipping %q: %v\n", filePath, err)
			}
			return nil
		}
		return fmt.Errorf("open %s: %w", filePath, err)
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			if !cfg.FailOnUnknown {
				if !cfg.NoWarn {
					warnf(progress, "Warning: skipping %q: %v\n", filePath, result.Error)
				}
				return nil
			}
			return fmt.Errorf("read %s: %w", filePath, result.Error)
		}
		parts = append(parts, result.Part)
	}
	reader.Close()

	t, err := cfg.NewTool()
	if err != nil {
		return fmt.Errorf("create tool: %w", err)
	}

	// Wrap with ParallelBlockTool if requested.
	if cfg.ParallelBlocks > 1 {
		t = tool.NewParallelBlockTool(t, cfg.ParallelBlocks)
	}

	// If the collector supports streaming, set document context and wrap the
	// tool with TappingTool so word counts (etc.) accumulate inline as parts
	// flow through, without buffering the entire stream for post-hoc counting.
	var streamingCollector flow.StreamingCollector
	if collector != nil {
		if sc, ok := collector.(flow.StreamingCollector); ok {
			streamingCollector = sc
			// Set document context before processing.
			item := &flow.FlowItem{
				Input:        doc,
				TargetLocale: model.LocaleID(cfg.TargetLang),
			}
			if err := sc.Collect(ctx, item, nil); err != nil {
				return fmt.Errorf("collector context for %s: %w", filePath, err)
			}
			t = flow.NewTappingTool(t, sc)
		}
	}

	// If --trace is set, wrap the tool with TracingTool.
	// The trace path supports {name} and {ext} placeholders for multi-file runs.
	var recorder *flow.TraceRecorder
	var traceNodes []flow.TraceNode
	resolvedTracePath := cfg.TracePath
	if resolvedTracePath != "" {
		ext := filepath.Ext(filePath)
		name := strings.TrimSuffix(filepath.Base(filePath), ext)
		extNoDot := strings.TrimPrefix(ext, ".")
		resolvedTracePath = strings.ReplaceAll(resolvedTracePath, "{name}", name)
		resolvedTracePath = strings.ReplaceAll(resolvedTracePath, "{ext}", extNoDot)
		recorder = flow.NewTraceRecorder()
		traceNodes = []flow.TraceNode{
			{ID: "reader", Type: "reader", Name: fmtName, Label: fmtName + " reader"},
			{ID: "tool-0", Type: "tool", Name: t.Name(), Label: t.Name()},
			{ID: "writer", Type: "writer", Name: fmtName, Label: fmtName + " writer"},
		}
		t = flow.NewTracingTool(t, "tool-0", recorder)
	}

	fb := flow.NewFlow(cfg.ToolName)
	fb.AddTool(t)
	f := fb.Build()

	executor := flow.NewFlowExecutor()
	inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f)

	go func() {
		for _, p := range parts {
			if recorder != nil && p.Resource != nil {
				recorder.SnapshotPart(p, "reader", "initial")
				recorder.Record("exit", "reader", p.Resource.ResourceID(), nil)
			}
			inCh <- p
		}
		close(inCh)
	}()

	var outputParts []*model.Part
	for p := range outCh {
		if recorder != nil && p.Resource != nil {
			id := p.Resource.ResourceID()
			recorder.Record("enter", "writer", id, nil)
			recorder.Record("exit", "writer", id, nil)
		}
		outputParts = append(outputParts, p)
	}

	if err := wait(); err != nil {
		return fmt.Errorf("tool execution on %s: %w", filePath, err)
	}

	// Write trace JSON if --trace was set.
	if resolvedTracePath != "" && recorder != nil {
		inputPreview := string(content)
		if len(inputPreview) > 2000 {
			inputPreview = inputPreview[:2000] + "\n... (truncated)"
		}
		traceData := &flow.FlowTrace{
			Name:        cfg.ToolName,
			Description: fmt.Sprintf("%s on %s", cfg.ToolName, filepath.Base(filePath)),
			Nodes:       traceNodes,
			ChannelSize: 64,
			Events:      recorder.Events(),
			Parts:       recorder.Snapshots(),
			InputFile:   flow.TraceFile{Name: filepath.Base(filePath), Format: fmtName, Preview: inputPreview},
			OutputFile:  flow.TraceFile{Name: filepath.Base(filePath)},
			DurationUs:  recorder.DurationUs(),
		}
		traceJSON, err := json.MarshalIndent(traceData, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal trace: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(resolvedTracePath), 0o755); err != nil {
			return fmt.Errorf("create trace dir: %w", err)
		}
		if err := os.WriteFile(resolvedTracePath, traceJSON, 0o644); err != nil {
			return fmt.Errorf("write trace: %w", err)
		}
	}

	if cfg.OutputTemplate != "" && writer != nil {
		outputPath := expandOutputPath(cfg.OutputTemplate, filePath, commonDir, cfg.TargetLang)

		if err := writer.SetOutput(outputPath); err != nil {
			return fmt.Errorf("set output %s: %w", outputPath, err)
		}

		// Prefer passing the file path over loading content bytes when the writer
		// supports it. This avoids duplicating the file in memory for gRPC transfer.
		if sps, ok := writer.(loader.SourcePathSetter); ok && filepath.IsAbs(filePath) {
			sps.SetSourcePath(filePath)
		} else if ocs, ok := writer.(loader.OriginalContentSetter); ok {
			ocs.SetOriginalContent(content)
		}

		locale := model.LocaleID(cfg.TargetLang)
		writer.SetLocale(locale)

		ch := make(chan *model.Part, len(outputParts))
		for _, p := range outputParts {
			ch <- p
		}
		close(ch)

		if err := writer.Write(ctx, ch); err != nil {
			return fmt.Errorf("write output %s: %w", outputPath, err)
		}
		if err := writer.Close(); err != nil {
			return fmt.Errorf("close writer %s: %w", outputPath, err)
		}

		if !a.Quiet && progress == nil {
			fmt.Fprintf(os.Stderr, "%s → %s\n", filePath, outputPath)
		}
		return nil
	}

	// Feed collector — skip if streaming collector already observed inline.
	if collector != nil && streamingCollector == nil {
		item := &flow.FlowItem{
			Input:        doc,
			TargetLocale: model.LocaleID(cfg.TargetLang),
		}
		if err := collector.Collect(ctx, item, outputParts); err != nil {
			return fmt.Errorf("collect %s: %w", filePath, err)
		}
	}

	return nil
}

func resolveFiles(patterns []string) ([]string, error) {
	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
		if matches == nil {
			matches = []string{pattern}
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				return nil, fmt.Errorf("stat %q: %w", m, err)
			}
			if info.IsDir() {
				continue
			}
			if isJunkFile(filepath.Base(m)) {
				continue
			}
			files = append(files, m)
		}
	}
	return files, nil
}

func isJunkFile(name string) bool {
	return strings.HasPrefix(name, "~$") || strings.HasPrefix(name, "._")
}

func commonDirPrefix(files []string) string {
	if len(files) == 0 {
		return ""
	}
	if len(files) == 1 {
		return filepath.Dir(files[0]) + string(filepath.Separator)
	}

	prefix := filepath.Dir(files[0])
	for _, f := range files[1:] {
		dir := filepath.Dir(f)
		prefix = commonPath(prefix, dir)
		if prefix == "" {
			return ""
		}
	}
	if prefix != "" && !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return prefix
}

func commonPath(a, b string) string {
	aParts := strings.Split(filepath.ToSlash(a), "/")
	bParts := strings.Split(filepath.ToSlash(b), "/")
	n := min(len(bParts), len(aParts))
	var common []string
	for i := range n {
		if aParts[i] != bParts[i] {
			break
		}
		common = append(common, aParts[i])
	}
	if len(common) == 0 {
		return ""
	}
	return filepath.FromSlash(strings.Join(common, "/"))
}

func expandOutputPath(tmpl, filePath, commonDir, lang string) string {
	rel := filePath
	if commonDir != "" {
		if r, err := filepath.Rel(commonDir, filePath); err == nil {
			rel = r
		}
	}

	ext := filepath.Ext(rel)
	name := strings.TrimSuffix(rel, ext)
	extNoDot := strings.TrimPrefix(ext, ".")

	result := tmpl
	result = strings.ReplaceAll(result, "{name}", name)
	result = strings.ReplaceAll(result, "{ext}", extNoDot)
	result = strings.ReplaceAll(result, "{lang}", lang)

	isDir := strings.HasSuffix(result, "/") || strings.HasSuffix(result, string(filepath.Separator))
	if !isDir {
		if info, err := os.Stat(result); err == nil && info.IsDir() {
			isDir = true
		}
	}
	if !isDir && filepath.Ext(result) == "" {
		isDir = true
	}
	if isDir {
		result = filepath.Join(result, rel)
	}

	dir := filepath.Dir(result)
	_ = os.MkdirAll(dir, 0o755)

	return result
}

func warnf(progress *mpb.Progress, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if progress != nil {
		fmt.Fprint(progress, msg)
	} else {
		fmt.Fprint(os.Stderr, msg)
	}
}
