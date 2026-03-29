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
	"sync"
	"time"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/neokapi/neokapi/core/plugin/loader"
	pluginreg "github.com/neokapi/neokapi/core/plugin/registry"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/providers/ai"
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
			if flowName == "pseudo-translate" {
				a.TargetLang = "qps"
			} else {
				return fmt.Errorf("--target-lang is required")
			}
		}
		outputFlag, _ := cmd.Flags().GetString("output")
		if len(inputPaths) == 1 {
			return a.runSingleFile(ctx, cmd, flowName, inputPaths[0])
		}
		return a.runMultipleFiles(ctx, cmd, flowName, inputPaths, concurrency, outputFlag)
	}

	// No --input: try fallback (e.g. project flow).
	if opts.FallbackRunE != nil {
		return opts.FallbackRunE(cmd, flowName, []string{flowName})
	}
	return fmt.Errorf("--input (-i) is required")
}

// addFlowRunFlags registers the common flags for flow execution commands.
func (a *App) addFlowRunFlags(cmd *cobra.Command) {
	a.AddProcessingFlags(cmd)
	cmd.Flags().StringSliceP("input", "i", nil, "input file path(s); repeat for multiple files")
	cmd.Flags().StringP("output", "o", "", "output path or template (e.g. ./out/{name}_{lang}.{ext})")
	cmd.Flags().IntP("concurrency", "j", 0, "number of files to process at once (0 = auto)")
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

	if opts.ExtraFlows != nil {
		flows = append(flows, opts.ExtraFlows()...)
	}

	out := output.FlowsListOutput{
		Flows: flows,
		Total: len(flows),
	}
	return output.Print(cmd, out)
}

// builtinComposedFlows returns the list of built-in composed flows
// (multi-tool pipelines). Single-tool operations are exposed as top-level
// tool commands instead.
func builtinComposedFlows() []output.FlowInfo {
	return []output.FlowInfo{
		{Name: "ai-translate-qa", Description: "Translate + quality check using AI/LLM"},
	}
}

func (a *App) runSingleFile(ctx context.Context, cmd *cobra.Command, flowName, inputPath string) error {
	fmtName := a.FormatFlag
	if fmtName == "" {
		ext := filepath.Ext(inputPath)
		detected, err := a.FormatReg.DetectByExtension(ext)
		if err != nil {
			return fmt.Errorf("unable to detect format: %w", err)
		}
		fmtName = detected
	}

	ref := pluginreg.ParseFormatRef(fmtName)
	registryName := ref.RegistryName()

	var mergedConfig map[string]any
	if ref.IsPreset() {
		presetReg := a.PluginLoader.Presets()
		preset.RegisterBuiltins(presetReg)
		resolver := preset.NewConfigResolver(presetReg, a.SchemaReg)

		var err error
		mergedConfig, err = resolver.ResolveFormatConfig(ref.Name, ref.Preset, nil, nil)
		if err != nil {
			return fmt.Errorf("resolve format config: %w", err)
		}
	}

	reader, err := a.FormatReg.NewReader(registryName)
	if err != nil {
		return fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}

	if len(mergedConfig) > 0 {
		if cfg := reader.Config(); cfg != nil {
			if err := cfg.ApplyMap(mergedConfig); err != nil {
				return fmt.Errorf("apply format config: %w", err)
			}
		}
	}

	// Bridge formats: use single-pass pipeline via BridgeProcessor.
	if bridgeReader, ok := reader.(*bridge.BridgeFormatReader); ok {
		outputFlag, _ := cmd.Flags().GetString("output")
		_, err := a.processFlowFileBridge(ctx, cmd, flowName, inputPath, outputFlag, bridgeReader, nil)
		return err
	}

	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	// Create writer early so we can wire skeleton store before reading.
	writer, err := a.FormatReg.NewWriter(registryName)
	if err != nil {
		return fmt.Errorf("no writer for format %q: %w", fmtName, err)
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
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(inputContent)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		return fmt.Errorf("open document: %w", err)
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			return fmt.Errorf("read error: %w", result.Error)
		}
		parts = append(parts, result.Part)
	}

	// Close the reader immediately after reading all parts. For bridge formats
	// this releases the JVM back to the pool so the writer can reuse it,
	// avoiding a second JVM startup.
	reader.Close()

	flowTools, cleanup, err := a.buildFlowTools(flowName, cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	// Wrap IO-bound tools with ParallelBlockTool.
	// AI flows default to 5 concurrent blocks; --parallel-blocks overrides.
	parallelBlocks, _ := cmd.Flags().GetInt("parallel-blocks")
	if parallelBlocks == 0 {
		parallelBlocks = defaultParallelBlocks(flowName)
	}
	if parallelBlocks > 1 {
		for i, ft := range flowTools {
			flowTools[i] = tool.NewParallelBlockTool(ft, parallelBlocks)
		}
	}

	// If --trace is set, wrap tools with TracingTool to record events.
	tracePath, _ := cmd.Flags().GetString("trace")
	var recorder *flow.TraceRecorder
	var traceNodes []flow.TraceNode
	if tracePath != "" {
		recorder = flow.NewTraceRecorder()
		traceNodes = append(traceNodes, flow.TraceNode{
			ID: "reader", Type: "reader", Name: fmtName, Label: fmtName + " reader",
		})
		for i, t := range flowTools {
			nodeID := fmt.Sprintf("tool-%d", i)
			traceNodes = append(traceNodes, flow.TraceNode{
				ID: nodeID, Type: "tool", Name: t.Name(), Label: t.Name(),
			})
			flowTools[i] = flow.NewTracingTool(t, nodeID, recorder)
		}
		traceNodes = append(traceNodes, flow.TraceNode{
			ID: "writer", Type: "writer", Name: fmtName, Label: fmtName + " writer",
		})
	}

	fb := flow.NewFlow(flowName)
	for _, t := range flowTools {
		fb.AddTool(t)
	}
	f2 := fb.Build()

	executor := flow.NewFlowExecutor()
	inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f2)

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
		return fmt.Errorf("flow execution error: %w", err)
	}

	outputPath, _ := cmd.Flags().GetString("output")
	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		base := inputPath[:len(inputPath)-len(ext)]
		outputPath = fmt.Sprintf("%s_%s%s", base, a.TargetLang, ext)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return fmt.Errorf("set output: %w", err)
	}

	// Prefer passing the file path over loading content bytes when the writer
	// supports it. This avoids duplicating the file in memory for gRPC transfer.
	if sps, ok := writer.(loader.SourcePathSetter); ok && filepath.IsAbs(inputPath) {
		sps.SetSourcePath(inputPath)
	} else if ocs, ok := writer.(loader.OriginalContentSetter); ok {
		ocs.SetOriginalContent(inputContent)
	}

	writer.SetLocale(model.LocaleID(a.TargetLang))

	ch := make(chan *model.Part, len(outputParts))
	for _, p := range outputParts {
		ch <- p
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	writer.Close()

	// Write trace JSON if --trace was set.
	if tracePath != "" && recorder != nil {
		inputPreview := string(inputContent)
		if len(inputPreview) > 2000 {
			inputPreview = inputPreview[:2000] + "\n... (truncated)"
		}
		outputData, _ := os.ReadFile(outputPath)
		outputPreview := string(outputData)
		if len(outputPreview) > 2000 {
			outputPreview = outputPreview[:2000] + "\n... (truncated)"
		}

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
			return fmt.Errorf("create trace dir: %w", err)
		}
		if err := os.WriteFile(tracePath, traceJSON, 0o644); err != nil {
			return fmt.Errorf("write trace: %w", err)
		}
	}

	if !a.Quiet {
		out := output.FlowRunOutput{
			FlowName:   flowName,
			InputPath:  inputPath,
			OutputPath: outputPath,
		}
		if showStats, _ := cmd.Flags().GetBool("stats"); showStats {
			out.Stats = countStats(outputParts)
		}
		return output.Print(cmd, out)
	}
	return nil
}

func (a *App) runMultipleFiles(ctx context.Context, cmd *cobra.Command, flowName string, inputPaths []string, concurrency int, outputTemplate string) error {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	// Pre-warm bridge JVMs so they're ready when files arrive.
	// This amortizes the ~1.3s JVM startup cost before the errgroup starts.
	if a.PluginLoader != nil {
		a.PluginLoader.WarmupBridges()
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
		for i := 0; i < concurrency; i++ {
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
// For bridge formats, uses a single-pass pipeline via BridgeProcessor where Go
// acts as an Okapi pipeline step — one read, inline writing, no document re-read.
// For native formats, uses the standard read → process → write pipeline.
func (a *App) processFlowFile(ctx context.Context, cmd *cobra.Command, flowName, inputPath, outputTemplate string, recorder *flow.TraceRecorder) (string, []flow.TraceNode, error) {
	fmtName := a.FormatFlag
	if fmtName == "" {
		ext := filepath.Ext(inputPath)
		detected, err := a.FormatReg.DetectByExtension(ext)
		if err != nil {
			return "", nil, fmt.Errorf("unable to detect format: %w", err)
		}
		fmtName = detected
	}

	ref := pluginreg.ParseFormatRef(fmtName)
	registryName := ref.RegistryName()

	var mergedConfig map[string]any
	if ref.IsPreset() {
		presetReg := a.PluginLoader.Presets()
		preset.RegisterBuiltins(presetReg)
		resolver := preset.NewConfigResolver(presetReg, a.SchemaReg)

		var err error
		mergedConfig, err = resolver.ResolveFormatConfig(ref.Name, ref.Preset, nil, nil)
		if err != nil {
			return "", nil, fmt.Errorf("resolve format config: %w", err)
		}
	}

	reader, err := a.FormatReg.NewReader(registryName)
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

	// Bridge formats: single-pass pipeline via BridgeProcessor.
	if bridgeReader, ok := reader.(*bridge.BridgeFormatReader); ok {
		nodes, err := a.processFlowFileBridge(ctx, cmd, flowName, inputPath, outputTemplate, bridgeReader, recorder)
		return fmtName, nodes, err
	}

	// Native formats: standard read → process → write pipeline.
	nodes, err := a.processFlowFileNative(ctx, cmd, flowName, inputPath, outputTemplate, registryName, reader, mergedConfig, recorder)
	return fmtName, nodes, err
}

// processFlowFileBridge runs a single-pass Okapi pipeline where Go acts as a
// step. Java reads each event, sends the part to Go, Go processes it and sends
// it back, Java applies the translation and writes — all in one filter iteration.
// When recorder is non-nil, tools are wrapped with TracingTool.
func (a *App) processFlowFileBridge(ctx context.Context, cmd *cobra.Command,
	flowName, inputPath, outputTemplate string, bridgeReader *bridge.BridgeFormatReader, recorder *flow.TraceRecorder) ([]flow.TraceNode, error) {

	outputPath := a.resolveOutputPath(inputPath, outputTemplate)

	flowTools, cleanup, err := a.buildFlowTools(flowName, cmd)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Auto-wrap IO-bound tools with ParallelBlockTool.
	if n := defaultParallelBlocks(flowName); n > 1 {
		for i, ft := range flowTools {
			flowTools[i] = tool.NewParallelBlockTool(ft, n)
		}
	}

	// Wrap tools with TracingTool if recorder is set.
	var traceNodes []flow.TraceNode
	if recorder != nil {
		traceNodes = append(traceNodes, flow.TraceNode{
			ID: "bridge-reader", Type: "reader", Name: "bridge", Label: "bridge reader",
		})
		for i, t := range flowTools {
			nodeID := fmt.Sprintf("tool-%d", i)
			traceNodes = append(traceNodes, flow.TraceNode{
				ID: nodeID, Type: "tool", Name: t.Name(), Label: t.Name(),
			})
			flowTools[i] = flow.NewTracingTool(t, nodeID, recorder)
		}
		traceNodes = append(traceNodes, flow.TraceNode{
			ID: "bridge-writer", Type: "writer", Name: "bridge", Label: "bridge writer",
		})
	}

	processor := bridgeReader.NewProcessor()
	_, err = processor.Execute(ctx, bridge.ProcessExecuteParams{
		InputPath:      inputPath,
		SourceLocale:   a.SourceLang,
		TargetLocale:   a.TargetLang,
		OutputPath:     outputPath,
		OutputLocale:   a.TargetLang,
		Encoding:       a.Encoding,
		SubscribeParts: []int32{int32(model.PartBlock)}, // Only send Blocks to Go
	}, func(parts <-chan *model.Part) <-chan *model.Part {
		fb := flow.NewFlow(flowName)
		for _, t := range flowTools {
			fb.AddTool(t)
		}
		f := fb.Build()

		executor := flow.NewFlowExecutor()
		inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f)

		go func() {
			for p := range parts {
				if recorder != nil && p.Resource != nil {
					recorder.Record("exit", "bridge-reader", p.Resource.ResourceID(), nil)
				}
				inCh <- p
			}
			close(inCh)
		}()

		// Wait for flow completion in a goroutine so outCh can drain.
		go func() {
			_ = wait()
		}()

		return outCh
	})
	return traceNodes, err
}

// processFlowFileNative uses the standard read → process → write pipeline.
// When recorder is non-nil, tools are wrapped with TracingTool and reader/writer
// events are recorded. Returns trace nodes (nil when recorder is nil).
func (a *App) processFlowFileNative(ctx context.Context, cmd *cobra.Command, flowName, inputPath, outputTemplate, registryName string, reader format.DataFormatReader, mergedConfig map[string]any, recorder *flow.TraceRecorder) ([]flow.TraceNode, error) {
	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}

	writer, err := a.FormatReg.NewWriter(registryName)
	if err != nil {
		return nil, fmt.Errorf("no writer for format %q: %w", registryName, err)
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
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(inputContent)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("open document: %w", err)
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			return nil, fmt.Errorf("read error: %w", result.Error)
		}
		parts = append(parts, result.Part)
	}
	reader.Close()

	// Build fresh tool instances for this file (thread-safe).
	flowTools, cleanup, err := a.buildFlowTools(flowName, cmd)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Auto-wrap IO-bound tools with ParallelBlockTool.
	if n := defaultParallelBlocks(flowName); n > 1 {
		for i, ft := range flowTools {
			flowTools[i] = tool.NewParallelBlockTool(ft, n)
		}
	}

	// Wrap tools with TracingTool if recorder is set.
	var traceNodes []flow.TraceNode
	if recorder != nil {
		traceNodes = append(traceNodes, flow.TraceNode{
			ID: "reader", Type: "reader", Name: registryName, Label: registryName + " reader",
		})
		for i, t := range flowTools {
			nodeID := fmt.Sprintf("tool-%d", i)
			traceNodes = append(traceNodes, flow.TraceNode{
				ID: nodeID, Type: "tool", Name: t.Name(), Label: t.Name(),
			})
			flowTools[i] = flow.NewTracingTool(t, nodeID, recorder)
		}
		traceNodes = append(traceNodes, flow.TraceNode{
			ID: "writer", Type: "writer", Name: registryName, Label: registryName + " writer",
		})
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
		return traceNodes, fmt.Errorf("flow execution error: %w", err)
	}

	outputPath := a.resolveOutputPath(inputPath, outputTemplate)

	if err := writer.SetOutput(outputPath); err != nil {
		return traceNodes, fmt.Errorf("set output: %w", err)
	}

	if sps, ok := writer.(loader.SourcePathSetter); ok && filepath.IsAbs(inputPath) {
		sps.SetSourcePath(inputPath)
	} else if ocs, ok := writer.(loader.OriginalContentSetter); ok {
		ocs.SetOriginalContent(inputContent)
	}

	writer.SetLocale(model.LocaleID(a.TargetLang))

	ch := make(chan *model.Part, len(outputParts))
	for _, p := range outputParts {
		ch <- p
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		return traceNodes, fmt.Errorf("write output: %w", err)
	}
	writer.Close()

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

	switch flowName {
	case "ai-translate":
		p := a.getProvider()
		return []tool.Tool{
			tools.NewAITranslateTool(p, tools.AITranslateConfig{
				SourceLocale: model.LocaleID(a.SourceLang),
				TargetLocale: model.LocaleID(a.TargetLang),
			}),
		}, noop, nil
	case "ai-translate-qa":
		p := a.getProvider()
		return []tool.Tool{
			tools.NewAITranslateTool(p, tools.AITranslateConfig{
				SourceLocale: model.LocaleID(a.SourceLang),
				TargetLocale: model.LocaleID(a.TargetLang),
			}),
			tools.NewAIQACheckTool(p, tools.AIQAConfig{
				SourceLocale: model.LocaleID(a.SourceLang),
				TargetLocale: model.LocaleID(a.TargetLang),
			}),
		}, noop, nil
	case "pseudo-translate":
		return []tool.Tool{
			libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
				TargetLocale: model.LocaleID(a.TargetLang),
			}),
		}, noop, nil
	case "qa-check":
		qaTools := []tool.Tool{
			libtools.NewQACheckTool(libtools.NewQACheckConfig(model.LocaleID(a.TargetLang))),
		}
		cleanup := noop
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
	case "segmentation":
		return []tool.Tool{
			libtools.NewSegmentationTool(&libtools.SegmentationConfig{
				TargetLocale: model.LocaleID(a.TargetLang),
			}),
		}, noop, nil
	case "tm-leverage":
		var tmProvider libtools.TMProvider = libtools.NullTMProvider{}
		cleanup := noop
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
				sqltm, err := sqltm.NewSQLiteTM(tmPath)
				if err != nil {
					return nil, nil, fmt.Errorf("open TM %q: %w", tmName, err)
				}
				tmProvider = &cliTMProvider{tm: sqltm}
				cleanup = func() { sqltm.Close() }
			}
		}
		return []tool.Tool{
			libtools.NewTMLeverageTool(&libtools.TMLeverageConfig{
				SourceLocale:   model.LocaleID(a.SourceLang),
				TargetLocale:   model.LocaleID(a.TargetLang),
				FuzzyThreshold: 70,
				Provider:       tmProvider,
			}),
		}, cleanup, nil
	default:
		return nil, nil, fmt.Errorf("unknown flow: %q", flowName)
	}
}

func (a *App) getProvider() provider.LLMProvider {
	return provider.NewMockProvider()
}

// countStats counts parts and blocks from the output parts slice.
func countStats(parts []*model.Part) *output.FlowStats {
	stats := &output.FlowStats{PartCount: len(parts)}
	for _, p := range parts {
		if p.Type == model.PartBlock {
			stats.BlockCount++
		}
	}
	return stats
}

// defaultParallelBlocks returns the default parallel block concurrency for
// IO-bound flows. Returns 0 (disabled) for CPU-bound flows.
func defaultParallelBlocks(flowName string) int {
	switch flowName {
	case "ai-translate", "ai-translate-qa":
		return 5
	default:
		return 0
	}
}

// cliTMProvider adapts a CLI SQLite TM to the libtools.TMProvider interface.
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
	return matches[0].Entry.TargetText(), true
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
	return matches[0].Entry.TargetText(), score, true
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
func (a *App) runProjectSteps(ctx context.Context, cmd *cobra.Command, flowName string, spec *flow.StepsSpec) error {
	inputPaths, _ := cmd.Flags().GetStringSlice("input")
	concurrency, _ := cmd.Flags().GetInt("concurrency")

	if a.TargetLang == "" {
		return fmt.Errorf("--target-lang is required")
	}

	// Build tools from step definitions using the tool registry + BuiltinToolCommands.
	var projectTools []tool.Tool
	for _, step := range spec.Steps {
		t, err := a.toolFromStep(step, cmd)
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
// It first checks BuiltinToolCommands for NewToolFromConfig support,
// then falls back to the tool registry.
func (a *App) toolFromStep(step flow.FlowStep, cmd *cobra.Command) (tool.Tool, error) {
	// Check if a BuiltinToolCommand has NewToolFromConfig for this tool.
	for _, def := range BuiltinToolCommands {
		if def.Use == step.Tool {
			if def.NewToolFromConfig != nil {
				return def.NewToolFromConfig(step.Config, a.TargetLang)
			}
			if def.NewTool != nil {
				return def.NewTool(cmd, a.TargetLang)
			}
		}
		// Check aliases too.
		for _, alias := range def.Aliases {
			if alias == step.Tool {
				if def.NewToolFromConfig != nil {
					return def.NewToolFromConfig(step.Config, a.TargetLang)
				}
				if def.NewTool != nil {
					return def.NewTool(cmd, a.TargetLang)
				}
			}
		}
	}

	// Fall back to tool registry (handles plugin tools).
	t, err := a.ToolReg.NewTool(step.Tool)
	if err != nil {
		return nil, fmt.Errorf("tool %q: %w", step.Tool, err)
	}
	return t, nil
}
