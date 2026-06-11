package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"io"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
	sqltm "github.com/neokapi/neokapi/sievepen"
	sqltb "github.com/neokapi/neokapi/termbase"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// FlowCmdOptions configures the flow and run commands.
type FlowCmdOptions struct {
	// FallbackRunE is called when the flow name doesn't match a built-in flow.
	// If nil, unknown flow names return an error.
	FallbackRunE func(cmd *cobra.Command, flowName string, args []string) error

	// ExtraFlows returns additional flows for the list command (e.g. project flows).
	ExtraFlows func() []output.FlowInfo
}

// RunFlow executes a flow by name with the given input files.
func (a *App) RunFlow(ctx context.Context, cmd *cobra.Command, flowName string, opts FlowCmdOptions) error {
	inputPaths, _ := cmd.Flags().GetStringSlice("input")
	concurrency, _ := cmd.Flags().GetInt("concurrency")

	if explain, _ := cmd.Flags().GetBool("explain"); explain {
		out, _ := cmd.Flags().GetString("output")
		return explainBindings(cmd.OutOrStdout(), flowName, inputPaths, out)
	}

	if len(inputPaths) > 0 {
		outputFlag, _ := cmd.Flags().GetString("output")
		// Running a flow on a .klz transforms the workspace IN PLACE
		// (appends overlays); output files come later from `kapi merge`. The
		// target locale may come from the workspace recipe, so this runs
		// before the --target-lang requirement below.
		if klzWorkspaceInput(inputPaths) {
			if outputFlag != "" {
				return errKlzTransformOutput
			}
			doPack, _ := cmd.Flags().GetBool("pack")
			return a.transformKlzInPlace(ctx, inputPaths[0], flowName, func() ([]tool.Tool, func(), error) {
				return a.buildFlowTools(flowName, cmd)
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
		if isKlzPath(outputFlag) {
			return errKlzCreateWithExtract
		}
		if len(inputPaths) == 1 {
			return a.runSingleFile(ctx, cmd, flowName, inputPaths[0])
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

// addFlowRunFlags registers the common flags for flow execution commands.
func (a *App) addFlowRunFlags(cmd *cobra.Command) {
	a.AddProcessingFlags(cmd)
	cmd.Flags().StringSliceP("input", "i", nil, "input file path(s); repeat for multiple files")
	cmd.Flags().StringP("output", "o", "", "output path or template (e.g. ./out/{name}_{lang}.{ext})")
	cmd.Flags().IntP("concurrency", "j", 0, "number of files to process at once (0 = auto)")
	cmd.Flags().String("credential", "", "saved credential name to use (see 'kapi credentials list')")
	cmd.Flags().String("provider", "anthropic", "AI provider (anthropic, openai, ollama)")
	cmd.Flags().String("api-key", "", "API key for the AI provider")
	cmd.Flags().String("model", "", "AI model name")
	cmd.Flags().String("trace", "", "write flow trace JSON to file (for flow visualization)")
	cmd.Flags().Bool("pack", false, "when transforming a .klz, also eject the result to the .klz (auto-pack)")
	cmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
	cmd.Flags().String("tm", "", "named TM for tm-leverage flow (resolves from KAPI_HOME)")
	cmd.Flags().String("termbase", "", "named termbase for term-lookup/enforce (resolves from KAPI_HOME)")
	cmd.Flags().Bool("stats", false, "include part/block counts in output")
	cmd.Flags().Bool("explain", false, "print the resolved source → sink bindings and exit without running")
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

// listFlows outputs the list of available flows.
func (a *App) listFlows(cmd *cobra.Command, opts FlowCmdOptions) error {
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
	for _, def := range flow.BuiltInFlows() {
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

func (a *App) runSingleFile(ctx context.Context, cmd *cobra.Command, flowName, inputPath string) error {
	// Resolve format with optional preset syntax (e.g., "okf_html:strict").
	fmtName := a.FormatFlag
	var mergedConfig map[string]any
	if fmtName != "" {
		ref := preset.ParseFormatRef(fmtName)
		fmtName = ref.RegistryName()
		if ref.IsPreset() {
			presetReg := preset.NewPresetRegistry()
			preset.RegisterBuiltins(presetReg)
			resolver := preset.NewConfigResolver(presetReg, a.SchemaReg)
			var err error
			mergedConfig, err = resolver.ResolveFormatConfig(ref.Name, ref.Preset, nil, nil)
			if err != nil {
				return fmt.Errorf("resolve format config: %w", err)
			}
		}
	}

	// Build tools.
	flowTools, cleanup, err := a.buildFlowTools(flowName, cmd)
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

	// Start TTY progress ticker (200ms) if interactive.
	jsonOut, _ := cmd.Flags().GetBool("json")
	showStepProgress := !a.Quiet && !jsonOut && isatty.IsTerminal(os.Stderr.Fd())
	var stopProgress func()
	if showStepProgress {
		stopProgress = startStepProgress(os.Stderr, metrics)
	}

	// In project mode, run against the project's persistent block store
	// (.kapi/cache/blocks.db) so SessionTools cache per-block work as
	// overlays — that is what lets a later run skip already-done steps and
	// makes the project's working state packable (AD-025 §5). Identical
	// output either way; the store only adds the overlay cache.
	var projStore blockstore.Store
	if a.projectContext != nil {
		if s := a.openProjectBlockStore(); s != nil {
			projStore = s
			defer s.Close()
		}
	}

	// Process-only default in a project (AD-026 §3/§5): when a .kapi recipe is
	// in scope AND the user did not pass an explicit -o, the run commits its
	// `targets/<locale>` overlays to the project store and emits no file.
	// Materializing the localized files is then a separate `kapi merge`. An
	// explicit -o (or no project) keeps the file-writing path below.
	processOnly := a.projectContext != nil && projStore != nil && !cmd.Flags().Changed("output")

	// Build reader configuration callback: applies preset config + project defaults.
	configureReader := func(reader format.DataFormatReader, detectedFmt registry.FormatID) error {
		if len(mergedConfig) > 0 {
			if cfg := reader.Config(); cfg != nil {
				if err := cfg.ApplyMap(mergedConfig); err != nil {
					return fmt.Errorf("apply format config: %w", err)
				}
			}
		}
		if a.projectContext != nil {
			if err := a.projectContext.ConfigureReader(reader, string(detectedFmt)); err != nil {
				return fmt.Errorf("apply project format config: %w", err)
			}
		}
		return nil
	}

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:       a.FormatReg,
		SourceLocale:    model.LocaleID(a.SourceLang),
		Encoding:        a.Encoding,
		Recorder:        recorder,
		Store:           projStore,
		ConfigureReader: configureReader,
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
		if !a.Quiet {
			fmt.Fprintf(cmd.OutOrStdout(),
				"Committed overlays for %s to the project store — run `kapi merge` to write localized files.\n",
				filepath.Base(inputPath))
		}
		return nil
	}

	// Resolve output path.
	outputPath, _ := cmd.Flags().GetString("output")
	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		base := inputPath[:len(inputPath)-len(ext)]
		outputPath = fmt.Sprintf("%s_%s%s", base, a.TargetLang, ext)
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

	// Write trace JSON if --trace was set.
	if tracePath != "" && recorder != nil {
		detectedFmt := fmtName
		if detectedFmt == "" {
			detected, _ := a.FormatReg.DetectByExtension(filepath.Ext(inputPath))
			detectedFmt = string(detected)
		}
		a.writeTraceFile(tracePath, flowName, detectedFmt, inputPath, outputPath, recorder, stepNames)
	}

	if !a.Quiet {
		out := output.FlowRunOutput{
			FlowName:   flowName,
			InputPath:  inputPath,
			OutputPath: outputPath,
		}
		return output.Print(cmd, out)
	}
	return nil
}

// writeTraceFile serializes a trace to JSON and writes it to disk. toolNames is
// the ordered list of tool names (one per "tool-N" node) used to label the
// graph nodes — without it the nodes would fall back to their bare "tool-N" ids.
func (a *App) writeTraceFile(tracePath, flowName, fmtName, inputPath, outputPath string, recorder *flow.TraceRecorder, toolNames []string) {
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
		return
	}
	_ = os.MkdirAll(filepath.Dir(tracePath), 0o755)
	_ = os.WriteFile(tracePath, traceJSON, 0o644)
}

func (a *App) runMultipleFiles(ctx context.Context, cmd *cobra.Command, flowName string, inputPaths []string, concurrency int, outputTemplate string) error {
	if concurrency <= 0 {
		if a.projectContext != nil && a.projectContext.Concurrency > 0 {
			concurrency = a.projectContext.Concurrency
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

	var mu sync.Mutex
	var processed int

	for _, inputPath := range inputPaths {
		g.Go(func() error {
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

			fmtName, nodes, err := a.processFlowFile(ctx, cmd, flowName, inputPath, outputTemplate, recorder)

			if tracePath != "" {
				info.endUs = time.Since(batchStart).Microseconds()
				info.format = fmtName
				info.nodes = nodes
				mu.Lock()
				traceInfos = append(traceInfos, info)
				mu.Unlock()
				lanes <- lane
			}

			if err != nil {
				return fmt.Errorf("%s: %w", inputPath, err)
			}
			mu.Lock()
			processed++
			mu.Unlock()
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

	if !a.Quiet {
		return output.Print(cmd, output.FlowRunOutput{
			FlowName:       flowName,
			FilesProcessed: processed,
		})
	}
	return nil
}

// processFlowFile performs the full read → process → write cycle for a single file.
// Safe for concurrent use — each call uses its own reader, writer, and tool instances.
// When recorder is non-nil, tools are wrapped with TracingTool for batch tracing.
// Returns the detected format name and trace nodes (both empty when recorder is nil).
//
// All formats — built-in and Mode-C plugin-backed — use the standard
// read → process → write pipeline. Plugin-backed formats are routed
// through their daemon transparently by the registered factories.
func (a *App) processFlowFile(ctx context.Context, cmd *cobra.Command, flowName, inputPath, outputTemplate string, recorder *flow.TraceRecorder) (string, []flow.TraceNode, error) {
	fmtName := a.FormatFlag
	if fmtName == "" {
		// Use project-scoped detection when running in project mode.
		if a.projectContext != nil {
			fmtName = a.projectContext.DetectFormat(a.FormatReg, inputPath)
		}
		if fmtName == "" {
			// Content-aware: disambiguates extensions claimed by several formats
			// (e.g. .xliff 1.x vs 2.x) by the file head, not extension alone.
			detected, err := a.FormatReg.DetectFile(inputPath, nil)
			if err != nil {
				return "", nil, fmt.Errorf("unable to detect format: %w", err)
			}
			fmtName = string(detected)
		}
	}

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
			return "", nil, fmt.Errorf("resolve format config: %w", err)
		}
	}

	reader, err := a.FormatReg.NewReader(registry.FormatID(registryName))
	if err != nil {
		return "", nil, fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}

	if len(mergedConfig) > 0 {
		if cfg := reader.Config(); cfg != nil {
			if err := cfg.ApplyMap(mergedConfig); err != nil {
				return "", nil, fmt.Errorf("apply format config: %w", err)
			}
		}
	}

	// Apply project format defaults.
	if a.projectContext != nil {
		if err := a.projectContext.ConfigureReader(reader, fmtName); err != nil {
			return "", nil, fmt.Errorf("apply project format config: %w", err)
		}
	}

	// All formats use the standard read → process → write pipeline.
	// Plugin-backed formats are routed through their Mode-C daemon
	// transparently by the registered factories.
	nodes, err := a.processFlowFileNative(ctx, cmd, flowName, inputPath, outputTemplate, registryName, reader, mergedConfig, recorder)
	return fmtName, nodes, err
}

// processFlowFileNative uses the standard read → process → write pipeline.
// When recorder is non-nil, tools are wrapped with TracingTool and reader/writer
// events are recorded. Returns trace nodes (nil when recorder is nil).
func (a *App) processFlowFileNative(ctx context.Context, cmd *cobra.Command, flowName, inputPath, outputTemplate, registryName string, reader format.DataFormatReader, mergedConfig map[string]any, recorder *flow.TraceRecorder) ([]flow.TraceNode, error) {
	// Build fresh tool instances for this file (thread-safe in batch mode).
	flowTools, cleanup, err := a.buildFlowTools(flowName, cmd)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Auto-wrap IO-bound tools with ParallelBlockTool.
	if n := a.resolveParallelBlocks(flowName); n > 1 {
		for i, ft := range flowTools {
			flowTools[i] = tool.NewParallelBlockTool(ft, n)
		}
	}

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
		traceNodes = append(traceNodes, flow.TraceNode{
			ID: "writer", Type: flow.NodeWriter, Name: registryName, Label: registryName + " writer",
		})
	}

	outputPath := a.resolveOutputPath(inputPath, outputTemplate)

	// Writer format defaults to the reader's format (same-in / same-out
	// round-trip) but a different output extension selects a different
	// writer. This is how "convert file.json → file.klf → file.mo" works
	// without a dedicated --writer flag — the output path is already the
	// user's intent declaration.
	writerFormatName := registryName
	if ext := filepath.Ext(outputPath); ext != "" {
		if det, err := a.FormatReg.DetectByExtension(ext); err == nil && det != "" {
			writerFormatName = string(det)
		}
	}
	// Reader is pre-created and pre-configured by processFlowFile.
	// Pass it via RunFileWithReaderWriter since format detection already happened.
	writer, err := a.FormatReg.NewWriter(registry.FormatID(writerFormatName))
	if err != nil {
		return traceNodes, fmt.Errorf("no writer for %q: %w", writerFormatName, err)
	}

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    a.FormatReg,
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
	})

	if err := runner.RunFileWithReaderWriter(ctx, flowName, flowTools, inputPath, outputPath, a.TargetLang, reader, writer); err != nil {
		return traceNodes, err
	}

	return traceNodes, nil
}

// resolveOutputPath computes the output file path for a given input file.
// If outputTemplate is empty, uses the default pattern: {base}_{lang}{ext}.
// If outputTemplate contains template placeholders ({name}, {lang}, {ext}, {dir}),
// they are expanded. Otherwise, outputTemplate is used as-is (single-file mode).
func (a *App) resolveOutputPath(inputPath, outputTemplate string) string {
	ext := filepath.Ext(inputPath)
	name := filepath.Base(inputPath[:len(inputPath)-len(ext)])
	dir := filepath.Dir(inputPath)

	if outputTemplate == "" {
		return filepath.Join(dir, fmt.Sprintf("%s_%s%s", name, a.TargetLang, ext))
	}

	// Check if template contains placeholders.
	if strings.Contains(outputTemplate, "{") {
		extNoDot := ""
		if len(ext) > 1 {
			extNoDot = ext[1:]
		}
		out := expandOutputTemplate(outputTemplate, name, a.TargetLang, extNoDot, dir)
		// Ensure output directory exists (template may target a new directory).
		if outDir := filepath.Dir(out); outDir != "." {
			_ = os.MkdirAll(outDir, 0o755)
		}
		return out
	}

	// Literal path (single-file mode).
	return outputTemplate
}

// expandOutputTemplate replaces {name}, {lang}, {ext}, and {dir} placeholders
// in a path template. ext should be without the leading dot.
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
// function releases any resources opened during tool creation (e.g. SQLite TM).
// Callers must defer cleanup() after checking err.
func (a *App) buildFlowTools(flowName string, cmd ...*cobra.Command) ([]tool.Tool, func(), error) {
	noop := func() {}

	// If project flow tools are set (via runProjectSteps), use them directly.
	if a.projectFlowTools != nil {
		return a.projectFlowTools, noop, nil
	}

	// Look up the flow definition from the built-in registry.
	var flowDef *flow.FlowDefinition
	for _, def := range flow.BuiltInFlows() {
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
		"source_locale": a.SourceLang,
		"target_locale": a.TargetLang,
	}

	// Inject the project's bound brand voice profile so built-in flows
	// (e.g. ai-translate-qa) run on-brand when executed inside a project.
	// ai-translate reads config["profile"]; tools that don't recognise it
	// ignore the key.
	if a.projectBindings != nil && a.projectBindings.profile != nil {
		config["profile"] = a.projectBindings.profile
	}

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
	}

	for _, tn := range toolNodes {
		// A graph/built-in node may carry per-tool config (e.g. redact's detectors
		// and entityTypes); overlay it on the shared run config for this node only.
		toolConfig := config
		if len(tn.config) > 0 {
			toolConfig = mergeFlowNodeConfig(config, tn.config)
		}
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

	return builtTools, cleanup, nil
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
	return def.CheckPlacement(a.ToolReg)
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
// cleanup function. Uses ToolInfo.Requires to drive resource setup (termbase,
// TM) rather than hardcoding tool names.
func (a *App) buildToolByName(toolName string, config map[string]any, cmd ...*cobra.Command) ([]tool.Tool, func(), error) {
	if a.ToolReg == nil || !a.ToolReg.Has(registry.ToolID(toolName)) {
		return nil, nil, fmt.Errorf("tool %q not found in registry", toolName)
	}

	info := a.ToolReg.ToolInfo(registry.ToolID(toolName))

	// Resource setup driven by Requires metadata.
	if info != nil {
		for _, req := range info.Requires {
			switch req {
			case "termbase":
				// Tools requiring a termbase get term lookup/enforce tools appended.
				t, err := a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), config, a.TargetLang)
				if err != nil {
					return nil, nil, err
				}
				qaTools := []tool.Tool{t}
				var cleanup func()
				if tb, tbCleanup, err := a.openTermbase(cmd...); err != nil {
					return nil, nil, err
				} else if tb != nil {
					qaTools = append(qaTools,
						sqltb.NewTermLookupTool(tb, sqltb.TermLookupConfig{
							SourceLocale: model.LocaleID(a.SourceLang),
							TargetLocale: model.LocaleID(a.TargetLang),
						}),
						sqltb.NewTermEnforceTool(tb, sqltb.TermEnforceConfig{
							SourceLocale: model.LocaleID(a.SourceLang),
							TargetLocale: model.LocaleID(a.TargetLang),
						}),
					)
					cleanup = tbCleanup
				}
				return qaTools, cleanup, nil

			case "tm":
				// Tools requiring a TM get a real SQLite provider injected from
				// the --tm flag or, with no flag, the project's .kapi/tm.db.
				tmConfig := map[string]any{
					"source_locale":   a.SourceLang,
					"target_locale":   a.TargetLang,
					"fuzzy_threshold": 70,
				}
				var cleanup func()
				var provider coretools.TMProvider
				if len(cmd) > 0 && cmd[0] != nil {
					p, cl, err := a.openToolTM(cmd[0])
					if err != nil {
						return nil, nil, err
					}
					cleanup = cl
					provider = p
				}
				t, err := a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), tmConfig, a.TargetLang)
				if err != nil {
					if cleanup != nil {
						cleanup()
					}
					return nil, nil, err
				}
				// The tm-leverage config factory cannot read a non-JSON provider
				// or the schema-hidden SourceLocale from the config map (both are
				// schema:"-"), so it defaults to NullTMProvider with an empty
				// source locale — which makes the SQLite lookup (WHERE locale = ?)
				// match nothing. Set both on the created tool so the flow step
				// actually leverages.
				if provider != nil {
					if cfg, ok := t.Config().(*coretools.TMLeverageConfig); ok {
						cfg.Provider = provider
						if cfg.SourceLocale.IsEmpty() && a.SourceLang != "" {
							cfg.SourceLocale = model.LocaleID(a.SourceLang)
						}
					}
				}
				return []tool.Tool{t}, cleanup, nil
			}
		}
	}

	// Default: create from registry.
	t, err := a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), config, a.TargetLang)
	if err != nil {
		return nil, nil, err
	}
	return []tool.Tool{t}, nil, nil
}

// resolveParallelBlocks returns the parallel block concurrency to use.
// Prefers project context setting, then falls back to flow/tool defaults.
func (a *App) resolveParallelBlocks(flowName string) int {
	if a.projectContext != nil && a.projectContext.ParallelBlocks > 0 {
		return a.projectContext.ParallelBlocks
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
	for _, def := range flow.BuiltInFlows() {
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

// cliTMProvider adapts a CLI translation memory to the tools.TMProvider
// interface. The backing store is any sievepen.TranslationMemory — a SQLite TM
// opened from a file, or the in-memory backend seeded in the wasm build — so
// tm-leverage works both natively and offline in the browser.
type cliTMProvider struct {
	tm sqltm.TranslationMemory
}

func (p *cliTMProvider) LookupExact(source string, sourceLocale, targetLocale model.LocaleID) (string, bool) {
	opts := sqltm.LookupOptions{
		MinScore:   1.0,
		MaxResults: 1,
		MatchModes: []sqltm.MatchMode{sqltm.MatchModePlain},
	}
	// TODO: thread a real context once tools.TMProvider carries one.
	matches, err := p.tm.LookupText(context.Background(), source, sourceLocale, targetLocale, opts)
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return matches[0].Entry.VariantText(targetLocale), true
}

// LookupBlock implements coretools.BlockTMProvider: a structure-aware
// lookup over the block's full source Run sequence via sievepen's tiered
// matching (generalized → structural → plain → fuzzy). A structurally
// identical entry scores 100; a plain-text exact with differing inline
// codes is capped below 100 by the TM; ambiguous exacts (several
// full-score entries with differing targets) carry the Ambiguous flag so
// the tool records them without filling.
func (p *cliTMProvider) LookupBlock(block *model.Block, sourceLocale, targetLocale model.LocaleID, threshold int) (coretools.TMBlockMatch, bool) {
	opts := sqltm.LookupOptions{
		MinScore:   float64(threshold) / 100.0,
		MaxResults: 1,
	}
	// TODO: thread a real context once tools.TMProvider carries one.
	matches, err := p.tm.Lookup(context.Background(), block, sourceLocale, targetLocale, opts)
	if err != nil || len(matches) == 0 {
		return coretools.TMBlockMatch{}, false
	}
	m := matches[0]
	runs := m.Entry.Variant(targetLocale)
	if len(runs) == 0 {
		return coretools.TMBlockMatch{}, false
	}
	return coretools.TMBlockMatch{
		TargetRuns: runs,
		Score:      int(math.Round(m.Score * 100)),
		Exact:      m.MatchType.IsExact(),
		Ambiguous:  m.Ambiguous,
	}, true
}

func (p *cliTMProvider) LookupFuzzy(source string, sourceLocale, targetLocale model.LocaleID, threshold int) (string, int, bool) {
	minScore := float64(threshold) / 100.0
	opts := sqltm.LookupOptions{
		MinScore:   minScore,
		MaxResults: 1,
	}
	// TODO: thread a real context once tools.TMProvider carries one.
	matches, err := p.tm.LookupText(context.Background(), source, sourceLocale, targetLocale, opts)
	if err != nil || len(matches) == 0 {
		return "", 0, false
	}
	score := int(matches[0].Score * 100)
	return matches[0].Entry.VariantText(targetLocale), score, true
}

// openTermbase resolves the --termbase flag and opens a SQLite termbase.
// The flag value can be a named resource (no path separators) which resolves
// via KAPI_HOME, or an explicit file path. When no flag is given but a .kapi
// project is in scope with a bound termbase (defaults.termbase) or a
// <root>/.kapi/termbase.db convention file, that project termbase is opened
// instead, so term tools in built-in flows are project-aware flag-free.
// Returns (nil, noop, nil) when neither a flag nor a project termbase exists.
func (a *App) openTermbase(cmd ...*cobra.Command) (*sqltb.SQLiteTermBase, func(), error) {
	noop := func() {}
	if len(cmd) == 0 || cmd[0] == nil {
		return nil, noop, nil
	}
	tbValue, _ := cmd[0].Flags().GetString("termbase")

	var tbPath string
	switch {
	case tbValue == "":
		// No flag — fall back to the project's bound termbase.
		p, err := a.resolveProjectTermbasePath(cmd[0])
		if err != nil {
			return nil, nil, err
		}
		if p == "" {
			return nil, noop, nil
		}
		if _, statErr := os.Stat(p); statErr != nil {
			// Bound but not yet created — nothing to enforce.
			return nil, noop, nil
		}
		tbPath = p
	case strings.ContainsAny(tbValue, "/\\") || strings.HasSuffix(tbValue, ".db"):
		// Explicit file path.
		tbPath = tbValue
	default:
		// Named resource.
		var err error
		tbPath, err = resolveNamedResource("termbases", tbValue)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve termbase %q: %w", tbValue, err)
		}
	}
	tb, err := sqltb.NewSQLiteTermBase(tbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open termbase %q: %w", tbPath, err)
	}
	return tb, func() { tb.Close() }, nil
}

// openToolTM resolves the TM a `tm`-requiring tool (e.g. tm-leverage) should
// leverage and opens it as a TMProvider. The --tm flag wins: a named resource
// (no path separators) resolves via KAPI_HOME, an explicit file path is opened
// directly. When no flag is given but a .kapi project is in scope, the project's
// authoritative TM (<root>/.kapi/tm.db) is opened, so `kapi tm-leverage fr.json`
// leverages the same TM that `kapi extract`/`kapi merge` use — with no flag.
//
// Returns (nil, noop, nil) when no TM is in scope, or when the resolved DB does
// not exist outside a project, preserving today's no-match behavior rather than
// erroring. Inside a project the .kapi/tm.db file is opened (and created on
// demand by SQLite) so the tool leverages it. This mirrors openTermbase and
// reuses the same resolution as the `kapi tm` subcommands (resolveProjectTMPath).
func (a *App) openToolTM(cmd *cobra.Command) (coretools.TMProvider, func(), error) {
	noop := func() {}
	if cmd == nil {
		return nil, noop, nil
	}
	tmValue, _ := cmd.Flags().GetString("tm")

	// A pre-seeded in-memory backend (the wasm build, or any host that sets
	// a.TMBackend) is the authoritative TM and the only one that works without
	// the SQLite driver — prefer it over any on-disk project path. The native
	// CLI never sets a.TMBackend, so this only takes effect in the browser/seed
	// case; the on-disk resolution below is unchanged for the native binary.
	// An explicit --tm path still wins (handled in the switch).
	if tmValue == "" && a.TMBackend != nil {
		return &cliTMProvider{tm: a.TMBackend}, noop, nil
	}

	var tmPath string
	switch {
	case tmValue == "":
		// No flag — fall back to the project's authoritative TM.
		p, err := a.resolveProjectTMPath(cmd)
		if err != nil {
			return nil, nil, err
		}
		if p == "" {
			return nil, noop, nil
		}
		tmPath = p
	case strings.ContainsAny(tmValue, "/\\") || strings.HasSuffix(tmValue, ".db"):
		// Explicit file path.
		tmPath = tmValue
	default:
		// Named resource.
		var err error
		tmPath, err = resolveNamedResource("tm", tmValue)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve TM %q: %w", tmValue, err)
		}
	}

	tm, err := sqltm.NewSQLiteTM(tmPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open TM %q: %w", tmPath, err)
	}
	return &cliTMProvider{tm: tm}, func() { tm.Close() }, nil
}

// projectBindings holds the standing brand-voice + glossary context resolved
// from a .kapi project, applied to project-flow steps that can honor them.
type projectBindings struct {
	// profile is the resolved brand voice profile (defaults.brand_voice),
	// injected into translate steps as config["profile"]. nil when unbound.
	profile *brand.VoiceProfile
	// glossary is the source→target glossary built from the project termbase
	// (defaults.termbase), injected into term-check steps. nil when unbound.
	glossary []coretools.GlossaryEntry
	// toolPresets holds the project-level tool presets (defaults.tools):
	// per-tool config defaults merged under each step's own config wherever
	// that tool runs in a project flow (the step wins per key). nil when the
	// recipe declares none.
	toolPresets map[string]map[string]any
}

// resolveProjectBindings resolves the standing brand-voice + glossary context
// for a project flow run. The brand voice comes from defaults.brand_voice (or
// a convention brand.yaml); the glossary comes from the project termbase
// (defaults.termbase or <root>/.kapi/termbase.db). Returns nil when the
// project carries neither, so ad-hoc behavior is unchanged.
func (a *App) resolveProjectBindings(cmd *cobra.Command, proj *project.KapiProject, projectPath string) (*projectBindings, error) {
	root := filepath.Dir(projectPath)

	profile, _, _, err := a.loadBoundBrandProfile(cmd, proj, root)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		// Convention file fallback at the project root.
		for _, conv := range []string{
			filepath.Join(root, "brand.yaml"),
			filepath.Join(root, project.StateDirName, "brand.yaml"),
		} {
			p, lerr := loadProfileFile(conv)
			if lerr != nil {
				return nil, lerr
			}
			if p != nil {
				profile = p
				break
			}
		}
	}

	glossary, err := a.resolveProjectGlossary(cmd, a.TargetLang)
	if err != nil {
		return nil, err
	}

	if profile == nil && len(glossary) == 0 && len(proj.Defaults.Tools) == 0 {
		return nil, nil
	}
	return &projectBindings{profile: profile, glossary: glossary, toolPresets: proj.Defaults.Tools}, nil
}

// toolRequires reports whether the tool schema declares the named requirement.
func toolRequires(s *schema.ComponentSchema, req string) bool {
	if s == nil || s.ToolMeta == nil {
		return false
	}
	return slices.Contains(s.ToolMeta.Requires, req)
}

// resolveProjectTermbasePath returns the termbase path a project-aware tool
// command should use, with no flag. Resolution order:
//
//  1. An explicit --termbase flag (named resource or path).
//  2. defaults.termbase in the .kapi recipe (relative to the project root).
//  3. <projectRoot>/.kapi/termbase.db when it exists.
//
// Returns "" (with nil error) when nothing resolves, so callers fall through
// to the tool's default (no glossary).
func (a *App) resolveProjectTermbasePath(cmd *cobra.Command) (string, error) {
	if cmd != nil {
		if tbValue, _ := cmd.Flags().GetString("termbase"); tbValue != "" {
			if strings.ContainsAny(tbValue, "/\\") || strings.HasSuffix(tbValue, ".db") {
				return tbValue, nil
			}
			path, err := resolveNamedResource("termbases", tbValue)
			if err != nil {
				return "", fmt.Errorf("resolve termbase %q: %w", tbValue, err)
			}
			return path, nil
		}
	}

	projectPath, err := ResolveProjectPath(cmd)
	if err != nil {
		return "", err
	}
	if projectPath == "" {
		return "", nil
	}
	root := filepath.Dir(projectPath)

	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return "", fmt.Errorf("load project for termbase: %w", lerr)
	}
	if bound := proj.Defaults.Termbase; bound != "" {
		if !filepath.IsAbs(bound) {
			bound = filepath.Join(root, bound)
		}
		return bound, nil
	}

	// Convention: the project's authoritative termbase under .kapi/.
	conv := filepath.Join(root, project.StateDirName, "termbase.db")
	if _, statErr := os.Stat(conv); statErr == nil {
		return conv, nil
	}
	return "", nil
}

// resolveProjectGlossary builds a source→target glossary from the project's
// bound termbase (see resolveProjectTermbasePath), for the active source and
// target locales. Returns nil when no termbase is in scope or it has no terms
// for the locale pair. The result is suitable for injection into a
// term-check tool config under the "glossary" key.
func (a *App) resolveProjectGlossary(cmd *cobra.Command, targetLang string) ([]coretools.GlossaryEntry, error) {
	tbPath, err := a.resolveProjectTermbasePath(cmd)
	if err != nil {
		return nil, err
	}
	if tbPath == "" {
		return nil, nil
	}
	if _, statErr := os.Stat(tbPath); statErr != nil {
		// A bound path that doesn't exist yet is not an error here — the
		// project simply has no glossary to enforce.
		return nil, nil
	}

	tb, err := sqltb.NewSQLiteTermBase(tbPath)
	if err != nil {
		return nil, fmt.Errorf("open termbase %q: %w", tbPath, err)
	}
	defer tb.Close()

	source := model.LocaleID(a.SourceLang)
	target := model.LocaleID(targetLang)
	if target == "" {
		target = model.LocaleID(a.TargetLang)
	}

	concepts, err := tb.Concepts(cmdContext(cmd))
	if err != nil {
		return nil, fmt.Errorf("list termbase concepts: %w", err)
	}
	var glossary []coretools.GlossaryEntry
	for _, c := range concepts {
		concept := c
		src := concept.SourceTerm(source)
		if src == nil || src.Text == "" {
			continue
		}
		tgt := concept.PreferredTerm(target)
		if tgt == nil || tgt.Text == "" {
			continue
		}
		glossary = append(glossary, coretools.GlossaryEntry{
			Source: src.Text,
			Target: tgt.Text,
		})
	}
	return glossary, nil
}

// runProjectSteps executes a flow defined in a .kapi project file.
// It resolves tools by name from the registry, applying per-step configs,
// and runs the flow using the standard single/multi-file execution path.
func (a *App) runProjectSteps(ctx context.Context, cmd *cobra.Command, flowName string, spec *flow.StepsSpec, rCtx *flow.ResourceContext) error {
	inputPaths, _ := cmd.Flags().GetStringSlice("input")
	concurrency, _ := cmd.Flags().GetInt("concurrency")

	if a.TargetLang == "" {
		return errors.New("--target-lang is required")
	}

	// Build tools from step definitions using the tool registry. Transformers
	// are ordinary ordered steps (AD-006); their position is validated by the
	// placement pass at flow load, not by a structural stage here.
	//
	// Tools that require a TM (e.g. tm-leverage) need the project's TM provider
	// injected: toolFromStep can't read the schema-hidden Provider/SourceLocale
	// from the step config, so it would default to a no-match NullTMProvider.
	// Open the TM once, share it across every TM step, and close it after the run.
	var tmCleanups []func()
	defer func() {
		for _, c := range tmCleanups {
			c()
		}
	}()
	injectTM := func(step flow.FlowStep, t tool.Tool) error {
		if !toolRequires(a.ToolReg.Schema(registry.ToolID(step.Tool)), schema.RequiresTM) {
			return nil
		}
		cfg, ok := t.Config().(*coretools.TMLeverageConfig)
		if !ok {
			return nil
		}
		provider, cleanup, err := a.openToolTM(cmd)
		if err != nil {
			return err
		}
		if cleanup != nil {
			tmCleanups = append(tmCleanups, cleanup)
		}
		if provider != nil {
			cfg.Provider = provider
		}
		if cfg.SourceLocale.IsEmpty() && a.SourceLang != "" {
			cfg.SourceLocale = model.LocaleID(a.SourceLang)
		}
		return nil
	}

	if len(spec.SourceTransforms) > 0 {
		return fmt.Errorf("flow %q uses the removed source_transforms stage (AD-006): list transformers as ordered steps", flowName)
	}
	// Transformer placement gate (AD-006) over the compiled steps graph —
	// unconditional at load, like the built-in flow path. The gate sees each
	// node's preset-merged config (defaults.tools), so a preset that enables
	// entity detection drives the same contract resolution the runtime uses.
	if nodes, edges, err := flow.StepsToGraph(spec); err == nil {
		if b := a.projectBindings; b != nil && len(b.toolPresets) > 0 {
			for i := range nodes {
				nodes[i].Config = mergeToolPreset(b.toolPresets[nodes[i].Name], nodes[i].Config)
			}
		}
		def := &flow.FlowDefinition{ID: flowName, Name: flowName, Nodes: nodes, Edges: edges}
		if err := a.checkFlowPlacement(def); err != nil {
			return err
		}
	}
	var projectTools []tool.Tool
	for _, step := range spec.Steps {
		t, err := a.toolFromStep(step, cmd, rCtx)
		if err != nil {
			return fmt.Errorf("flow %q: %w", flowName, err)
		}
		if err := injectTM(step, t); err != nil {
			return fmt.Errorf("flow %q: %w", flowName, err)
		}
		projectTools = append(projectTools, t)
	}

	// Store original buildFlowTools and temporarily replace it.
	origBuild := a.projectFlowTools
	a.projectFlowTools = projectTools
	defer func() { a.projectFlowTools = origBuild }()

	if len(inputPaths) == 1 {
		return a.runSingleFile(ctx, cmd, flowName, inputPaths[0])
	}
	outputFlag, _ := cmd.Flags().GetString("output")
	return a.runMultipleFiles(ctx, cmd, flowName, inputPaths, concurrency, outputFlag)
}

// toolFromStep creates a tool.Tool from a flow step definition.
// Uses the tool registry as the single source of truth. If rCtx is non-nil,
// resource references in the step config are resolved before applying.
func (a *App) toolFromStep(step flow.FlowStep, cmd *cobra.Command, rCtx *flow.ResourceContext) (tool.Tool, error) {
	toolID := registry.ToolID(step.Tool)

	// Try config factory first (schema-driven tools).
	if a.ToolReg.Has(toolID) {
		toolSchema := a.ToolReg.Schema(toolID)
		config := step.Config
		if rCtx != nil && toolSchema != nil {
			var err error
			config, err = flow.ResolveToolConfig(config, toolSchema, *rCtx)
			if err != nil {
				return nil, fmt.Errorf("resolve config for %q: %w", step.Tool, err)
			}
		}
		config = a.applyProjectBindings(step.Tool, toolSchema, config)
		t, err := a.ToolReg.NewToolWithConfig(toolID, config, a.TargetLang)
		if err == nil {
			return t, nil
		}
		// If NewToolWithConfig failed (e.g., no ConfigFactory), fall through
		// to the zero-arg factory below.
	}

	// Fall back to zero-arg factory.
	t, err := a.ToolReg.NewTool(toolID)
	if err != nil {
		return nil, fmt.Errorf("tool %q: %w", step.Tool, err)
	}
	return t, nil
}

// applyProjectBindings injects the project's standing context into a step's
// config: the tool's project preset (defaults.tools, applied under the step's
// own keys — the step wins), then the brand-voice and glossary bindings when
// the tool can honor them and the step did not set them explicitly. Returns
// the (possibly cloned) config so the recipe's in-memory step config is never
// mutated.
func (a *App) applyProjectBindings(toolName string, s *schema.ComponentSchema, config map[string]any) map[string]any {
	b := a.projectBindings
	if b == nil {
		return config
	}

	config = mergeToolPreset(b.toolPresets[toolName], config)

	clone := func() {
		next := make(map[string]any, len(config)+1)
		maps.Copy(next, config)
		config = next
	}

	// Brand voice → translate steps (ai-translate / its "translate" alias).
	if b.profile != nil && isTranslateTool(toolName, s) {
		if _, ok := config["profile"]; !ok {
			clone()
			config["profile"] = b.profile
		}
	}

	// Glossary → termbase-requiring steps (term-check).
	if len(b.glossary) > 0 && toolRequires(s, schema.RequiresTermbase) {
		if _, ok := config["glossary"]; !ok {
			clone()
			config["glossary"] = b.glossary
		}
	}

	return config
}

// mergeToolPreset overlays a step's config onto its project-level tool preset
// (defaults.tools): the preset supplies defaults, the step's own keys win.
// Returns config unchanged when there is no preset, and never mutates either
// input map.
func mergeToolPreset(preset, config map[string]any) map[string]any {
	if len(preset) == 0 {
		return config
	}
	merged := make(map[string]any, len(preset)+len(config))
	maps.Copy(merged, preset)
	maps.Copy(merged, config)
	return merged
}

// isTranslateTool reports whether a step's tool is the AI translate tool,
// which accepts a brand voice profile via config["profile"].
func isTranslateTool(toolName string, s *schema.ComponentSchema) bool {
	if toolName == "ai-translate" {
		return true
	}
	if s != nil && s.ToolMeta != nil {
		if s.ToolMeta.ID == "ai-translate" {
			return true
		}
		if slices.Contains(s.ToolMeta.Aliases, "translate") && s.ToolMeta.ID == "ai-translate" {
			return true
		}
	}
	return false
}

// startStepProgress starts a 200ms ticker that renders a single-line pipeline
// progress status to w using \r overwrite. Returns a stop function that clears
// the line and stops the ticker.
//
// Output format:
//
//	[2.3s] ● ai-translate [47/120] → ○ qa-check [32/120] → ◌ term-enforce
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
