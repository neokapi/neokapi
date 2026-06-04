package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
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
	JQ             string // optional jq filter for JSON output
	Colorize       bool   // ANSI-color JSON output
	FailOnUnknown  bool
	NoWarn         bool
	Progress       bool
	OutputTemplate string
	// InPlace: write back to the input file path instead of expanding
	// OutputTemplate. Enabled automatically when all inputs are KLF
	// files and -o was omitted — KLF writers are locale-additive,
	// so in-place accumulates translations rather than clobbering.
	InPlace bool
	// DefaultLayout: no -o/--output-dir was given, so resolve each output
	// path with localeOutputPath — swap the source locale in the input path
	// if present, else write under a {lang}/ directory beside the input.
	DefaultLayout bool
	TargetLang    string
	// Pack: when the input is a .klz workspace, auto-eject the transform to
	// the .klz (the --pack flag); otherwise the cache is left dirty.
	Pack           bool
	TracePath      string // write flow trace JSON to this file
	ParallelBlocks int    // fan out block processing across N goroutines (0 = off)
	NewTool        func() (tool.Tool, error)
	NewCollector   func() flow.Collector
	AfterTool      func() // called after tool execution completes (e.g. to clear progress)
}

// RunToolOnFiles processes each file through a single-tool flow and
// aggregates results via the collector. Files are processed in parallel.
func (a *App) RunToolOnFiles(ctx context.Context, cfg ToolRunConfig) error {
	// A tool run on a single .klz transforms the workspace IN PLACE; output
	// files come later from `kapi merge`.
	if klzWorkspaceInput(cfg.Files) {
		if cfg.OutputTemplate != "" {
			return errKlzTransformOutput
		}
		return a.transformKlzInPlace(ctx, cfg.Files[0], cfg.ToolName, func() ([]tool.Tool, func(), error) {
			t, terr := cfg.NewTool()
			if terr != nil {
				return nil, nil, terr
			}
			return []tool.Tool{t}, nil, nil
		}, cfg.TargetLang, a.toolDefaultLocale(cfg.ToolName), cfg.Pack)
	}
	if isKlzPath(cfg.OutputTemplate) {
		return errKlzCreateWithExtract
	}

	files, err := resolveFiles(cfg.Files)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("no files to process")
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

	var bar progressBar
	var progress progressGroup
	var active atomic.Int64
	if showProgress {
		progress, bar = newProgress(len(files), &active)
	}

	// Batch trace mode: when --trace is set without a {name} placeholder
	// and the run covers >1 files, capture per-file lane + timing into a
	// single BatchFlowTrace and write it once at the end. The pseudobench
	// concurrency view consumes this exact shape; without it that chart
	// renders empty even though kapi processed everything in parallel.
	// Per-file trace mode (with {name}) keeps its existing behaviour —
	// the placeholder signals the user wants individual flow traces.
	batchTrace := cfg.TracePath != "" && !strings.Contains(cfg.TracePath, "{name}") && len(files) > 1
	var batchStart time.Time
	var lanes chan int
	var traceMu sync.Mutex
	var traceInfos []*batchTraceInfo
	if batchTrace {
		batchStart = time.Now()
		lanes = make(chan int, concurrency)
		for i := range concurrency {
			lanes <- i
		}
	}

	g, ctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, concurrency)

	for _, file := range files {
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			active.Add(1)

			var info *batchTraceInfo
			if batchTrace {
				lane := <-lanes
				info = &batchTraceInfo{
					file:    filepath.Base(file),
					startUs: time.Since(batchStart).Microseconds(),
					lane:    lane,
				}
				defer func() {
					info.endUs = time.Since(batchStart).Microseconds()
					traceMu.Lock()
					traceInfos = append(traceInfos, info)
					traceMu.Unlock()
					lanes <- lane
				}()
			}

			// Suppress per-file trace writes in batch mode — the path has no
			// {name} placeholder so every file would clobber the same target.
			runCfg := cfg
			if batchTrace {
				runCfg.TracePath = ""
			}
			err := a.processOneFile(ctx, runCfg, file, collector, commonDir, progress, info)
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

	if batchTrace {
		bt := &flow.BatchFlowTrace{
			Name:        cfg.ToolName,
			Concurrency: concurrency,
			DurationUs:  time.Since(batchStart).Microseconds(),
		}
		for _, info := range traceInfos {
			bt.FileTraces = append(bt.FileTraces, flow.FileFlowTrace{
				File:       info.file,
				Format:     info.format,
				StartUs:    info.startUs,
				EndUs:      info.endUs,
				Lane:       info.lane,
				DurationUs: info.endUs - info.startUs,
			})
		}
		traceJSON, err := json.MarshalIndent(bt, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal batch trace: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(cfg.TracePath), 0o755); err != nil {
			return fmt.Errorf("create trace dir: %w", err)
		}
		if err := os.WriteFile(cfg.TracePath, traceJSON, 0o644); err != nil {
			return fmt.Errorf("write batch trace: %w", err)
		}
	}

	if collector == nil {
		return nil
	}

	result, err := collector.Result()
	if err != nil {
		return fmt.Errorf("collector result: %w", err)
	}
	return output.FormatCollectorResult(cfg.JSONOutput, cfg.JQ, cfg.Colorize, result.Data)
}

// batchTraceInfo holds per-file timing for the batch-trace assembler in
// RunToolOnFiles. Lane is allocated before processOneFile runs; format is
// filled in once the reader resolves it.
type batchTraceInfo struct {
	file    string
	format  string
	startUs int64
	endUs   int64
	lane    int
}

func (a *App) processOneFile(ctx context.Context, cfg ToolRunConfig, filePath string, collector flow.Collector, commonDir string, progress progressGroup, batchInfo *batchTraceInfo) error {
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
		fmtName = string(detected)
	}

	ref := preset.ParseFormatRef(fmtName)
	registryName := ref.RegistryName()

	if batchInfo != nil {
		batchInfo.format = registryName
	}

	reader, err := a.FormatReg.NewReader(registry.FormatID(registryName))
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
		presetReg := preset.NewPresetRegistry()
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

	// Resolve the output path once, before creating the writer, so the
	// writer-format probe and the final write agree on the destination.
	producesOutput := cfg.OutputTemplate != "" || cfg.InPlace || cfg.DefaultLayout
	var outputPath string
	switch {
	case !producesOutput:
		// Tool produces no output file (e.g. an analysis tool).
	case cfg.InPlace:
		outputPath = filePath
	case cfg.OutputTemplate != "":
		outputPath = expandOutputPath(cfg.OutputTemplate, filePath, commonDir, cfg.TargetLang)
	default: // cfg.DefaultLayout
		outputPath = localeOutputPath(filePath, a.SourceLang, cfg.TargetLang)
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
	}

	// Create writer early so we can wire skeleton store before reading.
	// Writer format defaults to the reader's format (same-in / same-out
	// round-trip) but a different output extension selects a different
	// writer — that's how "convert .json → .klf → .mo" works without a
	// dedicated --writer flag. The output path's extension IS the user's
	// intent declaration.
	var writer format.DataFormatWriter
	if producesOutput {
		writerFormatName := registryName
		if !cfg.InPlace {
			if ext := filepath.Ext(outputPath); ext != "" {
				if det, err := a.FormatReg.DetectByExtension(ext); err == nil && det != "" {
					writerFormatName = string(det)
				}
			}
		}
		var err error
		writer, err = a.FormatReg.NewWriter(registry.FormatID(writerFormatName))
		if err != nil {
			if !cfg.FailOnUnknown {
				if !cfg.NoWarn {
					warnf(progress, "Warning: skipping %q: %v\n", filePath, err)
				}
				return nil
			}
			return fmt.Errorf("no writer for format %q: %w", writerFormatName, err)
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
			item := &flow.Item{
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
			{ID: "reader", Type: flow.NodeReader, Name: fmtName, Label: fmtName + " reader"},
			{ID: "tool-0", Type: flow.NodeTool, Name: t.Name(), Label: t.Name()},
			{ID: "writer", Type: flow.NodeWriter, Name: fmtName, Label: fmtName + " writer"},
		}
		t = flow.NewTracingTool(t, "tool-0", recorder)
	}

	fb := flow.NewFlow(cfg.ToolName)
	fb.AddTool(t)
	f, err := fb.Build()
	if err != nil {
		return fmt.Errorf("build flow: %w", err)
	}

	executor := flow.NewExecutor()

	// Derive a cancellable context for the feeder so a tool error (which
	// cancels the executor's own internal context and stops the tools) can
	// also unblock the feeder if it's parked on an inCh send. Without this
	// the feeder goroutine would leak once more than a channel's worth of
	// parts (default 64) remain unsent. feedDone lets us join it.
	feedCtx, feedCancel := context.WithCancel(ctx)
	defer feedCancel()

	inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f)

	feedDone := make(chan struct{})
	go func() {
		defer close(feedDone)
		defer close(inCh)
		for _, p := range parts {
			if recorder != nil && p.Resource != nil {
				recorder.SnapshotPart(p, "reader", "initial")
				recorder.Record("exit", "reader", p.Resource.ResourceID(), nil)
			}
			select {
			case inCh <- p:
			case <-feedCtx.Done():
				return
			}
		}
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

	waitErr := wait()
	// The executor has finished draining; cancel and join the feeder so it
	// never leaks — on a tool error the tools stop early and the feeder may
	// still be parked on an inCh send for the parts past the channel buffer.
	feedCancel()
	<-feedDone

	if waitErr != nil {
		if cfg.AfterTool != nil {
			cfg.AfterTool()
		}
		return fmt.Errorf("tool execution on %s: %w", filePath, waitErr)
	}

	if cfg.AfterTool != nil {
		cfg.AfterTool()
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

	if producesOutput && writer != nil {
		if err := writer.SetOutput(outputPath); err != nil {
			return fmt.Errorf("set output %s: %w", outputPath, err)
		}

		// Prefer passing the file path over loading content bytes when the writer
		// supports it. This avoids duplicating the file in memory for gRPC transfer.
		if sps, ok := writer.(format.SourcePathSetter); ok && filepath.IsAbs(filePath) {
			sps.SetSourcePath(filePath)
		} else if ocs, ok := writer.(format.OriginalContentSetter); ok {
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

		// Print the destination only when stderr is an interactive terminal:
		// a TTY-only nicety, silent when piped or scripted (matching the flow
		// path's progress gating). The result file is the artifact; this line
		// is a diagnostic. --quiet and the progress bar suppress it too.
		if !a.Quiet && progress == nil && isatty.IsTerminal(os.Stderr.Fd()) {
			fmt.Fprintf(os.Stderr, "%s → %s\n", filePath, outputPath)
		}
		return nil
	}

	// Feed collector — skip if streaming collector already observed inline.
	if collector != nil && streamingCollector == nil {
		item := &flow.Item{
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
				walked, err := walkDirFiles(m)
				if err != nil {
					return nil, err
				}
				files = append(files, walked...)
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

// walkDirFiles recursively collects regular files under root, skipping
// hidden directories and junk files. Order is lexicographic for
// determinism.
func walkDirFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			// Skip hidden directories (e.g. .git) but never the root itself.
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if isJunkFile(name) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %q: %w", root, err)
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
	result = strings.ReplaceAll(result, "{dir}", filepath.Dir(filePath))
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

// localeOutputPath computes the default output path for an input file when no
// -o template or --output-dir was given. The aim is to land translated files
// where the project's own localization layout expects them:
//
//   - If the source locale appears in the input path — as a directory segment
//     (locales/en/app.json) or a filename token (app.en.json, app_en.arb,
//     en.json) — it is swapped for the target locale, writing beside the
//     source. A directory match is preferred over a filename match.
//   - Otherwise the file is placed under a {lang}/ directory beside the input
//     (messages.json → fr/messages.json).
//
// The source locale matches case-insensitively against a whole path segment or
// filename token, by either its full tag or its primary subtag (so en-US dirs
// match a source of "en", and vice versa). It performs no filesystem access;
// callers create the parent directory.
func localeOutputPath(filePath, sourceLang, targetLang string) string {
	if swapped, ok := swapLocaleSegment(filePath, sourceLang, targetLang); ok {
		return swapped
	}
	base := filepath.Base(filePath)
	if swapped, ok := swapLocaleToken(base, sourceLang, targetLang); ok {
		return filepath.Join(filepath.Dir(filePath), swapped)
	}
	return filepath.Join(filepath.Dir(filePath), targetLang, base)
}

// localeSubtags returns the lower-cased forms a locale tag may be matched by:
// its full tag and, when region-qualified, its primary language subtag.
func localeSubtags(lang string) []string {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return nil
	}
	full := strings.ToLower(lang)
	out := []string{full}
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		if primary := strings.ToLower(lang[:i]); primary != full {
			out = append(out, primary)
		}
	}
	return out
}

// localeMatches reports whether candidate is the source locale, compared as a
// whole token (case-insensitive) against the source's full tag or primary
// subtag.
func localeMatches(candidate, sourceLang string) bool {
	c := strings.ToLower(candidate)
	for _, s := range localeSubtags(sourceLang) {
		if c == s {
			return true
		}
	}
	return false
}

// swapLocaleSegment replaces the deepest directory segment that matches the
// source locale with the target locale. The filename is left untouched.
func swapLocaleSegment(filePath, sourceLang, targetLang string) (string, bool) {
	dir := filepath.Dir(filePath)
	if dir == "." || dir == string(filepath.Separator) {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(dir), "/")
	idx := -1
	for i, p := range parts {
		if localeMatches(p, sourceLang) {
			idx = i // prefer the deepest match (closest to the file)
		}
	}
	if idx < 0 {
		return "", false
	}
	parts[idx] = targetLang
	newDir := filepath.FromSlash(strings.Join(parts, "/"))
	return filepath.Join(newDir, filepath.Base(filePath)), true
}

// swapLocaleToken replaces a locale token within a filename (split on '.', '_',
// or '-') with the target locale, preserving the extension and separators.
// "app.en.json" → "app.fr.json", "en.json" → "fr.json", "app_en.arb" →
// "app_fr.arb".
func swapLocaleToken(base, sourceLang, targetLang string) (string, bool) {
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		return "", false
	}

	type token struct {
		text string
		sep  byte // trailing separator, 0 for the final token
	}
	var toks []token
	start := 0
	for i := range len(name) {
		if c := name[i]; c == '.' || c == '_' || c == '-' {
			toks = append(toks, token{name[start:i], c})
			start = i + 1
		}
	}
	toks = append(toks, token{name[start:], 0})

	idx := -1
	for i, t := range toks {
		if localeMatches(t.text, sourceLang) {
			idx = i // prefer the last matching token (e.g. app.en → en)
		}
	}
	if idx < 0 {
		return "", false
	}
	toks[idx].text = targetLang

	var b strings.Builder
	for _, t := range toks {
		b.WriteString(t.text)
		if t.sep != 0 {
			b.WriteByte(t.sep)
		}
	}
	return b.String() + ext, true
}

func warnf(progress progressGroup, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if progress != nil {
		fmt.Fprint(progress, msg)
	} else {
		fmt.Fprint(os.Stderr, msg)
	}
}
