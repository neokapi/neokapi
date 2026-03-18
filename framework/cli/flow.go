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

// FlowCmdOptions configures the flow command.
type FlowCmdOptions struct {
	// FallbackRunE is called when --input is not provided.
	// If nil, --input is required.
	FallbackRunE func(cmd *cobra.Command, flowName string, args []string) error

	// ExtraFlows returns additional flows for the list command (e.g. project flows).
	ExtraFlows func() []output.FlowInfo
}

// NewFlowCmd creates the flow command group (flow run, flow list).
func (a *App) NewFlowCmd(opts FlowCmdOptions) *cobra.Command {
	flowCmd := &cobra.Command{
		Use:   "flow",
		Short: "Run processing flows",
	}

	flowRunCmd := &cobra.Command{
		Use:   "run [flow-name]",
		Short: "Run a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowName := args[0]
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
				ctx := context.Background()
				if len(inputPaths) == 1 {
					return a.runSingleFile(ctx, cmd, flowName, inputPaths[0])
				}
				return a.runMultipleFiles(ctx, cmd, flowName, inputPaths, concurrency, outputFlag)
			}

			// No --input: try fallback (e.g. project flow).
			if opts.FallbackRunE != nil {
				return opts.FallbackRunE(cmd, flowName, args)
			}
			return fmt.Errorf("--input (-i) is required")
		},
	}

	a.AddProcessingFlags(flowRunCmd)
	flowRunCmd.Flags().StringSliceP("input", "i", nil, "input file path(s); repeat for multiple files")
	flowRunCmd.Flags().StringP("output", "o", "", "output path or template (e.g. ./out/{name}_{lang}.{ext})")
	flowRunCmd.Flags().IntP("concurrency", "j", 0, "number of files to process at once (0 = auto)")
	flowRunCmd.Flags().String("provider", "anthropic", "AI provider (anthropic, openai, ollama)")
	flowRunCmd.Flags().String("api-key", "", "API key for the AI provider")
	flowRunCmd.Flags().String("model", "", "AI model name")
	flowRunCmd.Flags().String("trace", "", "write flow trace JSON to file (for flow visualization)")
	flowRunCmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
	flowRunCmd.Flags().String("tm", "", "named TM for tm-leverage flow (resolves from KAPI_HOME)")
	flowRunCmd.Flags().String("termbase", "", "named termbase for term-lookup/enforce (resolves from KAPI_HOME)")
	flowRunCmd.Flags().Bool("stats", false, "include part/block counts in output")

	flowListCmd := &cobra.Command{
		Use:   "list",
		Short: "List available flows",
		RunE: func(cmd *cobra.Command, args []string) error {
			builtinFlows := []output.FlowInfo{
				{Name: "ai-translate", Description: "Translate content using AI/LLM"},
				{Name: "ai-translate-qa", Description: "Translate + quality check using AI/LLM"},
				{Name: "pseudo-translate", Description: "Generate pseudo-translations for testing"},
				{Name: "qa-check", Description: "Run rule-based quality checks on translations"},
				{Name: "tm-leverage", Description: "Pre-fill translations from translation memory"},
				{Name: "segmentation", Description: "Split source text into sentence segments"},
			}

			if opts.ExtraFlows != nil {
				builtinFlows = append(builtinFlows, opts.ExtraFlows()...)
			}

			out := output.FlowsListOutput{
				Flows: builtinFlows,
				Total: len(builtinFlows),
			}
			return output.Print(cmd, out)
		},
	}

	flowCmd.AddCommand(flowRunCmd)
	flowCmd.AddCommand(flowListCmd)
	return flowCmd
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
		return a.processFlowFileBridge(ctx, cmd, flowName, inputPath, outputFlag, bridgeReader)
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

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	var mu sync.Mutex
	var processed int

	for _, inputPath := range inputPaths {
		g.Go(func() error {
			if err := a.processFlowFile(ctx, cmd, flowName, inputPath, outputTemplate); err != nil {
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
//
// For bridge formats, uses a single-pass pipeline via BridgeProcessor where Go
// acts as an Okapi pipeline step — one read, inline writing, no document re-read.
// For native formats, uses the standard read → process → write pipeline.
func (a *App) processFlowFile(ctx context.Context, cmd *cobra.Command, flowName, inputPath, outputTemplate string) error {
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

	// Bridge formats: single-pass pipeline via BridgeProcessor.
	if bridgeReader, ok := reader.(*bridge.BridgeFormatReader); ok {
		return a.processFlowFileBridge(ctx, cmd, flowName, inputPath, outputTemplate, bridgeReader)
	}

	// Native formats: standard read → process → write pipeline.
	return a.processFlowFileNative(ctx, cmd, flowName, inputPath, outputTemplate, registryName, reader, mergedConfig)
}

// processFlowFileBridge runs a single-pass Okapi pipeline where Go acts as a
// step. Java reads each event, sends the part to Go, Go processes it and sends
// it back, Java applies the translation and writes — all in one filter iteration.
func (a *App) processFlowFileBridge(ctx context.Context, cmd *cobra.Command,
	flowName, inputPath, outputTemplate string, bridgeReader *bridge.BridgeFormatReader) error {

	outputPath := a.resolveOutputPath(inputPath, outputTemplate)

	flowTools, cleanup, err := a.buildFlowTools(flowName, cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	// Auto-wrap IO-bound tools with ParallelBlockTool.
	if n := defaultParallelBlocks(flowName); n > 1 {
		for i, ft := range flowTools {
			flowTools[i] = tool.NewParallelBlockTool(ft, n)
		}
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
	return err
}

// processFlowFileNative uses the standard read → process → write pipeline.
func (a *App) processFlowFileNative(ctx context.Context, cmd *cobra.Command, flowName, inputPath, outputTemplate, registryName string, reader format.DataFormatReader, mergedConfig map[string]any) error {
	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	writer, err := a.FormatReg.NewWriter(registryName)
	if err != nil {
		return fmt.Errorf("no writer for format %q: %w", registryName, err)
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
	reader.Close()

	// Build fresh tool instances for this file (thread-safe).
	flowTools, cleanup, err := a.buildFlowTools(flowName, cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	// Auto-wrap IO-bound tools with ParallelBlockTool.
	if n := defaultParallelBlocks(flowName); n > 1 {
		for i, ft := range flowTools {
			flowTools[i] = tool.NewParallelBlockTool(ft, n)
		}
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
		return fmt.Errorf("flow execution error: %w", err)
	}

	outputPath := a.resolveOutputPath(inputPath, outputTemplate)

	if err := writer.SetOutput(outputPath); err != nil {
		return fmt.Errorf("set output: %w", err)
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
		return fmt.Errorf("write output: %w", err)
	}
	writer.Close()

	return nil
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
