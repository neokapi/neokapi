package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gokapi/gokapi/core/ai/provider"
	"github.com/gokapi/gokapi/core/ai/tools"
	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/loader"
	pluginreg "github.com/gokapi/gokapi/core/plugin/registry"
	"github.com/gokapi/gokapi/core/preset"
	"github.com/gokapi/gokapi/core/tool"
	libtools "github.com/gokapi/gokapi/core/tools"
	"github.com/gokapi/gokapi/cli/output"
	"github.com/spf13/cobra"
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
				ctx := context.Background()
				if len(inputPaths) == 1 {
					return a.runSingleFile(ctx, cmd, flowName, inputPaths[0])
				}
				return a.runMultipleFiles(ctx, cmd, flowName, inputPaths, concurrency)
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
	flowRunCmd.Flags().StringP("output", "o", "", "output file path (single-file mode only)")
	flowRunCmd.Flags().IntP("concurrency", "j", 0, "number of files to process at once (0 = auto)")
	flowRunCmd.Flags().String("provider", "anthropic", "AI provider (anthropic, openai, ollama)")
	flowRunCmd.Flags().String("api-key", "", "API key for the AI provider")
	flowRunCmd.Flags().String("model", "", "AI model name")

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
		resolver := preset.NewConfigResolver(presetReg, a.PluginLoader.Schemas())

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
	defer reader.Close()

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return fmt.Errorf("read error: %w", result.Error)
		}
		parts = append(parts, result.Part)
	}

	flowTools, err := a.buildFlowTools(flowName)
	if err != nil {
		return err
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

	if !a.Quiet {
		return output.Print(cmd, output.FlowRunOutput{
			FlowName:   flowName,
			InputPath:  inputPath,
			OutputPath: outputPath,
		})
	}
	return nil
}

func (a *App) runMultipleFiles(ctx context.Context, cmd *cobra.Command, flowName string, inputPaths []string, concurrency int) error {
	items := make([]*flow.FlowItem, len(inputPaths))
	for i, p := range inputPaths {
		items[i] = &flow.FlowItem{
			Input: &model.RawDocument{
				URI:          p,
				SourceLocale: model.LocaleID(a.SourceLang),
				Encoding:     a.Encoding,
			},
			TargetLocale: model.LocaleID(a.TargetLang),
		}
	}

	fb := flow.NewFlow(flowName)
	factories, err := a.buildFlowToolFactories(flowName)
	if err != nil {
		return err
	}
	for _, f := range factories {
		fb.AddToolFactory(f)
	}
	f2 := fb.Build()

	var opts []flow.ExecutorOption
	opts = append(opts, flow.WithMaxConcurrency(concurrency))
	executor := flow.NewFlowExecutor(opts...)

	if err := executor.Execute(ctx, f2, items); err != nil {
		return fmt.Errorf("flow execution error: %w", err)
	}

	if !a.Quiet {
		return output.Print(cmd, output.FlowRunOutput{
			FlowName:       flowName,
			FilesProcessed: len(inputPaths),
		})
	}
	return nil
}

func (a *App) buildFlowTools(flowName string) ([]tool.Tool, error) {
	switch flowName {
	case "ai-translate":
		p := a.getProvider()
		return []tool.Tool{
			tools.NewAITranslateTool(p, tools.AITranslateConfig{
				SourceLocale: model.LocaleID(a.SourceLang),
				TargetLocale: model.LocaleID(a.TargetLang),
			}),
		}, nil
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
		}, nil
	case "pseudo-translate":
		return []tool.Tool{
			libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
				TargetLocale: model.LocaleID(a.TargetLang),
			}),
		}, nil
	case "qa-check":
		return []tool.Tool{
			libtools.NewQACheckTool(libtools.NewQACheckConfig(model.LocaleID(a.TargetLang))),
		}, nil
	case "segmentation":
		return []tool.Tool{
			libtools.NewSegmentationTool(&libtools.SegmentationConfig{
				TargetLocale: model.LocaleID(a.TargetLang),
			}),
		}, nil
	case "tm-leverage":
		return []tool.Tool{
			libtools.NewTMLeverageTool(&libtools.TMLeverageConfig{
				SourceLocale:   model.LocaleID(a.SourceLang),
				TargetLocale:   model.LocaleID(a.TargetLang),
				FuzzyThreshold: 70,
				Provider:       libtools.NullTMProvider{},
			}),
		}, nil
	default:
		return nil, fmt.Errorf("unknown flow: %q", flowName)
	}
}

func (a *App) buildFlowToolFactories(flowName string) ([]flow.ToolFactory, error) {
	switch flowName {
	case "ai-translate":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				p := a.getProvider()
				return tools.NewAITranslateTool(p, tools.AITranslateConfig{
					SourceLocale: model.LocaleID(a.SourceLang),
					TargetLocale: model.LocaleID(a.TargetLang),
				}), nil
			},
		}, nil
	case "ai-translate-qa":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				p := a.getProvider()
				return tools.NewAITranslateTool(p, tools.AITranslateConfig{
					SourceLocale: model.LocaleID(a.SourceLang),
					TargetLocale: model.LocaleID(a.TargetLang),
				}), nil
			},
			func() (tool.Tool, error) {
				p := a.getProvider()
				return tools.NewAIQACheckTool(p, tools.AIQAConfig{
					SourceLocale: model.LocaleID(a.SourceLang),
					TargetLocale: model.LocaleID(a.TargetLang),
				}), nil
			},
		}, nil
	case "pseudo-translate":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				return libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
					TargetLocale: model.LocaleID(a.TargetLang),
				}), nil
			},
		}, nil
	case "qa-check":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				return libtools.NewQACheckTool(libtools.NewQACheckConfig(model.LocaleID(a.TargetLang))), nil
			},
		}, nil
	case "segmentation":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				return libtools.NewSegmentationTool(&libtools.SegmentationConfig{
					TargetLocale: model.LocaleID(a.TargetLang),
				}), nil
			},
		}, nil
	case "tm-leverage":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				return libtools.NewTMLeverageTool(&libtools.TMLeverageConfig{
					SourceLocale:   model.LocaleID(a.SourceLang),
					TargetLocale:   model.LocaleID(a.TargetLang),
					FuzzyThreshold: 70,
					Provider:       libtools.NullTMProvider{},
				}), nil
			},
		}, nil
	default:
		return nil, fmt.Errorf("unknown flow: %q", flowName)
	}
}

func (a *App) getProvider() provider.LLMProvider {
	return provider.NewMockProvider()
}
