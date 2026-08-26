package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"io"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/safeio"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/flowdef"
	"github.com/neokapi/neokapi/host/output"
	sqlmemory "github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
	sqlterms "github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"golang.org/x/sync/errgroup"
)

// FlowCmdOptions configures the flow and run commands.
type FlowCmdOptions struct {
	// FallbackRunE is called when the flow name doesn't match a built-in flow.
	// If nil, unknown flow names return an error.
	FallbackRunE func(cmd Command, flowName string, args []string) error

	// ExtraFlows returns additional flows for the list command (e.g. project flows).
	ExtraFlows func() []output.FlowInfo
}

// RunFlow executes a flow by name with the given input files.
func (a *App) RunFlow(ctx context.Context, cmd Command, flowName string, opts FlowCmdOptions) error {
	inputPaths, _ := cmd.Flags().GetStringSlice("input")
	// Directories and globs expand exactly as the tool runner expands them
	// (recursive, hidden dirs and junk files skipped), so the flow-backed
	// porcelain keeps the old tool commands' ergonomics: `kapi
	// pseudo-translate frontend/i18n` processes the catalog directory.
	if len(inputPaths) > 0 {
		expanded, err := resolveFiles(inputPaths)
		if err != nil {
			return err
		}
		inputPaths = expanded
	}
	concurrency, _ := cmd.Flags().GetInt("concurrency")

	if explain, _ := cmd.Flags().GetBool("explain"); explain {
		out, _ := cmd.Flags().GetString("output")
		return explainBindings(cmd.OutOrStdout(), flowName, inputPaths, out)
	}

	if len(inputPaths) > 0 {
		outputFlag, _ := cmd.Flags().GetString("output")
		// Running a flow on a .kpz transforms the workspace IN PLACE
		// (appends overlays); output files come later from `kapi merge`. The
		// target locale may come from the workspace recipe, so this runs
		// before the --target-lang requirement below.
		if kpzWorkspaceInput(inputPaths) {
			if outputFlag != "" {
				return errKpzTransformOutput
			}
			doPack, _ := cmd.Flags().GetBool("pack")
			return a.transformKpzInPlace(ctx, inputPaths[0], flowName, func() ([]tool.Tool, func(), error) {
				return a.buildFlowTools(flowName, inputPaths[0], cmd)
			}, a.TargetLang, "", doPack)
		}
		if a.TargetLang == "" {
			// Check tool registry for a default locale (e.g., pseudo-translate → "qps").
			if info := a.ToolReg.ToolInfo(registry.ToolID(flowName)); info != nil && info.DefaultLocale != "" {
				a.TargetLang = string(info.DefaultLocale)
			} else {
				return errors.New("--target-lang is required")
			}
		}
		if IsKpzPath(outputFlag) {
			return errKpzCreateWithExtract
		}
		if len(inputPaths) == 1 {
			return a.RunSingleFile(ctx, cmd, flowName, inputPaths[0])
		}
		return a.runMultipleFiles(ctx, cmd, flowName, inputPaths, concurrency, outputFlag)
	}

	// No --input: try fallback (e.g. project flow). Read at run time so
	// plugin App initializers have already installed the App-level
	// FallbackRunE.
	fallback := opts.FallbackRunE
	if fallback == nil {
		fallback = a.FallbackRunE
	}
	if fallback != nil {
		return fallback(cmd, flowName, []string{flowName})
	}
	return errors.New("--input (-i) is required")
}

// AddFlowRunFlags registers the common flow-execution flags (provider/model/
// tm/terms/source-lang/target-lang/parallel-blocks/trace/…) on cmd. The
// built-in `kapi up` gets these via NewUpCmd; the kapi-bowrain plugin, which
// owns the `up` verb when installed, calls this so its up presents the exact
// same flag surface and ExecuteUp stays byte-identical.
func (a *App) AddFlowRunFlags(cmd Command) { a.addFlowRunFlags(cmd) }

// addFlowRunFlags registers the common flags for flow execution commands.
func (a *App) addFlowRunFlags(cmd Command) {
	a.AddProcessingFlags(cmd)
	cmd.Flags().StringSliceP("input", "i", nil, "input file path(s); repeat for multiple files")
	cmd.Flags().StringP("output", "o", "", "output path or template (e.g. ./out/{name}_{lang}.{ext})")
	cmd.Flags().IntP("concurrency", "j", 0, "number of files to process at once (0 = auto)")
	cmd.Flags().String("credential", "", "saved credential name to use (see 'kapi credentials list')")
	cmd.Flags().String("provider", "anthropic", "AI provider (anthropic, openai, ollama)")
	cmd.Flags().String("api-key", "", "API key for the AI provider")
	cmd.Flags().String("model", "", "AI model name")
	cmd.Flags().String("instruction", "", "extra guidance for the model while translating (e.g. \"informal register; keep product names in English\")")
	cmd.Flags().String("context", "", "what the model is told about a block besides the block: none, key (default), neighbours")
	cmd.Flags().String("batching", "", "how many blocks share one LLM call: auto (default), single")
	cmd.Flags().String("trace", "", "write flow trace JSON to file (for flow visualization)")
	cmd.Flags().Bool("pack", false, "when transforming a .kpz, also eject the result to the .kpz (auto-pack)")
	cmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
	cmd.Flags().String("memory", "", "named Memory for recycle flow (resolves from KAPI_HOME)")
	cmd.Flags().String("termstore", "", "named terms store for term-lookup/enforce (resolves from KAPI_HOME)")
	cmd.Flags().Bool("stats", false, "include part/block counts in output")
	cmd.Flags().Bool("explain", false, "print the resolved source → sink bindings and exit without running")
	// The exec commands have carried this pair since they gained a file list;
	// the flow commands take a file list too and had neither, so there was no
	// way to ask a flow run to be strict about a format it could not read.
	cmd.Flags().Bool("fail-on-unknown", false, "exit with error if any file cannot be processed (default: skip with a warning)")
	cmd.Flags().Bool("strict", false, "alias for --fail-on-unknown")
}

// explainBindings resolves and prints the source → sink bindings for a flow run
// without executing it (kapi run --explain). It mirrors the precedence used at
// run time: an explicit -i/-o locator wins; with no -i the source is the project
// store; with no -o a store source stays in the store (process-only) and a file
// source writes a file. See AD-026.
func explainBindings(w io.Writer, flowName string, inputPaths []string, outputFlag string) error {
	var src flow.Locator
	if len(inputPaths) > 0 {
		src = flow.ParseLocator(inputPaths[0])
	} else {
		src = flow.Locator{Scheme: flow.SchemeStore}
	}

	var sink flow.Locator
	switch {
	case outputFlag != "":
		sink = flow.ParseLocator(outputFlag)
	case src.Kind() == flow.BindingStore:
		sink = flow.Locator{Scheme: flow.SchemeStore}
	default:
		sink = flow.Locator{Scheme: flow.SchemeFile}
	}

	_, err := fmt.Fprintf(w, "flow %s: %s → %s\n", flowName, src.Explain(), sink.Explain())
	return err
}

// explainProjectFlowRun renders the plan for a project-defined flow run
// without executing it — the project-path counterpart of explainBindings
// (kapi run <project-flow> --explain), sharing its source → sink rendering.
// One binding line per resolved input (explicit -i or content-derived), then
// the locale pass(es) the run would execute. Plan only: no tool is built, no
// store or content memory is opened, and nothing is written (#1295).
func explainProjectFlowRun(w io.Writer, flowName string, inputPaths []string, outputFlag string, locales []string) error {
	for _, in := range inputPaths {
		if err := explainBindings(w, flowName, []string{in}, outputFlag); err != nil {
			return err
		}
	}
	shown := make([]string, 0, len(locales))
	for _, l := range locales {
		if l != "" {
			shown = append(shown, l)
		}
	}
	if len(shown) > 0 {
		if _, err := fmt.Fprintf(w, "locales: %s\n", strings.Join(shown, ", ")); err != nil {
			return err
		}
	}
	return nil
}

// ListFlows outputs the list of available flows.
func (a *App) ListFlows(cmd Command, opts FlowCmdOptions) error {
	flows := builtinComposedFlows()

	// Read ExtraFlows at run time — plugins install via
	// RegisterAppInitializer which fires during PersistentPreRun, after
	// NewFlowsCmd has already constructed the cobra command.
	extra := opts.ExtraFlows
	if extra == nil {
		extra = a.ExtraFlows
	}
	if extra != nil {
		flows = append(flows, extra()...)
	}

	out := output.FlowsListOutput{
		Flows: flows,
		Total: len(flows),
	}
	return output.Print(cmd, out)
}

// builtinComposedFlows returns the list of built-in composed flows
// (multi-tool pipelines with 2+ tool nodes). Single-tool operations are
// exposed as top-level tool commands instead.
func builtinComposedFlows() []output.FlowInfo {
	var composed []output.FlowInfo
	for _, def := range flowdef.BuiltInFlows() {
		toolCount := 0
		for _, n := range def.Nodes {
			if n.Type == flow.NodeTool {
				toolCount++
			}
		}
		if toolCount >= 2 {
			composed = append(composed, output.FlowInfo{
				Name:        def.ID,
				Description: def.Description,
			})
		}
	}
	return composed
}

func (a *App) RunSingleFile(ctx context.Context, cmd Command, flowName, inputPath string) error {
	// A check step in the chain needs somewhere to report, and the chain is
	// built below — so the collector is armed first and read at the run's own
	// output.
	defer a.beginFlowFindings()()

	// Resolve format with optional preset syntax (e.g., "okf_html:strict").
	fmtName := a.FormatFlag
	var mergedConfig map[string]any
	if fmtName != "" {
		var err error
		fmtName, mergedConfig, err = a.resolveFormatRef(fmtName)
		if err != nil {
			return err
		}
	}

	// Build tools.
	flowTools, cleanup, err := a.buildFlowTools(flowName, inputPath, cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	// Wrap IO-bound tools with ParallelBlockTool.
	parallelBlocks, _ := cmd.Flags().GetInt("parallel-blocks")
	if parallelBlocks == 0 {
		parallelBlocks = a.resolveParallelBlocks(flowName)
	}
	if parallelBlocks > 1 {
		for i, ft := range flowTools {
			flowTools[i] = tool.NewParallelBlockTool(ft, parallelBlocks)
		}
	}

	// Wrap with TracingTool if --trace is set.
	tracePath, _ := cmd.Flags().GetString("trace")
	var recorder *flow.TraceRecorder
	if tracePath != "" {
		recorder = flow.NewTraceRecorder()
		for i, t := range flowTools {
			nodeID := fmt.Sprintf("tool-%d", i)
			flowTools[i] = flow.NewTracingTool(t, nodeID, recorder)
		}
	}

	// Wrap with pipeline metrics (outermost wrapper).
	stepNames := make([]string, len(flowTools))
	for i, t := range flowTools {
		stepNames[i] = t.Name()
	}
	metrics := flow.NewPipelineMetrics(stepNames)
	flowTools = flow.WrapWithMetrics(flowTools, metrics)
	// Outside the metrics wrapper, so the span covers the whole run.
	flowTools = flow.WrapWithSpans(flowTools)

	// Start TTY progress ticker (200ms) if interactive.
	jsonOut, _ := cmd.Flags().GetBool("json")
	showStepProgress := !a.Quiet && !jsonOut && isatty.IsTerminal(os.Stderr.Fd())
	var stopProgress func()
	if showStepProgress {
		stopProgress = startStepProgress(os.Stderr, metrics)
	}

	// NDJSON progress (--progress jsonl): the flow-run event vocabulary on
	// stderr, so machine consumers get the same event stream the desktop
	// run sink receives.
	sink, progressReport := progressSink(cmd)
	emit := func(ev FlowRunEvent) {
		if sink != nil {
			ev.Flow = flowName
			ev.Locale = a.TargetLang
			sink(ev)
		}
	}
	runStart := time.Now()
	emit(FlowRunEvent{Type: FlowEventState, Message: "running"})
	emit(FlowRunEvent{Type: FlowEventProgress, FileIndex: 0, FileCount: 1, FilePath: inputPath})

	// In project mode, run against the project's block cache so SessionTools
	// cache per-block work as overlays — that is what lets a later run skip
	// already-done steps and makes the project's working state packable (AD-025
	// §5). Identical output either way; the store only adds the overlay cache.
	// In project mode also open the document cache (L1): the runner parses
	// each source once and replays it on a later run — the parse companion to
	// the block store's overlay cache. Both rebuild from the files. Identical
	// output either way.
	//
	// The block store is a view on the App's project store, so it is not closed
	// here — the App owns the handle for as long as it lives.
	var projStore blockstore.Store
	var runnerCache flow.PartCache
	var runnerCacheKey string
	if a.ProjectContext != nil {
		if s := a.openProjectBlockStore(ctx); s != nil {
			projStore = s
		}
		closeCache := a.openParseCacheDefer(a.ProjectContext.ProjectDir)
		defer closeCache()
		runnerCache, runnerCacheKey = a.runnerPartCache(a.ProjectContext.ProjectDir, mergedConfig)
	}

	// Process-only default in a project (AD-026 §3/§5): when a .kapi recipe is
	// in scope AND the user did not pass an explicit -o, the run commits its
	// `targets/<locale>` overlays to the project store and emits no file.
	// Materializing the localized files is then a separate `kapi merge`. An
	// explicit -o (or no project) keeps the file-writing path below.
	processOnly := a.ProjectContext != nil && projStore != nil && !cmd.Flags().Changed("output") && !a.convergeWriteFiles

	// The recipe's word on this file: the project's format defaults overlaid by
	// the claiming item's own format.config, which is where the extraction rules
	// that separate content from identifiers are declared.
	item := a.projectItemFor(inputPath)

	// Build reader configuration callback: applies preset config + the recipe's
	// configuration for this item.
	// The runner builds the reader, and this callback is where it passes
	// through, so it is also the only place to catch the instance whose
	// diagnostics say what the run could not translate.
	var readReader format.DataFormatReader
	defer func() { a.reportReaderDiagnostics(cmd, inputPath, readReader) }()

	configureReader := func(reader format.DataFormatReader, detectedFmt registry.FormatID) error {
		readReader = reader
		if err := applyFormatConfig(reader, mergedConfig); err != nil {
			return fmt.Errorf("apply format config: %w", err)
		}
		if a.ProjectContext != nil {
			if err := a.ProjectContext.ConfigureReaderFor(reader, string(detectedFmt), item); err != nil {
				return fmt.Errorf("apply project format config: %w", err)
			}
		}
		return nil
	}

	// Build writer configuration callback: applies the shared output options
	// (output.bom / output.newline / output.encoding) plus the format's own
	// serialization keys, from preset config + the recipe's configuration for
	// this item.
	configureWriter := func(writer format.DataFormatWriter, fmtName registry.FormatID) error {
		if err := applyWriterOutputConfig(writer, mergedConfig); err != nil {
			return fmt.Errorf("apply writer output config: %w", err)
		}
		if a.ProjectContext != nil {
			if err := a.ProjectContext.ConfigureWriterFor(writer, string(fmtName), item); err != nil {
				return fmt.Errorf("apply project writer config: %w", err)
			}
		}
		return nil
	}

	// Project root so a process-only run keys its target overlays uniquely per
	// source file (blockstore.StoreKey) rather than by the collision-prone
	// file-local block id. Empty for ad-hoc (non-project) runs.
	projRoot := ""
	if a.ProjectContext != nil {
		projRoot = a.ProjectContext.ProjectDir
	}
	// The format the run reads this file with: the --format flag, else the one
	// the recipe's matching item declares, else registry detection. Declared
	// beats detected because every other derivation over this project — the
	// block store, coverage, checks, extract — resolves it that way, and a run
	// that detects differently numbers the document's blocks differently from
	// the store it writes into.
	declaredFormat := fmtName
	if declaredFormat == "" {
		declaredFormat = itemFormatName(item)
	}
	var detectFormat func(string) registry.FormatID
	if declaredFormat != "" {
		detectFormat = func(string) registry.FormatID { return registry.FormatID(declaredFormat) }
	}

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:       a.FormatReg,
		SourceLocale:    model.LocaleID(a.SourceLocale()),
		Encoding:        a.InputEncoding(),
		Recorder:        recorder,
		Store:           projStore,
		ProjectRoot:     projRoot,
		DetectFormat:    detectFormat,
		ConfigureReader: configureReader,
		ConfigureWriter: configureWriter,
		PartCache:       runnerCache,
		PartCacheKey:    runnerCacheKey,
	})

	if processOnly {
		if err := runner.RunFileProcessOnly(ctx, flowName, flowTools, inputPath, a.TargetLang); err != nil {
			if stopProgress != nil {
				stopProgress()
			}
			return err
		}
		if stopProgress != nil {
			stopProgress()
		}
		emit(FlowRunEvent{Type: FlowEventFileDone, FilePath: inputPath})
		emit(FlowRunEvent{
			Type:       FlowEventComplete,
			DurationMs: time.Since(runStart).Milliseconds(), FilesProcessed: 1,
		})
		if !a.Quiet {
			return a.printFlowRun(cmd, output.FlowRunOutput{
				FlowName:    flowName,
				InputPath:   inputPath,
				ProcessOnly: true,
			})
		}
		return nil
	}

	// Resolve output path through the unified resolver: a matched project
	// content-item target (via core's ResolveTargetPath) when a .kapi project is
	// in scope and no explicit -o was given, otherwise the ad-hoc template or
	// the <basename>_<lang><ext> default.
	outputFlag, _ := cmd.Flags().GetString("output")
	outputPath, err := a.resolveOutputPath(inputPath, outputFlag)
	if err != nil {
		if stopProgress != nil {
			stopProgress()
		}
		return err
	}

	// Whole-image (target-asset) replacement: when the source is a binary asset
	// and a localized variant already exists at the target, it is authoritative —
	// kapi can't regenerate a real image localization, so keep it rather than
	// clobber it by reprocessing the source.
	srcFmt := fmtName
	if srcFmt == "" {
		if det, derr := a.FormatReg.Detect(inputPath, registry.DetectOptions{ExtensionOnly: true}); derr == nil {
			srcFmt = string(det)
		}
	}
	if preserveAssetVariant(srcFmt, inputPath, outputPath) {
		if stopProgress != nil {
			stopProgress()
		}
		if !a.Quiet {
			out := output.FlowRunOutput{FlowName: flowName, InputPath: inputPath, OutputPath: outputPath}
			if err := a.printFlowRun(cmd, out); err != nil {
				return err
			}
		}
		return progressReport()
	}

	if err := runner.RunFile(ctx, flowName, flowTools, inputPath, outputPath, a.TargetLang); err != nil {
		if stopProgress != nil {
			stopProgress()
		}
		return err
	}
	if stopProgress != nil {
		stopProgress()
	}
	emit(FlowRunEvent{Type: FlowEventFileDone, FilePath: inputPath, OutputPath: outputPath})
	emit(FlowRunEvent{
		Type:       FlowEventComplete,
		DurationMs: time.Since(runStart).Milliseconds(), FilesProcessed: 1,
	})

	// Write trace JSON if --trace was set.
	if tracePath != "" && recorder != nil {
		detectedFmt := fmtName
		if detectedFmt == "" {
			detected, _ := a.FormatReg.Detect(inputPath, registry.DetectOptions{ExtensionOnly: true})
			detectedFmt = string(detected)
		}
		if err := a.writeTraceFile(tracePath, flowName, detectedFmt, inputPath, outputPath, recorder, stepNames); err != nil {
			return err
		}
	}

	if !a.Quiet {
		out := output.FlowRunOutput{
			FlowName:   flowName,
			InputPath:  inputPath,
			OutputPath: outputPath,
		}
		if err := a.printFlowRun(cmd, out); err != nil {
			return err
		}
	}
	// A truncated progress feed is reported after the result, so the deliverable
	// still lands and the consumer still learns its feed was incomplete.
	return progressReport()
}

// writeTraceFile serializes a trace to JSON and writes it to disk. toolNames is
// the ordered list of tool names (one per "tool-N" node) used to label the
// graph nodes — without it the nodes would fall back to their bare "tool-N" ids.
//
// Every failure is RETURNED: `--trace` is an explicit request for a file, so a
// run that writes none must not exit 0 (the user would open a visualizer on
// nothing). The batch path (runMultipleFiles) has always propagated these three;
// the single-file path discarding them was the asymmetry.
func (a *App) writeTraceFile(tracePath, flowName, fmtName, inputPath, outputPath string, recorder *flow.TraceRecorder, toolNames []string) error {
	inputContent, _ := os.ReadFile(inputPath)
	inputPreview := string(inputContent)
	if len(inputPreview) > 2000 {
		inputPreview = inputPreview[:2000] + "\n... (truncated)"
	}
	outputData, _ := os.ReadFile(outputPath)
	outputPreview := string(outputData)
	if len(outputPreview) > 2000 {
		outputPreview = outputPreview[:2000] + "\n... (truncated)"
	}

	traceNodes := []flow.TraceNode{
		{ID: "reader", Type: flow.NodeReader, Name: fmtName, Label: fmtName + " reader"},
	}
	for i, name := range toolNames {
		traceNodes = append(traceNodes, flow.TraceNode{
			ID: fmt.Sprintf("tool-%d", i), Type: flow.NodeTool, Name: name, Label: name,
		})
	}
	traceNodes = append(traceNodes, flow.TraceNode{
		ID: "writer", Type: flow.NodeWriter, Name: fmtName, Label: fmtName + " writer",
	})

	trace := &flow.FlowTrace{
		Name:        flowName,
		Description: fmt.Sprintf("%s flow on %s", flowName, filepath.Base(inputPath)),
		Nodes:       traceNodes,
		ChannelSize: 64,
		Events:      recorder.Events(),
		Parts:       recorder.Snapshots(),
		InputFile:   flow.TraceFile{Name: filepath.Base(inputPath), Format: fmtName, Preview: inputPreview},
		OutputFile:  flow.TraceFile{Name: filepath.Base(outputPath), Preview: outputPreview},
		DurationUs:  recorder.DurationUs(),
	}

	traceJSON, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trace: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(tracePath), 0o755); err != nil {
		return fmt.Errorf("create trace dir %s: %w", filepath.Dir(tracePath), err)
	}
	if err := os.WriteFile(tracePath, traceJSON, 0o644); err != nil {
		return fmt.Errorf("write trace %s: %w", tracePath, err)
	}
	return nil
}

func (a *App) runMultipleFiles(ctx context.Context, cmd Command, flowName string, inputPaths []string, concurrency int, outputTemplate string) error {
	// One collector for the batch: the report covers the files this run's own
	// output line covers.
	defer a.beginFlowFindings()()

	// Mirror a directory-style -o against the batch's common input root (not
	// each file's own dir), so nested inputs keep their relative structure —
	// the same base the exec tool runner uses.
	outputBase := commonDirPrefix(inputPaths)
	if concurrency <= 0 {
		if a.ProjectContext != nil && a.ProjectContext.Concurrency > 0 {
			concurrency = a.ProjectContext.Concurrency
		} else {
			concurrency = runtime.NumCPU()
		}
	}

	// Check if batch tracing is enabled.
	tracePath, _ := cmd.Flags().GetString("trace")
	var batchStart time.Time
	var lanes chan int
	type fileTraceInfo struct {
		file     string
		format   string
		recorder *flow.TraceRecorder
		nodes    []flow.TraceNode
		startUs  int64
		endUs    int64
		lane     int
	}
	var traceInfos []*fileTraceInfo
	// Files passed over rather than processed. Reported once after the run: a
	// silent skip is how a batch comes to look complete while a format nobody
	// registered went by untouched.
	var skipped []string

	if tracePath != "" {
		batchStart = time.Now()
		// Lane allocator: goroutines acquire/release lane IDs.
		lanes = make(chan int, concurrency)
		for i := range concurrency {
			lanes <- i
		}
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	// Global byte admission: SetLimit caps the number of concurrent files,
	// but the memory envelope is concurrency × per-file peak. The shared
	// Admission additionally caps total in-flight bytes, so a batch of huge
	// files serializes while small files still run wide. A file larger than
	// the whole budget acquires the full budget and runs alone (its
	// per-document safeio budget still bounds the read).
	admission := newFanoutAdmission()

	var mu sync.Mutex
	var processed int

	// NDJSON progress (--progress jsonl); the sink is mutex-guarded, so the
	// concurrent per-file goroutines may emit directly.
	sink, progressReport := progressSink(cmd)
	emit := func(ev FlowRunEvent) {
		if sink != nil {
			ev.Flow = flowName
			ev.Locale = a.TargetLang
			sink(ev)
		}
	}
	runStart := time.Now()
	emit(FlowRunEvent{Type: FlowEventState, Message: "running"})

	for fileIndex, inputPath := range inputPaths {
		g.Go(func() error {
			releaseAdmission, admErr := admission.Acquire(ctx, safeio.FileWeight(inputPath))
			if admErr != nil {
				return admErr
			}
			defer releaseAdmission()

			emit(FlowRunEvent{
				Type: FlowEventProgress, FileIndex: fileIndex, FileCount: len(inputPaths), FilePath: inputPath,
			})

			var recorder *flow.TraceRecorder
			var info *fileTraceInfo
			var lane int

			if tracePath != "" {
				lane = <-lanes
				recorder = flow.NewTraceRecorderWithStart(batchStart)
				info = &fileTraceInfo{
					file:     filepath.Base(inputPath),
					recorder: recorder,
					startUs:  time.Since(batchStart).Microseconds(),
					lane:     lane,
				}
			}

			fmtName, nodes, err := a.processFlowFile(ctx, cmd, flowName, inputPath, outputTemplate, outputBase, recorder)

			if tracePath != "" {
				info.endUs = time.Since(batchStart).Microseconds()
				info.format = fmtName
				info.nodes = nodes
				mu.Lock()
				traceInfos = append(traceInfos, info)
				mu.Unlock()
				lanes <- lane
			}

			if errors.Is(err, errSkippedFile) {
				mu.Lock()
				skipped = append(skipped, inputPath)
				mu.Unlock()
				return nil
			}
			if err != nil {
				return fmt.Errorf("%s: %w", inputPath, err)
			}
			mu.Lock()
			processed++
			mu.Unlock()
			emit(FlowRunEvent{Type: FlowEventFileDone, FilePath: inputPath})
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("flow execution error: %w", err)
	}

	// Write batch trace JSON if --trace was set.
	if tracePath != "" && len(traceInfos) > 0 {
		batchTrace := &flow.BatchFlowTrace{
			Name:        flowName,
			Concurrency: concurrency,
			DurationUs:  time.Since(batchStart).Microseconds(),
		}
		for _, info := range traceInfos {
			ft := flow.FileFlowTrace{
				File:       info.file,
				Format:     info.format,
				StartUs:    info.startUs,
				EndUs:      info.endUs,
				Lane:       info.lane,
				Nodes:      info.nodes,
				Events:     info.recorder.Events(),
				DurationUs: info.endUs - info.startUs,
			}
			batchTrace.FileTraces = append(batchTrace.FileTraces, ft)
		}

		traceJSON, err := json.MarshalIndent(batchTrace, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal batch trace: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(tracePath), 0o755); err != nil {
			return fmt.Errorf("create trace dir: %w", err)
		}
		if err := os.WriteFile(tracePath, traceJSON, 0o644); err != nil {
			return fmt.Errorf("write batch trace: %w", err)
		}
	}

	emit(FlowRunEvent{
		Type:       FlowEventComplete,
		DurationMs: time.Since(runStart).Milliseconds(), FilesProcessed: processed,
	})

	if !a.Quiet {
		if err := a.printFlowRun(cmd, output.FlowRunOutput{
			FlowName:       flowName,
			FilesProcessed: processed,
		}); err != nil {
			return err
		}
	}
	if len(skipped) > 0 {
		warnf(nil, "Warning: skipped %d file(s) with no registered format: %s\n",
			len(skipped), strings.Join(shortList(skipped, 3), ", "))
	}
	// A truncated progress feed is reported after the result, so the deliverable
	// still lands and the consumer still learns its feed was incomplete.
	return progressReport()
}

// failOnUnknown reads the strictness flag, under either of its two spellings.
// An absent flag means the default, which is to skip.
func failOnUnknown(cmd Command) bool {
	if v, err := cmd.Flags().GetBool("fail-on-unknown"); err == nil && v {
		return true
	}
	v, err := cmd.Flags().GetBool("strict")
	return err == nil && v
}

// wrapSkip marks a file as passed over, naming it and why. Both halves matter:
// the batch runner matches the sentinel with errors.Is, and a warning that says
// only "skipped 1 file" leaves a reader unable to tell an unreadable format
// from a file that was never there.
func wrapSkip(path string, cause error) error {
	return fmt.Errorf("%w: %s: %w", errSkippedFile, path, cause)
}

// shortList keeps a warning readable when a batch skips many files.
func shortList(xs []string, n int) []string {
	if len(xs) <= n {
		return xs
	}
	return append(append([]string{}, xs[:n]...), fmt.Sprintf("and %d more", len(xs)-n))
}

// errSkippedFile reports that a file was passed over rather than processed, and
// that passing it over was the requested behaviour.
//
// It exists because --fail-on-unknown was honoured on one code path and ignored
// on the other. The flag documents itself as "exit with error if any file
// cannot be processed (default: skip with warning)", and host/toolrun.go does
// exactly that; this path hard-errored regardless. The batch runner is an
// errgroup, so one unreadable file cancelled every sibling: a directory holding
// a single .h file among 843 readable ones produced no output at all and one
// error message. The engine benchmark ran into it and went three months without
// a refresh.
var errSkippedFile = errors.New("skipped")

// processFlowFile performs the full read → process → write cycle for a single file.
// Safe for concurrent use — each call uses its own reader, writer, and tool instances.
// When recorder is non-nil, tools are wrapped with TracingTool for batch tracing.
// Returns the detected format name and trace nodes (both empty when recorder is nil).
//
// All formats — built-in and Mode-C plugin-backed — use the standard
// read → process → write pipeline. Plugin-backed formats are routed
// through their daemon transparently by the registered factories.
func (a *App) processFlowFile(ctx context.Context, cmd Command, flowName, inputPath, outputTemplate, outputBase string, recorder *flow.TraceRecorder) (string, []flow.TraceNode, error) {
	// The recipe's word on this file: the item whose glob claims it names the
	// format to read it with and carries the format.config that says which of
	// its leaves are content. Both are needed here, and the item is resolved
	// once for both.
	item := a.projectItemFor(inputPath)

	fmtName := a.FormatFlag
	if fmtName == "" {
		fmtName = itemFormatName(item)
	}
	if fmtName == "" {
		// Use project-scoped detection when running in project mode.
		if a.ProjectContext != nil {
			fmtName = a.ProjectContext.DetectFormat(a.FormatReg, inputPath)
		}
		if fmtName == "" {
			// Content-aware: disambiguates extensions claimed by several formats
			// (e.g. .xliff 1.x vs 2.x) by the file head, not extension alone.
			detected, err := a.FormatReg.Detect(inputPath, registry.DetectOptions{})
			if err != nil {
				if !failOnUnknown(cmd) {
					return "", nil, wrapSkip(inputPath, err)
				}
				return "", nil, fmt.Errorf("unable to detect format: %w", err)
			}
			fmtName = string(detected)
		}
	}

	registryName, mergedConfig, err := a.resolveFormatRef(fmtName)
	if err != nil {
		return "", nil, err
	}

	reader, err := a.FormatReg.NewReader(registry.FormatID(registryName))
	if err != nil {
		return "", nil, fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}

	if err := applyFormatConfig(reader, mergedConfig); err != nil {
		return "", nil, fmt.Errorf("apply format config: %w", err)
	}

	// The project's format defaults overlaid by the claiming item's own
	// format.config. Both halves, or the run reads the document under a
	// configuration the recipe never described and extracts identifiers and
	// slugs as prose.
	if a.ProjectContext != nil {
		if err := a.ProjectContext.ConfigureReaderFor(reader, fmtName, item); err != nil {
			return "", nil, fmt.Errorf("apply project format config: %w", err)
		}
	}

	// All formats use the standard read → process → write pipeline.
	// Plugin-backed formats are routed through their Mode-C daemon
	// transparently by the registered factories.
	nodes, err := a.processFlowFileNative(ctx, cmd, flowName, inputPath, outputTemplate, outputBase, registryName, reader, mergedConfig, item, recorder)
	return fmtName, nodes, err
}

// processFlowFileNative uses the standard read → process → write pipeline.
// When recorder is non-nil, tools are wrapped with TracingTool and reader/writer
// events are recorded. Returns trace nodes (nil when recorder is nil).
func (a *App) processFlowFileNative(ctx context.Context, cmd Command, flowName, inputPath, outputTemplate, outputBase, registryName string, reader format.DataFormatReader, mergedConfig map[string]any, item *project.ContentItem, recorder *flow.TraceRecorder) ([]flow.TraceNode, error) {
	// A project run hands back the pass's assembled chain — ONE slice, shared by
	// every file in the batch, which runs its files concurrently. So the wrapping
	// below builds this file's own slice rather than writing into that one: an
	// in-place wrap is a write to shared memory from each goroutine, and the
	// wrapper another goroutine happens to be reading is not the one it created.
	shared, cleanup, err := a.buildFlowTools(flowName, inputPath, cmd)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	flowTools := make([]tool.Tool, len(shared))
	copy(flowTools, shared)

	// Auto-wrap IO-bound tools with ParallelBlockTool.
	if n := a.resolveParallelBlocks(flowName); n > 1 {
		for i, ft := range flowTools {
			flowTools[i] = tool.NewParallelBlockTool(ft, n)
		}
	}

	// In project mode this file's run belongs to the same project block store the
	// single-file path uses, keyed by the same project root, so a batch run commits
	// the same `targets/<locale>` overlays and a later merge finds them. A run with
	// no store leaves the project holding files it cannot account for, and
	// materializing then writes the source back over them. Autocommit sessions make
	// the store safe for the concurrent per-file goroutines (host/workspace.go).
	var projStore blockstore.Store
	projRoot := ""
	if a.ProjectContext != nil {
		projStore = a.openProjectBlockStore(ctx)
		projRoot = a.ProjectContext.ProjectDir
	}

	// Process-only default in a project (AD-026 §3/§5) — the same decision
	// RunSingleFile makes, so a batch and a one-file run of the same flow agree:
	// with a recipe in scope and no explicit -o, the run commits its
	// `targets/<locale>` overlays and emits no file, and `kapi merge`
	// materializes afterwards. It is also what stops a run feeding itself. The
	// ad-hoc destination below is a SIBLING of the input, which the collection
	// glob that supplied that input re-tracks as source on the next run, so a
	// writing batch inside a project doubles its own content every time.
	processOnly := a.ProjectContext != nil && projStore != nil &&
		!cmd.Flags().Changed("output") && !a.convergeWriteFiles

	// Wrap tools with TracingTool if recorder is set.
	var traceNodes []flow.TraceNode
	if recorder != nil {
		traceNodes = append(traceNodes, flow.TraceNode{
			ID: "reader", Type: flow.NodeReader, Name: registryName, Label: registryName + " reader",
		})
		for i, t := range flowTools {
			nodeID := fmt.Sprintf("tool-%d", i)
			traceNodes = append(traceNodes, flow.TraceNode{
				ID: nodeID, Type: flow.NodeTool, Name: t.Name(), Label: t.Name(),
			})
			flowTools[i] = flow.NewTracingTool(t, nodeID, recorder)
		}
		if !processOnly {
			traceNodes = append(traceNodes, flow.TraceNode{
				ID: "writer", Type: flow.NodeWriter, Name: registryName, Label: registryName + " writer",
			})
		}
	}

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    a.FormatReg,
		SourceLocale: model.LocaleID(a.SourceLocale()),
		Encoding:     a.InputEncoding(),
		Store:        projStore,
		ProjectRoot:  projRoot,
	})

	if processOnly {
		if err := runner.RunFileToStore(ctx, flowName, flowTools, inputPath, a.TargetLang, reader); err != nil {
			return traceNodes, err
		}
		return traceNodes, nil
	}

	outputPath, err := a.resolveOutputPathFrom(inputPath, outputTemplate, outputBase)
	if err != nil {
		return traceNodes, err
	}

	// Writer format defaults to the reader's format but a different output
	// extension selects a different writer — see registry.WriterFormatFor,
	// shared with the bowrain pull so both venues project cross-format
	// targets identically.
	writerFormatName := string(a.FormatReg.WriterFormatFor(registry.FormatID(registryName), outputPath))
	// Reader is pre-created and pre-configured by processFlowFile.
	// Pass it via RunFileWithReaderWriter since format detection already happened.
	writer, err := a.FormatReg.NewWriter(registry.FormatID(writerFormatName))
	if err != nil {
		return traceNodes, fmt.Errorf("no writer for %q: %w", writerFormatName, err)
	}

	// Apply the shared output options (preset config + project defaults).
	if err := applyWriterOutputConfig(writer, mergedConfig); err != nil {
		return traceNodes, fmt.Errorf("apply writer output config: %w", err)
	}
	if a.ProjectContext != nil {
		if err := a.ProjectContext.ConfigureWriterFor(writer, writerFormatName, item); err != nil {
			return traceNodes, fmt.Errorf("apply project writer config: %w", err)
		}
	}

	if err := runner.RunFileWithReaderWriter(ctx, flowName, flowTools, inputPath, outputPath, a.TargetLang, reader, writer); err != nil {
		return traceNodes, err
	}

	// The reader may have skipped content it could not handle faithfully. That
	// is the right call and a silent one: the run succeeds, the file is
	// written, and a region of it is simply still in the source language. Say
	// so here, on the ordinary run, rather than only under a validation flag
	// nobody passes.
	a.reportReaderDiagnostics(cmd, inputPath, reader)

	return traceNodes, nil
}

// reportReaderDiagnostics prints what the reader could not translate. Discovery
// is by assertion, so a reader that records nothing costs nothing.
func (a *App) reportReaderDiagnostics(cmd Command, inputPath string, reader format.DataFormatReader) {
	if a.Quiet || cmd == nil || reader == nil {
		return
	}
	dr, ok := reader.(format.DiagnosticReader)
	if !ok {
		return
	}
	diags := dr.Diagnostics()
	if len(diags) == 0 {
		return
	}
	w := cmd.ErrOrStderr()
	for _, d := range diags {
		where := filepath.Base(inputPath)
		if d.Line > 0 {
			where = fmt.Sprintf("%s:%d:%d", where, d.Line, d.Column)
		}
		fmt.Fprintf(w, "%s: %s: %s\n", where, d.Category, d.Message)
	}
}

// resolveOutputPath computes the output file path for one input file in a flow
// run. Resolution, in precedence order:
//
//  1. No explicit -o (outputTemplate == "") and a .kapi project is in scope
//     whose matched content item carries a target template → the per-file output
//     comes from core's project.ResolveTargetPath (the single canonical
//     resolver, shared with merge and the desktop). This honours {lang}, the
//     full path-token set, the legacy bare `*`, and directory targets — and so
//     fixes the double-extension class of bug.
//  2. No explicit -o and no project target → the ad-hoc default
//     <basename>_<lang><ext> beside the source, unless that sibling would land
//     inside a collection the project already tracks as source (see
//     collectionTracking), in which case there is no safe destination and the
//     run is refused.
//  3. An explicit -o template/path → the shared ad-hoc token vocabulary
//     (project.ResolvePathPattern + project.ExpandTemplate); a template ending
//     in a separator or naming a directory mirrors the input beneath it. An
//     explicit -o always wins over the project target (user override).
func (a *App) resolveOutputPath(inputPath, outputTemplate string) (string, error) {
	return a.resolveOutputPathFrom(inputPath, outputTemplate, filepath.Dir(inputPath))
}

// resolveOutputPathFrom is resolveOutputPath with an explicit mirror base: a
// directory-style -o mirrors the input beneath it relative to base. Batch runs
// pass the files' common root so nested inputs keep their structure; single
// runs pass the file's own dir (via resolveOutputPath).
func (a *App) resolveOutputPathFrom(inputPath, outputTemplate, base string) (string, error) {
	if outputTemplate == "" {
		if out, ok := a.projectItemTargetPath(inputPath, a.TargetLang); ok {
			// A convergence pass under a gating policy drafts into the run's own
			// tree; delivery to this destination happens once, at the end, for
			// the locales that cleared their gate (host/convergedrafts.go).
			out, _ = a.draftPathFor(a.TargetLang, out)
			ensureParentDir(out)
			return out, nil
		}
		if a.TargetLang == "" {
			return "", fmt.Errorf("%s: no output destination — give the collection a target: template, or pass -o",
				filepath.Base(inputPath))
		}
		ext := format.Ext(inputPath)
		name := filepath.Base(inputPath[:len(inputPath)-len(ext)])
		out := filepath.Join(filepath.Dir(inputPath), fmt.Sprintf("%s_%s%s", name, a.TargetLang, ext))
		if pattern, tracked := a.collectionTracking(out); tracked {
			return "", fmt.Errorf("%s: writing %s beside its source would land inside the collection %q, which tracks it as source on the next run — give the collection a target: template, or pass -o",
				filepath.Base(inputPath), filepath.Base(out), pattern)
		}
		return out, nil
	}

	if base == "" {
		base = filepath.Dir(inputPath)
	}
	out := expandAdhocOutputTemplate(outputTemplate, inputPath, base, a.TargetLang)
	ensureParentDir(out)
	return out, nil
}

// collectionTracking reports the first collection pattern in the active project
// that matches outPath, and so would pick it up as SOURCE on the next run.
//
// A destination inside the collection that supplied the input is self-feeding
// by construction: whatever the run writes, the glob reads back, and the
// project doubles on every pass. Callers refuse rather than write there.
// Returns ("", false) outside a project, or for a path the project does not
// track.
func (a *App) collectionTracking(outPath string) (string, bool) {
	if a.ProjectContext == nil || a.ProjectContext.Project == nil {
		return "", false
	}
	rel, ok := projectRelPath(a.ProjectContext.ProjectDir, outPath)
	if !ok {
		return "", false
	}
	for _, coll := range a.ProjectContext.Project.Collections {
		for _, item := range coll.EffectiveItems() {
			if item.Path == "" {
				continue
			}
			if project.MatchGlob(item.Path, rel) {
				return item.Path, true
			}
		}
	}
	return "", false
}

// itemFormatName is the format a content item declares, or "" when it declares
// none (or declares `$auto`, the explicit ask for detection). It is the same
// "explicit > auto-detected" precedence ProjectContext.ResolveContent applies,
// which is what keeps a run's reader the reader every other derivation used: a
// path that detects instead splits the document into a different set of blocks,
// and the file-local block ids that address the store then name different
// content on the two sides of one convergence.
func itemFormatName(item *project.ContentItem) string {
	if item == nil || item.Format == nil {
		return ""
	}
	return project.ResolveFormat(item.Format.Name)
}

// projectItemFor returns the recipe content item whose glob claims inputPath —
// the item whose `format.config` says which leaves of that file are content and
// how the writer serializes them. The walk is the recipe's own first-match
// order, the same one target resolution uses, so a file cannot be read under one
// item's rules and written under another's. nil when no recipe is in scope, the
// input lies outside the project root, or no item claims it.
//
// An item with no `target:` still governs its own reading, so — unlike
// projectItemTargetPath — a missing target is not a reason to skip it.
func (a *App) projectItemFor(inputPath string) *project.ContentItem {
	if a.ProjectContext == nil || a.ProjectContext.Project == nil {
		return nil
	}
	relSlash, ok := projectRelPath(a.ProjectContext.ProjectDir, inputPath)
	if !ok {
		return nil
	}
	for _, coll := range a.ProjectContext.Project.Collections {
		for _, item := range coll.EffectiveItems() {
			if item.Path == "" || !project.MatchGlob(item.Path, relSlash) {
				continue
			}
			matched := item
			return &matched
		}
	}
	return nil
}

// projectItemTargetPath resolves inputPath to its output path via the matched
// project content item's target template, using the one core resolver
// (project.ResolveTargetPath). Mirrors the desktop runner's resolution. Returns
// ("", false) when no kapi project is in scope, the input is outside the
// project root, or no content item with a target matches it.
func (a *App) projectItemTargetPath(inputPath, lang string) (string, bool) {
	if a.ProjectContext == nil || a.ProjectContext.Project == nil {
		return "", false
	}
	root := a.ProjectContext.ProjectDir
	relSlash, ok := projectRelPath(root, inputPath)
	if !ok {
		return "", false
	}
	for _, coll := range a.ProjectContext.Project.Collections {
		for _, item := range coll.EffectiveItems() {
			if item.Path == "" || item.Target == "" {
				continue
			}
			if !project.MatchGlob(item.Path, relSlash) {
				continue
			}
			out := project.ResolveTargetPath(item.Path, item.Base, item.Target, relSlash, lang)
			if !filepath.IsAbs(out) {
				out = filepath.Join(root, out)
			}
			return out, true
		}
	}
	return "", false
}

// expandAdhocOutputTemplate expands an explicit -o template/path for one input
// file using the shared core path-token vocabulary: project.ResolvePathPattern
// for {lang} and project.ExpandTemplate for the path tokens ({name} {basename}
// {ext} {dir} {path} {relpath} {filename}), plus the legacy bare `*` == {name}.
// When the template (after {lang} expansion) denotes a directory — it ends in a
// separator, or its last segment has no extension and no wildcard/token — the
// input is mirrored beneath it (relative to base). This keeps the ad-hoc -o
// vocabulary identical to the project content-item target vocabulary.
func expandAdhocOutputTemplate(tmpl, inputPath, base, lang string) string {
	resolved := project.ResolvePathPattern(tmpl, lang)
	if isDirTemplate(resolved) {
		rel := filepath.Base(inputPath)
		if base != "" {
			if r, err := filepath.Rel(base, inputPath); err == nil && !strings.HasPrefix(filepath.ToSlash(r), "../") {
				rel = r
			}
		}
		return filepath.Join(resolved, rel)
	}
	out := project.ExpandTemplate(resolved, inputPath)
	name := format.Stem(inputPath)
	out = strings.ReplaceAll(out, "*", name)
	return filepath.FromSlash(out)
}

// isDirTemplate reports whether an output template (after {lang} expansion)
// denotes a directory to mirror into rather than a filename template. Mirrors
// core's isDirectoryTarget: true when it ends in a separator, is empty, or its
// final segment carries no file extension and no wildcard/template token.
func isDirTemplate(s string) bool {
	if s == "" || strings.HasSuffix(s, "/") || strings.HasSuffix(s, string(filepath.Separator)) {
		return true
	}
	last := s
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		last = s[i+1:]
	}
	if strings.ContainsAny(last, "*?[{") {
		return false
	}
	return format.Ext(last) == ""
}

// ensureParentDir best-effort creates the parent directory of p so a template
// targeting a new directory writes cleanly.
func ensureParentDir(p string) {
	if dir := filepath.Dir(p); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
}

// expandOutputTemplate replaces {name}, {lang}, {ext}, and {dir} placeholders
// in a path template. ext should be without the leading dot. Retained for the
// .kpz workspace merge path (kpzworkspace.go); flow/tool runs now route their
// explicit-template expansion through expandAdhocOutputTemplate.
func expandOutputTemplate(tmpl, name, lang, ext, dir string) string {
	r := strings.NewReplacer(
		"{name}", name,
		"{lang}", lang,
		"{ext}", ext,
		"{dir}", dir,
	)
	return r.Replace(tmpl)
}

// buildFlowTools creates the tool chain for the given flow. The returned cleanup
// function releases any resources opened during tool creation (e.g. SQLite content memory).
// Callers must defer cleanup() after checking err.
//
// inputPath is the file the chain will process. A built-in flow builds fresh
// tools per file, so the path is what tells the standing bindings which content
// collection governs this run — the voice profile and terms of the collection the
// file belongs to, not necessarily the project-wide ones. Empty when there is no
// single file in view; the bindings are then the project-wide ones.
func (a *App) buildFlowTools(flowName, inputPath string, cmd ...Command) ([]tool.Tool, func(), error) {
	noop := func() {}

	// If project flow tools are set (via runProjectSteps), use them directly.
	if a.projectFlowTools != nil {
		return a.tapFindings(a.projectFlowTools, inputPath), noop, nil
	}

	// Look up the flow definition from the built-in registry.
	var flowDef *flow.FlowDefinition
	for _, def := range flowdef.BuiltInFlows() {
		if def.ID == flowName {
			d := def
			flowDef = &d
			break
		}
	}
	if flowDef == nil {
		return nil, nil, fmt.Errorf("unknown flow: %q", flowName)
	}

	// Hard data-flow validation (tool/data-model redesign, phase 4): reject a
	// flow whose tool requires a port that no upstream tool or the source
	// binding produces.
	if err := flowDef.ValidateDataFlow(a.ToolReg); err != nil {
		return nil, nil, err
	}
	// Transformer placement gate (AD-006): errors (a transformer after a
	// target producer, a redactor after remote egress) reject the flow;
	// warnings (avoidable overlay rebasing) are surfaced but don't block.
	if err := a.checkFlowPlacement(flowDef); err != nil {
		return nil, nil, err
	}

	// Extract tool node names in topological order (by X position).
	type toolPos struct {
		name   string
		x      float64
		config map[string]any
	}
	var toolNodes []toolPos
	for _, n := range flowDef.Nodes {
		if n.Type == flow.NodeTool {
			toolNodes = append(toolNodes, toolPos{name: n.Name, x: n.Position.X, config: n.Config})
		}
	}
	slices.SortFunc(toolNodes, func(a, b toolPos) int {
		if a.x < b.x {
			return -1
		}
		if a.x > b.x {
			return 1
		}
		return 0
	})

	// Build the tool chain from tool definitions.
	var builtTools []tool.Tool
	cleanups := []func(){}
	cleanup := func() {
		for _, fn := range cleanups {
			fn()
		}
	}

	config := map[string]any{
		"source_locale": a.SourceLocale(),
		"target_locale": a.TargetLang,
	}

	// The standing context a built-in flow runs under. Project-defined flows get
	// this via toolFromStep; built-in flows used to get only the voice profile
	// profile, so `kapi translate` inside a project silently ignored
	// defaults.tools and never sent the project's terminology, while `kapi up` —
	// the same tools, the project path — honored both. One recipe, two behaviours,
	// depending on the verb.
	bindings := a.resolveRunBindings(inputPath, cmd...)

	// Inject credential/provider flags from the command into the tool config.
	if len(cmd) > 0 && cmd[0] != nil {
		if v, _ := cmd[0].Flags().GetString("credential"); v != "" {
			config["credential"] = v
		}
		// Only inject --provider when the user explicitly passed it; the flag
		// has a default of "anthropic" which must not shadow a named
		// credential's provider_type (fixes #637).
		if cmd[0].Flags().Changed("provider") {
			if v, _ := cmd[0].Flags().GetString("provider"); v != "" {
				config["provider"] = v
			}
		}
		if v, _ := cmd[0].Flags().GetString("api-key"); v != "" {
			config["apiKey"] = v
		}
		if v, _ := cmd[0].Flags().GetString("model"); v != "" {
			config["model"] = v
		}
		// --instruction steers the translation without replacing the prompt: it
		// is rendered into the prompt's Instruction section. Tools that don't
		// recognise the key ignore it.
		if v, _ := cmd[0].Flags().GetString("instruction"); v != "" {
			config["instruction"] = v
		}
		// --context decides what the model is told about a block besides the
		// block: its key, its neighbours. A bare "Save" is a coin flip between a
		// verb and a noun; its key settles it.
		if v, _ := cmd[0].Flags().GetString("context"); v != "" {
			config["context"] = v
		}
		// --batching is an intent (auto | single), not a block count: the right
		// count depends on the model's output ceiling and the length of these
		// particular segments, which is not something a user can be asked to know.
		if v, _ := cmd[0].Flags().GetString("batching"); v != "" {
			config["batching"] = v
		}
	}

	for _, tn := range toolNodes {
		// A graph/built-in node may carry per-tool config (e.g. redact's detectors
		// and entityTypes); overlay it on the shared run config for this node only.
		toolConfig := config
		if len(tn.config) > 0 {
			toolConfig = mergeFlowNodeConfig(config, tn.config)
		}
		toolConfig = a.applyBindings(bindings, tn.name, a.ToolReg.Schema(registry.ToolID(tn.name)), toolConfig)
		t, toolCleanup, err := a.buildToolByName(tn.name, toolConfig, cmd...)
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("tool %q in flow %q: %w", tn.name, flowName, err)
		}
		builtTools = append(builtTools, t...)
		if toolCleanup != nil {
			cleanups = append(cleanups, toolCleanup)
		}
	}

	return a.tapFindings(builtTools, inputPath), cleanup, nil
}

// BuildFlowTools assembles the tool chain for a built-in flow for one
// (source, target) pass. It runs the same data-flow validation, AD-006
// placement gate, per-node config merge and standing bindings as a CLI flow
// run — the reason an embedding surface (the MCP run_flow porcelain) must route
// through it rather than reimplement the loop. The returned cleanup releases
// resources the assembly opened (e.g. SQLite content memory handles) and must
// run after the pass.
func (a *App) BuildFlowTools(flowName, inputPath, src, tgt string, cmd ...Command) ([]tool.Tool, func(), error) {
	savedSrc, savedTgt := a.SourceLang, a.TargetLang
	a.SourceLang, a.TargetLang = src, tgt
	defer func() { a.SourceLang, a.TargetLang = savedSrc, savedTgt }()
	return a.buildFlowTools(flowName, inputPath, cmd...)
}

// checkFlowPlacement runs the AD-006 transformer placement pass over a flow
// definition: error-severity diagnostics reject the flow (the unconditional
// build gate beside ValidateDataFlow), warnings are printed to stderr.
func (a *App) checkFlowPlacement(def *flow.FlowDefinition) error {
	diags, err := def.ValidatePlacement(a.ToolReg)
	if err != nil {
		return err
	}
	for _, d := range diags {
		if d.Severity == flow.PlacementWarning {
			fmt.Fprintf(os.Stderr, "warning: flow %q: %s\n", def.Name, d.Message)
		}
	}
	if err := def.CheckPlacement(a.ToolReg); err != nil {
		return err
	}
	// A declared redaction policy binds every route to a remote sink, including
	// a flow the operator composes step by step: if the project declares
	// defaults.redaction, a flow that egresses source remotely must carry a
	// redact step.
	requireRedaction := a.ProjectContext != nil && ProjectRedaction(a.ProjectContext.Project) != nil
	return def.CheckRedactionCoverage(a.ToolReg, requireRedaction)
}

// mergeFlowNodeConfig overlays a flow node's per-tool config onto the shared run
// config, returning a new map (the node values win). The shared config is left
// untouched so sibling nodes don't see each other's overrides.
func mergeFlowNodeConfig(base, over map[string]any) map[string]any {
	m := make(map[string]any, len(base)+len(over))
	maps.Copy(m, base)
	maps.Copy(m, over)
	return m
}

// buildToolByName creates tool(s) for a named tool, returning any resource
// cleanup function. Uses ToolInfo.Requires to drive resource setup (terms,
// content memory) rather than hardcoding tool names.
func (a *App) buildToolByName(toolName string, config map[string]any, cmd ...Command) ([]tool.Tool, func(), error) {
	if a.ToolReg == nil || !a.ToolReg.Has(registry.ToolID(toolName)) {
		return nil, nil, fmt.Errorf("tool %q not found in registry", toolName)
	}

	// Same gate as toolFromStep: a flow-driven exec-class tool needs an
	// established trust decision (host/exectrust.go).
	if err := a.checkExecToolAllowed(toolName); err != nil {
		return nil, nil, err
	}

	info := a.ToolReg.ToolInfo(registry.ToolID(toolName))

	// Resource setup driven by Requires metadata.
	if info != nil {
		for _, req := range info.Requires {
			switch req {
			case schema.RequiresTerms:
				// Tools requiring a terms store get term lookup/enforce tools appended.
				t, err := a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), config, a.TargetLang)
				if err != nil {
					return nil, nil, err
				}
				qaTools := []tool.Tool{t}
				var cleanup func()
				if tb, tbCleanup, err := a.openTerms(cmd...); err != nil {
					return nil, nil, err
				} else if tb != nil {
					qaTools = append(qaTools,
						sqlterms.NewTermLookupTool(tb, sqlterms.TermLookupConfig{
							SourceLocale: model.LocaleID(a.SourceLocale()),
							TargetLocale: model.LocaleID(a.TargetLang),
						}),
						sqlterms.NewTermEnforceTool(tb, sqlterms.TermEnforceConfig{
							SourceLocale: model.LocaleID(a.SourceLocale()),
							TargetLocale: model.LocaleID(a.TargetLang),
						}),
					)
					cleanup = tbCleanup
				}
				return qaTools, cleanup, nil

			}
		}
	}

	// A content memory reaches a tool as a live handle in the config map, the
	// way a voice profile does: neither survives the JSON round trip the rest of
	// the config takes.
	//
	// One route for every tool that wants one, whether it REQUIRES a corpus
	// (recycle, which is a no-op without one) or merely ACCEPTS one (translate,
	// which reads a block's previously approved answer as prompt reference and
	// translates fine without it). Before this there were three, each narrowing
	// the built tool to recycle's concrete config — so the capability could only
	// ever serve one tool, and translate reached its corpus through a side
	// channel to avoid being routed into an assertion it would fail.
	config, corpusCleanup, err := a.grantMemory(toolName, config, cmd...)
	if err != nil {
		return nil, nil, err
	}

	// Default: create from registry.
	t, err := a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), config, a.TargetLang)
	if err != nil {
		corpusCleanup()
		return nil, nil, err
	}
	return []tool.Tool{t}, corpusCleanup, nil
}

// MemoryGrantFor opens a content memory once and returns a function that grants
// it to each of a tool's config maps.
//
// It exists for the CLI, which builds one tool per input file and must not open
// a store per file. The grant itself is grantMemory, so a corpus reaches a tool
// the same way whether it came from a flow step, a direct build, or a command
// run over twenty files. The returned cleanup is always safe to call.
func (a *App) MemoryGrantFor(toolName string, cmd Command) (func(map[string]any) map[string]any, func()) {
	noop := func() {}
	identity := func(c map[string]any) map[string]any { return c }

	// Asked once, with an empty config, purely to resolve the store. A failure
	// here is not fatal: a tool that merely accepts a corpus should still run,
	// and one that requires it is a no-op rather than an error without it.
	probe, cleanup, err := a.grantMemory(toolName, map[string]any{}, cmd)
	if err != nil || probe == nil {
		if cleanup != nil {
			cleanup()
		}
		return identity, noop
	}
	if cleanup == nil {
		cleanup = noop
	}

	granted := make(map[string]any, len(probe))
	maps.Copy(granted, probe)
	return func(config map[string]any) map[string]any {
		if len(granted) == 0 {
			return config
		}
		next := make(map[string]any, len(config)+len(granted))
		maps.Copy(next, config)
		// The resolved values fill gaps; anything the caller set explicitly
		// wins, which is the same precedence the flow path applies.
		for k, v := range granted {
			if _, ok := next[k]; !ok {
				next[k] = v
			}
		}
		return next
	}, cleanup
}

// grantMemory puts a content memory in a tool's config when the tool asked for
// one, and returns the config unchanged when it did not.
//
// Requires and Accepts are both honoured and mean different things. A tool that
// REQUIRES a corpus cannot do its job without one; a tool that ACCEPTS one does
// more with it. Neither is failed here for the absence of a store: a project
// with no content memory should still build every step, and recycle with no
// corpus is a no-op rather than an error. What would be wrong is silently
// leaving the handle out of a tool that asked, which is what three separate
// injection routes made easy.
//
// The returned cleanup is always safe to call.
func (a *App) grantMemory(toolName string, config map[string]any, cmd ...Command) (map[string]any, func(), error) {
	noop := func() {}
	s := a.ToolReg.Schema(registry.ToolID(toolName))
	if !ToolRequires(s, schema.RequiresMemory) && !ToolAccepts(s, schema.AcceptsMemory) {
		return config, noop, nil
	}
	if len(cmd) == 0 || cmd[0] == nil {
		return config, noop, nil
	}

	provider, cleanup, err := a.OpenToolMemory(cmd[0])
	if err != nil {
		return nil, noop, err
	}
	if provider == nil {
		if cleanup != nil {
			cleanup()
		}
		return config, noop, nil
	}
	if cleanup == nil {
		cleanup = noop
	}

	// Cloned rather than mutated: the recipe's in-memory step config is shared,
	// and a run must not leave a live store handle in it.
	next := make(map[string]any, len(config)+3)
	maps.Copy(next, config)
	next[corememory.ConfigKey] = provider

	// The source locale is schema-hidden, so a config factory cannot read it
	// from the map — and without it a SQLite lookup (WHERE locale = ?) matches
	// nothing. It was previously set on the built tool, which only worked for
	// the one tool the injection knew about.
	if _, ok := next[corememory.SourceLocaleKey]; !ok {
		next[corememory.SourceLocaleKey] = a.SourceLocale()
	}
	return next, cleanup, nil
}

// resolveParallelBlocks returns the parallel block concurrency to use.
// Prefers project context setting, then falls back to flow/tool defaults.
func (a *App) resolveParallelBlocks(flowName string) int {
	if a.ProjectContext != nil && a.ProjectContext.ParallelBlocks > 0 {
		return a.ProjectContext.ParallelBlocks
	}
	return a.defaultParallelBlocks(flowName)
}

// defaultParallelBlocks returns the default parallel block concurrency for a
// flow. Looks up each tool in the flow and returns the max DefaultParallelBlocks
// from the tool registry. Returns 0 (sequential) if no tool specifies it.
func (a *App) defaultParallelBlocks(flowName string) int {
	if a.ToolReg == nil {
		return 0
	}
	for _, def := range flowdef.BuiltInFlows() {
		if def.ID == flowName {
			maxPB := 0
			for _, n := range def.Nodes {
				if n.Type != "tool" {
					continue
				}
				if info := a.ToolReg.ToolInfo(registry.ToolID(n.Name)); info != nil && info.DefaultParallelBlocks > maxPB {
					maxPB = info.DefaultParallelBlocks
				}
			}
			return maxPB
		}
	}
	return 0
}

// openTerms opens the terms store the term tools in built-in flows enforce
// against: an explicit --termstore (a named resource or a file path), a
// profile's standalone `terms:`, else the project's own store — so those tools
// are project-aware with no flag.
//
// Returns (nil, noop, nil) when nothing is bound, and also when the project
// store holds NO concepts: the vocabulary tables exist from the store's first
// open, so their presence stopped meaning anything and emptiness is what "there
// is nothing to enforce" looks like now.
func (a *App) openTerms(cmd ...Command) (*sqlterms.SQLiteStore, func(), error) {
	noop := func() {}
	if len(cmd) == 0 || cmd[0] == nil {
		return nil, noop, nil
	}
	sel, err := a.ResolveTermsStore(cmd[0], project.GovernancePoint{})
	if err != nil {
		return nil, nil, err
	}
	if sel.InProject() {
		ctx := CmdContext(cmd[0])
		db, err := a.ProjectDB(ctx, sel.Root)
		if err != nil {
			return nil, nil, err
		}
		has, err := db.HasTerms(ctx)
		if err != nil || !has {
			return nil, noop, err
		}
		return db.Terms(), noop, nil
	}
	if sel.Path == "" {
		return nil, noop, nil
	}
	if !sel.Explicit {
		if _, statErr := os.Stat(sel.Path); statErr != nil {
			// A standalone store the recipe names but nothing has created yet.
			return nil, noop, nil
		}
	}
	tb, err := sqlterms.NewSQLiteStore(sel.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("open terms %q: %w", sel.Path, err)
	}
	return tb, func() { tb.Close() }, nil
}

// OpenToolMemory resolves the content memory a `memory`-requiring tool (e.g.
// recycle) should leverage and opens it as a MemoryProvider. The --memory flag
// wins: a named resource (no path separators) resolves via KAPI_HOME, an
// explicit file path is opened directly. When no flag is given but a project is
// in scope, the project's own store is used, so `kapi recycle fr.json` leverages
// the same content memory `kapi extract`/`kapi merge` use — with no flag.
//
// Returns (nil, noop, nil) when no content memory is in scope. This mirrors
// openTerms and shares the project resolution with the `kapi memory`
// subcommands (ResolveMemoryStore).
func (a *App) OpenToolMemory(cmd Command) (corememory.Provider, func(), error) {
	noop := func() {}
	if cmd == nil {
		return nil, noop, nil
	}
	memoryValue, _ := cmd.Flags().GetString("memory")

	// A pre-seeded in-memory backend (the wasm build, or any host that sets
	// a.MemoryBackend) is the authoritative content memory and the only one that works without
	// the SQLite driver — prefer it over any on-disk project path. The native
	// CLI never sets a.MemoryBackend, so this only takes effect in the browser/seed
	// case; the on-disk resolution below is unchanged for the native binary.
	// An explicit --memory path still wins (handled in the switch).
	if memoryValue == "" && a.MemoryBackend != nil {
		return leverage.NewProvider(a.MemoryBackend), noop, nil
	}

	var memoryPath string
	switch {
	case memoryValue == "":
		// No flag — the project's own store.
		root, err := a.projectRootFor(cmd)
		if err != nil {
			return nil, nil, err
		}
		if root == "" {
			return nil, noop, nil
		}
		db, err := a.ProjectDB(CmdContext(cmd), root)
		if err != nil {
			return nil, nil, err
		}
		tm := db.Memory()
		if tm == nil {
			return nil, noop, nil
		}
		return leverage.NewProvider(tm), noop, nil
	case strings.ContainsAny(memoryValue, "/\\") || strings.HasSuffix(memoryValue, ".db"):
		// Explicit file path.
		memoryPath = memoryValue
	default:
		// Named resource.
		var err error
		memoryPath, err = resolveNamedResource("memory", memoryValue)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve content memory %q: %w", memoryValue, err)
		}
	}

	tm, err := sqlmemory.NewSQLiteStore(memoryPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open content memory %q: %w", memoryPath, err)
	}
	return leverage.NewProvider(tm), func() { tm.Close() }, nil
}

// ProjectBindings holds the standing voice + terminology context resolved
// from a .kapi project, applied to project-flow steps that can honor them.
type ProjectBindings struct {
	// profile is the resolved voice profile (defaults.voice),
	// injected into translate steps as config["profile"]. nil when unbound.
	profile *coreprofile.VoiceProfile
	// termRules are the terminology constraints built from the project terms
	// store (defaults.terms_source), injected into the steps governed by
	// terminology as config["term_rules"]. nil when unbound.
	termRules []coreprofile.TermRule
	// ToolPresets holds the project-level tool presets (defaults.tools):
	// per-tool config defaults merged under each step's own config wherever
	// that tool runs in a project flow (the step wins per key). nil when the
	// recipe declares none.
	ToolPresets map[string]map[string]any
	// localePresets holds per-target-locale tool presets (defaults.locales.<lang>.tools).
	// For a run whose target locale has an entry, these merge on top of
	// ToolPresets (per-locale wins) and under the step's own config (the step
	// still wins). nil when the recipe declares none.
	localePresets map[string]project.LocaleDefaults
	// point is where this group's content sits in the context space, rendered
	// for the content memory (memory.NewPoint). It is injected into recycle as
	// config["point"], which is how a fill asks the corpus for the answer
	// approved nearest here rather than the one it repeats most.
	//
	// It is resolved to the group's channel, not to a file: a project flow bakes
	// its governance into the tool chain before any content is read, and the
	// chain is shared by every file in the group, so the channel is the finest
	// place a fill can honestly name itself at.
	point string
}

// resolveProjectBindings resolves the standing voice + terminology context
// for one point of a project flow run — the governance in force there. The
// voice comes from the profile matching that point, else defaults.voice, else a
// convention voice.yaml, with the point's channel selecting the override inside
// it; the term rules come from that profile's standalone terms, else the
// project's own store. Returns nil when the project carries neither, so ad-hoc
// behavior is unchanged.
//
// point is the zero value for a run that is not split by governance — every
// caller outside the grouping (groupInputsByBinding) passes it, which resolves
// exactly the project-wide bindings it always did.
func (a *App) resolveProjectBindings(cmd Command, proj *project.KapiProject, projectPath string, point project.GovernancePoint) (*ProjectBindings, error) {
	root := filepath.Dir(projectPath)

	store, release, err := a.VoiceLookupStore(cmd)
	if err != nil {
		return nil, err
	}
	defer release()
	profile, _, _, err := a.ResolveVoiceProfile(CmdContext(cmd), proj, root, VoiceResolveOptions{
		Store: store,
		Point: point,
	})
	if err != nil {
		return nil, err
	}

	termRules, err := a.ResolveTermRulesFor(cmd, a.TargetLang, point)
	if err != nil {
		return nil, err
	}

	// Resolved directly rather than through ResolveGovernanceAtPoint: this is a
	// second read of a resolution the run has already reported, and reporting a
	// closing governance window twice would read as two events.
	gov, err := proj.ResolveGovernanceFor(point)
	if err != nil {
		return nil, err
	}
	at := sqlmemory.NewPoint(gov.Profile, gov.Channel, "")

	if profile == nil && len(termRules) == 0 && at == "" &&
		len(proj.Defaults.Tools) == 0 && len(proj.Defaults.Locales) == 0 {
		return nil, nil
	}
	return &ProjectBindings{
		profile:       profile,
		termRules:     termRules,
		ToolPresets:   proj.Defaults.Tools,
		localePresets: proj.Defaults.Locales,
		point:         at,
	}, nil
}

// ToolRequires reports whether the tool schema declares the named requirement.
func ToolRequires(s *schema.ComponentSchema, req string) bool {
	if s == nil || s.ToolMeta == nil {
		return false
	}
	return slices.Contains(s.ToolMeta.Requires, req)
}

// ToolAccepts reports whether the tool schema declares the named optional
// capability: something the tool uses when the run has it and runs without.
func ToolAccepts(s *schema.ComponentSchema, cap string) bool {
	if s == nil || s.ToolMeta == nil {
		return false
	}
	return slices.Contains(s.ToolMeta.Accepts, cap)
}

// ResolveTermsStore returns the terms store a project-aware tool command should
// use, with no flag. Resolution order:
//
//  1. An explicit --termstore flag (named resource or path) — a standalone store.
//  2. The `terms:` of the profile governing the point
//     (project.ResolveGovernanceFor) — a standalone store, relative to the
//     project root. This is the one place a recipe still names a terms FILE: it
//     binds a vocabulary to a region of the context space, and there is nothing
//     on the wire for it, so it stays a local path.
//  3. That profile's conventional override, `.kapi/profiles/<name>/terms.json`,
//     when the file exists — the same binding without the line in the recipe.
//  4. The project's own store.
//
// The zero point resolves the project-wide binding, which is what every caller
// outside a partitioned flow run wants.
//
// Returns the zero selection (with nil error) when there is no project and no
// flag, so callers fall through to the tool's default (no vocabulary).
func (a *App) ResolveTermsStore(cmd Command, point project.GovernancePoint) (StoreSelection, error) {
	if cmd != nil {
		if tbValue, _ := cmd.Flags().GetString("termstore"); tbValue != "" {
			if strings.ContainsAny(tbValue, "/\\") || strings.HasSuffix(tbValue, ".db") {
				return StoreSelection{Path: tbValue, Explicit: true}, nil
			}
			path, err := resolveNamedResource("terms", tbValue)
			if err != nil {
				return StoreSelection{}, fmt.Errorf("resolve terms %q: %w", tbValue, err)
			}
			return StoreSelection{Path: path, Explicit: true}, nil
		}
	}

	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return StoreSelection{}, err
	}
	root := filepath.Dir(projectPath)

	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return StoreSelection{}, fmt.Errorf("load project for terms: %w", lerr)
	}
	rc, rerr := proj.ResolveGovernanceFor(point)
	if rerr != nil {
		return StoreSelection{}, rerr
	}
	if bound := governedTermsPath(root, rc); bound != "" {
		return StoreSelection{Path: bound}, nil
	}
	return StoreSelection{Root: root}, nil
}

// governedTermsPath returns the terms bundle a resolved governance points at,
// absolute, or "" when the project's own terms govern.
//
// Two rungs. The matched profile's explicit `terms:` wins, as written in the
// recipe. Failing that, a profile is answered by its own directory:
// `.kapi/profiles/<name>/terms.json` is that profile's vocabulary by
// convention, and only when the file is actually there — an absent override is
// the ordinary case, and it has to fall through to the project's own terms
// rather than resolve to a path nothing wrote.
func governedTermsPath(root string, rc *project.ResolvedGovernance) string {
	if rc == nil {
		return ""
	}
	if bound := rc.TermStore; bound != "" {
		if !filepath.IsAbs(bound) {
			bound = filepath.Join(root, bound)
		}
		return bound
	}
	if rc.Profile == "" {
		return ""
	}
	conv := filepath.Join(root, project.RelStatePath(project.ProfilesDirName, rc.Profile, ktb.ConventionalName))
	if _, err := os.Stat(conv); err == nil {
		return conv
	}
	return ""
}

// ResolveTermRules builds the term rules governing the project — for each
// concept, the source term and the translation approved for the target locale —
// for the active source and target locales. It reads the committed .terms.json
// serialization, the terms store's durable form (AD-010), directly, so the
// terminology gate validates the committed state and works at check time in CI,
// where the gitignored working-store .db doesn't exist (see projectConcepts for
// the full precedence). Returns nil when no terms store is in scope or it has
// no terms for the locale pair. The result is what every governed step takes
// under the "term_rules" key.
func (a *App) ResolveTermRules(cmd Command, targetLang string) ([]coreprofile.TermRule, error) {
	return a.ResolveTermRulesFor(cmd, targetLang, project.GovernancePoint{})
}

// ResolveTermRulesFor is ResolveTermRules scoped to one point in the
// context space: it reads the terms governing there (the profile's own `terms:`,
// else defaults.terms_source), so a recipe governing two brands enforces each
// brand's vocabulary over its own content. The zero point is the project-wide
// resolution.
func (a *App) ResolveTermRulesFor(cmd Command, targetLang string, point project.GovernancePoint) ([]coreprofile.TermRule, error) {
	concepts, err := a.projectConcepts(cmd, point)
	if err != nil || len(concepts) == 0 {
		return nil, err
	}

	source := model.LocaleID(a.SourceLocale())
	target := model.LocaleID(targetLang)
	if target == "" {
		target = model.LocaleID(a.TargetLang)
	}

	var rules []coreprofile.TermRule
	for _, c := range concepts {
		concept := c
		src := concept.SourceTerm(source)
		if src == nil || src.Text == "" {
			continue
		}
		// A do-not-translate concept is answered by the source term alone. It
		// needs no entry for the target: the whole claim is that this string is
		// the same in every locale, including ones the store has never been
		// told about. Requiring a target term here is why the pseudo locale saw
		// no rules at all — nothing in the store speaks qps — and rendered the
		// product names like ordinary prose.
		if concept.DoNotTranslate {
			rules = append(rules, coreprofile.TermRule{
				Term:           src.Text,
				ConceptID:      concept.ID,
				DoNotTranslate: true,
			})
			continue
		}
		tgt := concept.PreferredTerm(target)
		if tgt == nil || tgt.Text == "" {
			continue
		}
		rules = append(rules, coreprofile.TermRule{
			Term:        src.Text,
			Replacement: tgt.Text,
		})
	}
	return rules, nil
}

// projectConcepts loads the project's terms concepts for the read-only check
// gates. Per the terms store model (AD-010), the committed .terms.json is the authored
// source and the SQLite .db is only a rebuildable read-cache over it — so a ship
// gate validates the *committed* source: when the recipe binds a termbase_source
// we decode it directly, no cache required. That is also why the terminology
// gate works on a fresh CI checkout, where the gitignored .db is absent.
//
// Precedence: an explicit --termstore selects a specific store (honour it); else
// the committed serialization wins; else the working index the recipe binds
// directly (a project whose store holds concepts but binds no committed
// source — the shape a `kapi terms import` leaves behind).
//
// point scopes the resolution to the terms binding governing there; the zero
// point is the project-wide answer.
func (a *App) projectConcepts(cmd Command, point project.GovernancePoint) ([]sqlterms.Concept, error) {
	explicitStore := false
	if cmd != nil {
		if v, _ := cmd.Flags().GetString("termstore"); v != "" {
			explicitStore = true
		}
	}

	// Source of truth: the committed .terms.json serialization.
	if !explicitStore {
		srcPath, err := a.resolveProjectTermsSourcePath(cmd, point)
		if err != nil {
			return nil, err
		}
		if srcPath != "" {
			return conceptsFromKTB(srcPath)
		}
	}

	// Working index: an explicit --termstore, the point's standalone terms
	// binding, or the project's own store. Read directly only when no
	// serialization is bound (or the user explicitly selected a store).
	sel, err := a.ResolveTermsStore(cmd, point)
	if err != nil {
		return nil, err
	}
	ctx := CmdContext(cmd)
	if sel.InProject() {
		db, err := a.ProjectDB(ctx, sel.Root)
		if err != nil {
			return nil, err
		}
		if db.Terms() == nil {
			return nil, nil
		}
		concepts, err := db.Terms().Concepts(ctx)
		if err != nil {
			return nil, fmt.Errorf("list terms concepts: %w", err)
		}
		return concepts, nil
	}
	if sel.Path != "" {
		if _, statErr := os.Stat(sel.Path); statErr == nil {
			// A point's `terms:` binding names a committed bundle, not a
			// working store: it is the one place a recipe still names a terms
			// FILE, so the same path can arrive as either shape and reading it
			// as the wrong one fails with "file is not a database".
			if ktb.IsBundlePath(sel.Path) {
				return conceptsFromKTB(sel.Path)
			}
			tb, err := sqlterms.NewSQLiteStore(sel.Path)
			if err != nil {
				return nil, fmt.Errorf("open terms %q: %w", sel.Path, err)
			}
			defer tb.Close()
			concepts, err := tb.Concepts(ctx)
			if err != nil {
				return nil, fmt.Errorf("list terms concepts: %w", err)
			}
			return concepts, nil
		}
	}
	return nil, nil
}

// conceptsFromKTB decodes the committed .terms.json terms serialization into
// concepts — the read-only fast path a check gate uses to validate the
// committed source of truth without materializing the SQLite working index.
func conceptsFromKTB(path string) ([]sqlterms.Concept, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open terms source %q: %w", path, err)
	}
	defer f.Close()
	file, err := ktb.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode terms source %q: %w", path, err)
	}
	return file.Concepts, nil
}

// resolveProjectTermsSourcePath returns the absolute path of the project's
// committed terms bundle, or "" when there is none.
//
// An explicit defaults.terms_source binding wins. With none, the well-known
// locations are searched, mirroring the ladder the voice profile uses: a
// project that keeps its terms at the conventional path needs no recipe
// entry at all.
//
// A point whose profile binds its own terms has named the vocabulary for that
// content, and the project-wide committed source then belongs to other content
// — so there is no committed source at that point, and the profile's store is
// read through ResolveTermsStore instead. Without this the profile's terms would
// be shadowed in every recipe that also binds defaults.terms_source, which is
// most of them.
func (a *App) resolveProjectTermsSourcePath(cmd Command, point project.GovernancePoint) (string, error) {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return "", err
	}
	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return "", fmt.Errorf("load project for terms source: %w", lerr)
	}
	root := filepath.Dir(projectPath)

	rc, rerr := proj.ResolveGovernanceFor(point)
	if rerr != nil {
		return "", rerr
	}
	if governedTermsPath(root, rc) != "" {
		return "", nil
	}

	src := proj.Defaults.TermsSource
	if src == "" {
		return firstExistingTermsBundle(root), nil
	}
	// Only the canonical terms bundle is resolved live at check time. Lossy
	// interchange sources (CSV, TBX) are import formats: they're compiled into
	// the project store by up/apply and read from there, so we don't try to
	// decode them here.
	if !ktb.IsBundlePath(src) {
		return "", nil
	}
	if !filepath.IsAbs(src) {
		src = filepath.Join(root, src)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		// Bound but missing — no terms to enforce, not an error.
		return "", nil
	}
	return src, nil
}

// firstExistingTermsBundle returns the first well-known terms bundle present
// under root, or "" when none is.
//
// `.kapi/` is searched FIRST: it is committed, and it is where a project's
// authored sources live, so the conventional home and the reviewed home are the
// same directory. The root spelling stays second because a project that keeps
// its terms beside its content is not wrong, and dropping the rung would
// silently unbind it.
func firstExistingTermsBundle(root string) string {
	for _, candidate := range []string{
		filepath.Join(root, project.RelStatePath(ktb.ConventionalName)),
		filepath.Join(root, ktb.ConventionalName),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// runProjectStepsOver runs a project flow's steps over an explicit input set.
// The convergence loop uses it to drive the default flow over a chosen
// (locale, pending-files) slice per pass; RunFromProject drives it once per
// resolved locale pass.
func (a *App) runProjectStepsOver(ctx context.Context, cmd Command, flowName string, spec *flow.StepsSpec, rCtx *flow.ResourceContext, inputPaths []string) error {
	concurrency, _ := cmd.Flags().GetInt("concurrency")

	// A flow whose every tool is monolingual reconciles the source alone, so it
	// runs on a project that declares no target languages. Only a chain that
	// crosses a language pair needs the locale.
	if a.TargetLang == "" && flow.FlowNeedsTargetLanguage(spec, flow.BuildToolInfoMap(a.ToolReg)) {
		return errors.New("--target-lang is required")
	}

	// Assemble the pass's tool chain through the shared builder (placement
	// gate, per-step config resolution, project content memory injection) — the same
	// implementation the multi-locale orchestrator (RunFlowAllLocales) uses.
	projectTools, cleanup, err := a.buildProjectFlowTools(cmd, flowName, spec, rCtx, nil)
	if err != nil {
		return err
	}
	defer cleanup()

	// A convergence worker appends its progress tap as a trailing read-only
	// step: it observes blocks leaving the pipeline and feeds the run's live
	// unit_progress events. Never set outside converge workers.
	if a.convergeProgressTap != nil {
		projectTools = append(projectTools, a.convergeProgressTap)
	}

	// Store original buildFlowTools and temporarily replace it.
	origBuild := a.projectFlowTools
	a.projectFlowTools = projectTools
	defer func() { a.projectFlowTools = origBuild }()

	if len(inputPaths) == 1 {
		return a.RunSingleFile(ctx, cmd, flowName, inputPaths[0])
	}
	outputFlag, _ := cmd.Flags().GetString("output")
	return a.runMultipleFiles(ctx, cmd, flowName, inputPaths, concurrency, outputFlag)
}

// toolFromStep creates a tool.Tool from a flow step definition.
// Uses the tool registry as the single source of truth. If rCtx is non-nil,
// resource references in the step config are resolved before applying.
func (a *App) toolFromStep(step flow.FlowStep, cmd Command, rCtx *flow.ResourceContext) (tool.Tool, func(), error) {
	toolID := registry.ToolID(step.Tool)

	// A step's argv comes from a file, which is not necessarily written by
	// whoever is running kapi. Building an exec-class tool needs an
	// established trust decision for the project (host/exectrust.go).
	noop := func() {}
	if err := a.checkExecToolAllowed(step.Tool); err != nil {
		return nil, noop, err
	}

	// Try config factory first (schema-driven tools).
	if a.ToolReg.Has(toolID) {
		ToolSchema := a.ToolReg.Schema(toolID)
		config := step.Config
		if rCtx != nil && ToolSchema != nil {
			var err error
			config, err = flow.ResolveToolConfig(config, ToolSchema, *rCtx)
			if err != nil {
				return nil, noop, fmt.Errorf("resolve config for %q: %w", step.Tool, err)
			}
		}
		config = a.ApplyProjectBindings(step.Tool, ToolSchema, config)

		// The same grant the direct path makes, at the one place a flow step
		// becomes a tool. It used to be a separate pass over the BUILT tools in
		// flowrun, which could only reach the one config type it knew to assert.
		config, cleanup, err := a.grantMemory(step.Tool, config, cmd)
		if err != nil {
			return nil, noop, err
		}

		t, err := a.ToolReg.NewToolWithConfig(toolID, config, a.TargetLang)
		if err != nil {
			cleanup()
			// Return the real failure (a credential-resolution or config
			// error). NewToolWithConfig already falls back to the zero-arg
			// factory for tools with no ConfigFactory, so retrying NewTool
			// here could only mask the cause (or silently swap in a
			// mock-provider default for AI tools).
			return nil, noop, fmt.Errorf("tool %q: %w", step.Tool, err)
		}
		return t, cleanup, nil
	}

	// Unregistered name: fall back to the zero-arg factory for its error text.
	t, err := a.ToolReg.NewTool(toolID)
	if err != nil {
		return nil, noop, fmt.Errorf("tool %q: %w", step.Tool, err)
	}
	return t, noop, nil
}

// resolveRunBindings returns the standing context a flow run should carry.
//
// The project path (RunFromProject, converge) resolves bindings up front and
// leaves them on the App. A built-in flow given explicit files — `kapi translate
// messages.json` inside a project, `kapi run translate -i …` — does not, so it
// used to see none: the recipe's defaults.tools were ignored and its terms
// never reached the model, while `kapi up` over the same recipe honored both.
// One recipe, two behaviours, depending on the verb. Resolve them here instead.
//
// Outside a project there are no bindings — but `--termstore` is an explicit
// request to use that terminology, and it used to reach only term-check (which
// validates the output afterwards) and never translate (which could have got it
// right the first time). That run now carries a binding set holding just the
// terms, so they are in the prompt.
//
// inputPath is the file this chain will process; the content collection that
// claims it decides which voice and which terms the run carries, so
// `kapi translate` over one brand's content is governed like that brand rather
// than like the project as a whole. Empty resolves the project-wide bindings.
//
// Nothing here is fatal: a recipe or terms that cannot be read leaves the run
// unbound rather than failing a translation over context that is, at worst,
// advisory. The checks still report what the model got wrong.
func (a *App) resolveRunBindings(inputPath string, cmd ...Command) *ProjectBindings {
	if a.ProjectBindings != nil {
		return a.ProjectBindings
	}
	if len(cmd) == 0 || cmd[0] == nil {
		return nil
	}
	c := cmd[0]

	if projectPath, err := ResolveProjectPath(c); err == nil && projectPath != "" {
		if proj, err := project.Load(projectPath); err == nil {
			point := a.GovernancePointFor("", "")
			if inputPath != "" {
				root, aerr := filepath.Abs(filepath.Dir(projectPath))
				if rel, ok := projectRelPath(root, inputPath); aerr == nil && ok {
					point = a.GovernancePointFor("", rel)
				}
			}
			if rc, rerr := proj.ResolveGovernanceFor(point); rerr == nil {
				a.NoteGovernance(c, rc)
			}
			b, err := a.resolveProjectBindings(c, proj, projectPath, point)
			if err == nil {
				return b
			}
			if !a.Quiet {
				fmt.Fprintf(os.Stderr, "Warning: project bindings: %v\n", err)
			}
		}
	}

	// No project: honor an explicit --termstore on its own.
	if tb, _ := c.Flags().GetString("termstore"); tb == "" {
		return nil
	}
	termRules, err := a.ResolveTermRules(c, a.TargetLang)
	if err != nil {
		if !a.Quiet {
			fmt.Fprintf(os.Stderr, "Warning: --termstore: %v\n", err)
		}
		return nil
	}
	if len(termRules) == 0 {
		return nil
	}
	return &ProjectBindings{termRules: termRules}
}

// ApplyProjectBindings injects the project's standing context into a step's
// config: the tool's project preset (defaults.tools, applied under the step's
// own keys — the step wins), then the voice and terminology bindings when
// the tool can honor them and the step did not set them explicitly. Returns
// the (possibly cloned) config so the recipe's in-memory step config is never
// mutated.
func (a *App) ApplyProjectBindings(toolName string, s *schema.ComponentSchema, config map[string]any) map[string]any {
	return a.applyBindings(a.ProjectBindings, toolName, s, config)
}

// applyBindings is ApplyProjectBindings over an explicit binding set, so a run
// with no project (an ad-hoc `kapi translate --termstore …`) can still carry the
// terminology the user asked for.
func (a *App) applyBindings(b *ProjectBindings, toolName string, s *schema.ComponentSchema, config map[string]any) map[string]any {
	if b == nil {
		return config
	}

	// Project preset, then the per-locale preset for the current target locale
	// on top (per-locale wins), then the step's own config on top of both (the
	// step still wins): step > locale > project.
	preset := b.ToolPresets[toolName]
	if lp, ok := b.localePresets[a.TargetLang]; ok && len(lp.Tools) > 0 {
		preset = MergeToolPreset(preset, lp.Tools[toolName])
	}
	config = MergeToolPreset(preset, config)

	clone := func() {
		next := make(map[string]any, len(config)+1)
		maps.Copy(next, config)
		config = next
	}

	// Voice profile → translate steps and recycle. Translate consumes it as
	// prompt guidance; recycle does not consult it, but stamps it (with the
	// term rules below) onto every target it fills so a recycled target is as
	// attributable to the governing context as a translated one.
	if b.profile != nil && (isTranslateTool(toolName, s) || isMemoryRecycleTool(toolName, s)) {
		if _, ok := config["profile"]; !ok {
			clone()
			config["profile"] = b.profile
		}
	}

	// Context point → recycle and translate. A fill asks the content memory from
	// where it is, so a source string the record answers more than one way is
	// answered by the approval nearest this point rather than by whichever
	// wording the corpus happens to repeat most.
	//
	// Translate needs it for a different reason: it reads the block's own
	// history. It used to take no point, on the reasoning that it produces a new
	// translation rather than choosing between approved ones. That is true of
	// the output and beside the point once the previous version became
	// reference — a wording approved for one surface must not steer another.
	if b.point != "" && (isMemoryRecycleTool(toolName, s) || isTranslateTool(toolName, s)) {
		if _, ok := config["point"]; !ok {
			clone()
			config["point"] = b.point
		}
	}

	// Term rules → the steps governed by terminology: term-check, which declares
	// RequiresTerms, and the producers, which do not.
	//
	// Translate renders the rules straight into its prompt, but it declares
	// Requires{TargetLanguage, Credentials} — not Terms — so the project's
	// terminology never reached it. A project with a terms store had its
	// terminology *checked* after the fact, yet never *enforced at generation*:
	// the model was simply never told. Wired here the way the voice profile is
	// above, rather than by adding Terms to translate's Requires, which gates
	// nothing else and would imply a terms store is mandatory.
	//
	// One key, one shape, three tools: each takes the rule list and projects it
	// for itself (a prompt line, a check, a stamped fingerprint), so no tool has
	// to guess what a caller meant by a bare map.
	if len(b.termRules) > 0 && (ToolRequires(s, schema.RequiresTerms) ||
		isTranslateTool(toolName, s) || isMemoryRecycleTool(toolName, s) ||
		isPseudoTranslateTool(toolName, s) || isDNTCheckTool(toolName, s)) {
		if _, ok := config["term_rules"]; !ok {
			clone()
			config["term_rules"] = b.termRules
		}
	}

	return config
}

// MergeToolPreset overlays a step's config onto its project-level tool preset
// (defaults.tools): the preset supplies defaults, the step's own keys win.
// Returns config unchanged when there is no preset, and never mutates either
// input map.
func MergeToolPreset(preset, config map[string]any) map[string]any {
	if len(preset) == 0 {
		return config
	}
	merged := make(map[string]any, len(preset)+len(config))
	maps.Copy(merged, preset)
	maps.Copy(merged, config)
	return merged
}

// isTranslateTool reports whether a step's tool is the translate tool, which
// accepts a voice profile via config["profile"].
func isTranslateTool(toolName string, s *schema.ComponentSchema) bool {
	if toolName == "translate" {
		return true
	}
	return s != nil && s.ToolMeta != nil && s.ToolMeta.ID == "translate"
}

// isMemoryRecycleTool reports whether a step's tool is the memory recycle tool,
// which accepts the governing context via config["profile"] and config["term_rules"]
// to stamp onto the targets it fills.
// isDNTCheckTool reports whether a step is the do-not-translate check.
//
// It is listed here rather than given schema.RequiresTerms because the check is
// valid with no store bound — the recipe may name its terms directly. What it
// must not be is a check with nothing to check: with an empty list it passes
// everything, which is the reassurance a guardrail should never give.
func isDNTCheckTool(toolName string, s *schema.ComponentSchema) bool {
	if toolName == "dnt-check" {
		return true
	}
	return s != nil && s.ToolMeta != nil && s.ToolMeta.ID == "dnt-check"
}

// isPseudoTranslateTool reports whether a step is the pseudo-translate tool.
//
// It takes the rules for the do-not-translate ones alone: a product name has to
// survive the probe, or the probe reports a bug in every string that mentions
// one. Listed here rather than given schema.RequiresTerms, which would make a
// terms store mandatory for a tool that runs offline with nothing bound.
func isPseudoTranslateTool(toolName string, s *schema.ComponentSchema) bool {
	if toolName == "pseudo-translate" {
		return true
	}
	return s != nil && s.ToolMeta != nil && s.ToolMeta.ID == "pseudo-translate"
}

func isMemoryRecycleTool(toolName string, s *schema.ComponentSchema) bool {
	if toolName == "recycle" {
		return true
	}
	return s != nil && s.ToolMeta != nil && s.ToolMeta.ID == "recycle"
}

// startStepProgress starts a 200ms ticker that renders a single-line pipeline
// progress status to w using \r overwrite. Returns a stop function that clears
// the line and stops the ticker.
//
// Output format:
//
//	[2.3s] ● translate [47/120] → ○ qa [32/120] → ◌ term-enforce
func startStepProgress(w io.Writer, metrics *flow.PipelineMetrics) func() {
	start := time.Now()
	ticker := time.NewTicker(200 * time.Millisecond)
	stop := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		for {
			select {
			case <-ticker.C:
				renderStepProgress(w, metrics, start)
			case <-stop:
				return
			}
		}
	}()

	return func() {
		ticker.Stop()
		close(stop)
		<-stopped
		// Clear the line.
		fmt.Fprintf(w, "\r\033[K")
	}
}

func renderStepProgress(w io.Writer, metrics *flow.PipelineMetrics, start time.Time) {
	snap := metrics.Snapshot()
	elapsed := time.Since(start).Truncate(100 * time.Millisecond)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("\r\033[K[%s]", elapsed))

	for i, s := range snap {
		if i > 0 {
			b.WriteString(" → ")
		} else {
			b.WriteByte(' ')
		}

		switch {
		case s.PartsIn == 0:
			// Pending
			b.WriteString("◌ ")
			b.WriteString(s.Name)
		case s.PartsIn > s.PartsOut:
			// Active
			b.WriteString("● ")
			b.WriteString(s.Name)
			b.WriteString(fmt.Sprintf(" [%d/%d]", s.PartsOut, s.PartsIn))
		default:
			// Done
			b.WriteString("○ ")
			b.WriteString(s.Name)
			b.WriteString(fmt.Sprintf(" [%d/%d]", s.PartsOut, s.PartsIn))
		}
	}

	fmt.Fprint(w, b.String())
}
