package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
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

	if len(inputPaths) > 0 {
		if a.TargetLang == "" {
			// Check tool registry for a default locale (e.g., pseudo-translate → "qps").
			if info := a.ToolReg.GetToolInfo(registry.ToolID(flowName)); info != nil && info.DefaultLocale != "" {
				a.TargetLang = string(info.DefaultLocale)
			} else {
				return errors.New("--target-lang is required")
			}
		}
		outputFlag, _ := cmd.Flags().GetString("output")
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
	cmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
	cmd.Flags().String("tm", "", "named TM for tm-leverage flow (resolves from KAPI_HOME)")
	cmd.Flags().String("termbase", "", "named termbase for term-lookup/enforce (resolves from KAPI_HOME)")
	cmd.Flags().Bool("stats", false, "include part/block counts in output")
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

	// Resolve output path.
	outputPath, _ := cmd.Flags().GetString("output")
	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		base := inputPath[:len(inputPath)-len(ext)]
		outputPath = fmt.Sprintf("%s_%s%s", base, a.TargetLang, ext)
	}

	// Build reader configuration callback: applies preset config + project defaults.
	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    a.FormatReg,
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		ConfigureReader: func(reader format.DataFormatReader, detectedFmt registry.FormatID) error {
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
		},
	})

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
		a.writeTraceFile(tracePath, flowName, detectedFmt, inputPath, outputPath, recorder)
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

// writeTraceFile serializes a trace to JSON and writes it to disk.
func (a *App) writeTraceFile(tracePath, flowName, fmtName, inputPath, outputPath string, recorder *flow.TraceRecorder) {
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

	var traceNodes []flow.TraceNode
	traceNodes = append(traceNodes, flow.TraceNode{
		ID: "reader", Type: flow.NodeReader, Name: fmtName, Label: fmtName + " reader",
	})
	for _, e := range recorder.Events() {
		if e.Type == flow.TraceEnter && e.NodeID != "reader" && e.NodeID != "writer" {
			found := false
			for _, n := range traceNodes {
				if n.ID == e.NodeID {
					found = true
					break
				}
			}
			if !found {
				traceNodes = append(traceNodes, flow.TraceNode{
					ID: e.NodeID, Type: flow.NodeTool, Name: e.NodeID,
				})
			}
		}
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
			ext := filepath.Ext(inputPath)
			detected, err := a.FormatReg.DetectByExtension(ext)
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

	// Extract tool node names in topological order (by X position).
	type toolPos struct {
		name string
		x    float64
	}
	var toolNodes []toolPos
	for _, n := range flowDef.Nodes {
		if n.Type == flow.NodeTool {
			toolNodes = append(toolNodes, toolPos{name: n.Name, x: n.Position.X})
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
		t, toolCleanup, err := a.buildToolByName(tn.name, config, cmd...)
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

// buildToolByName creates tool(s) for a named tool, returning any resource
// cleanup function. Uses ToolInfo.Requires to drive resource setup (termbase,
// TM) rather than hardcoding tool names.
func (a *App) buildToolByName(toolName string, config map[string]any, cmd ...*cobra.Command) ([]tool.Tool, func(), error) {
	if a.ToolReg == nil || !a.ToolReg.Has(registry.ToolID(toolName)) {
		return nil, nil, fmt.Errorf("tool %q not found in registry", toolName)
	}

	info := a.ToolReg.GetToolInfo(registry.ToolID(toolName))

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
				// Tools requiring a TM get the provider injected from CLI flags.
				tmConfig := map[string]any{
					"source_locale":   a.SourceLang,
					"target_locale":   a.TargetLang,
					"fuzzy_threshold": 70,
				}
				var cleanup func()
				if len(cmd) > 0 && cmd[0] != nil {
					if tmName, _ := cmd[0].Flags().GetString("tm"); tmName != "" {
						var tmPath string
						if strings.ContainsAny(tmName, "/\\") || strings.HasSuffix(tmName, ".db") {
							tmPath = tmName
						} else {
							var err error
							tmPath, err = resolveNamedResource("tm", tmName)
							if err != nil {
								return nil, nil, fmt.Errorf("resolve TM %q: %w", tmName, err)
							}
						}
						tm, err := sqltm.NewSQLiteTM(tmPath)
						if err != nil {
							return nil, nil, fmt.Errorf("open TM %q: %w", tmName, err)
						}
						tmConfig["provider"] = &cliTMProvider{tm: tm}
						cleanup = func() { tm.Close() }
					}
				}
				t, err := a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), tmConfig, a.TargetLang)
				if err != nil {
					return nil, nil, err
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
				if info := a.ToolReg.GetToolInfo(registry.ToolID(n.Name)); info != nil && info.DefaultParallelBlocks > maxPB {
					maxPB = info.DefaultParallelBlocks
				}
			}
			return maxPB
		}
	}
	return 0
}

// cliTMProvider adapts a CLI SQLite TM to the tools.TMProvider interface.
type cliTMProvider struct {
	tm *sqltm.SQLiteTM
}

func (p *cliTMProvider) LookupExact(source string, sourceLocale, targetLocale model.LocaleID) (string, bool) {
	opts := sqltm.LookupOptions{
		MinScore:   1.0,
		MaxResults: 1,
		MatchModes: []sqltm.MatchMode{sqltm.MatchModePlain},
	}
	matches, err := p.tm.LookupText(source, sourceLocale, targetLocale, opts)
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return matches[0].Entry.VariantText(targetLocale), true
}

func (p *cliTMProvider) LookupFuzzy(source string, sourceLocale, targetLocale model.LocaleID, threshold int) (string, int, bool) {
	minScore := float64(threshold) / 100.0
	opts := sqltm.LookupOptions{
		MinScore:   minScore,
		MaxResults: 1,
	}
	matches, err := p.tm.LookupText(source, sourceLocale, targetLocale, opts)
	if err != nil || len(matches) == 0 {
		return "", 0, false
	}
	score := int(matches[0].Score * 100)
	return matches[0].Entry.VariantText(targetLocale), score, true
}

// openTermbase resolves the --termbase flag and opens a SQLite termbase.
// The flag value can be a named resource (no path separators) which resolves
// via KAPI_HOME, or an explicit file path.
// Returns (nil, noop, nil) if no --termbase flag was provided.
func (a *App) openTermbase(cmd ...*cobra.Command) (*sqltb.SQLiteTermBase, func(), error) {
	noop := func() {}
	if len(cmd) == 0 || cmd[0] == nil {
		return nil, noop, nil
	}
	tbValue, _ := cmd[0].Flags().GetString("termbase")
	if tbValue == "" {
		return nil, noop, nil
	}
	var tbPath string
	if strings.ContainsAny(tbValue, "/\\") || strings.HasSuffix(tbValue, ".db") {
		// Explicit file path.
		tbPath = tbValue
	} else {
		// Named resource.
		var err error
		tbPath, err = resolveNamedResource("termbases", tbValue)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve termbase %q: %w", tbValue, err)
		}
	}
	tb, err := sqltb.NewSQLiteTermBase(tbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open termbase %q: %w", tbValue, err)
	}
	return tb, func() { tb.Close() }, nil
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

	// Build tools from step definitions using the tool registry.
	var projectTools []tool.Tool
	for _, step := range spec.Steps {
		t, err := a.toolFromStep(step, cmd, rCtx)
		if err != nil {
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
		toolSchema := a.ToolReg.GetSchema(toolID)
		config := step.Config
		if rCtx != nil && toolSchema != nil {
			var err error
			config, err = flow.ResolveToolConfig(config, toolSchema, *rCtx)
			if err != nil {
				return nil, fmt.Errorf("resolve config for %q: %w", step.Tool, err)
			}
		}
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
