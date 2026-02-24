package main

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
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/loader"
	pluginreg "github.com/gokapi/gokapi/core/plugin/registry"
	"github.com/gokapi/gokapi/core/preset"
	"github.com/gokapi/gokapi/core/tool"
	libtools "github.com/gokapi/gokapi/core/tools"
	"github.com/gokapi/gokapi/kapi/cmd/kapi/output"
	kapipreset "github.com/gokapi/gokapi/kapi/preset"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var flowCmd = &cobra.Command{
	Use:   "flow",
	Short: "Run processing flows",
}

var flowRunCmd = &cobra.Command{
	Use:   "run [flow-name]",
	Short: "Run a flow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		flowName := args[0]

		// Try to find .kapi/ project first
		proj, err := findProject()
		if err == nil {
			// Project-based flow execution (ADR-016)
			return runProjectFlow(cmd, proj, flowName, args)
		}

		// Built-in flow execution without a project

		inputPaths, _ := cmd.Flags().GetStringSlice("input")
		concurrency, _ := cmd.Flags().GetInt("concurrency")

		if len(inputPaths) == 0 {
			return fmt.Errorf("--input (-i) is required")
		}
		if targetLang == "" {
			if flowName == "pseudo-translate" {
				targetLang = "qps"
			} else {
				return fmt.Errorf("--target-lang is required")
			}
		}

		ctx := context.Background()

		// Single file: use the existing direct pipeline path.
		if len(inputPaths) == 1 {
			return runSingleFile(ctx, cmd, flowName, inputPaths[0])
		}

		// Multiple files: use parallel executor with tool factories.
		return runMultipleFiles(ctx, cmd, flowName, inputPaths, concurrency)
	},
}

// runSingleFile processes a single input file through the flow.
func runSingleFile(ctx context.Context, cmd *cobra.Command, flowName, inputPath string) error {
	fmtName := formatFlag
	if fmtName == "" {
		ext := filepath.Ext(inputPath)
		detected, err := formatReg.Detector().DetectByExtension(ext)
		if err != nil {
			return fmt.Errorf("unable to detect format: %w", err)
		}
		fmtName = detected
	}

	// Parse format reference for preset/version
	ref := pluginreg.ParseFormatRef(fmtName)
	resolvedFmtName := ref.Name

	// Resolve format config if preset or project overrides are specified
	var mergedConfig map[string]any
	if ref.IsPreset() || hasProjectOverrides(resolvedFmtName) {
		presetReg := pluginLoader.Presets()
		kapipreset.RegisterBuiltins(presetReg)

		resolver := preset.NewConfigResolver(presetReg, pluginLoader.Schemas())

		localPresets := getLocalPresets()

		var err error
		mergedConfig, err = resolver.ResolveFormatConfig(
			resolvedFmtName, ref.Preset, localPresets, nil,
		)
		if err != nil {
			return fmt.Errorf("resolve format config: %w", err)
		}
	}

	reader, err := formatReg.NewReader(resolvedFmtName)
	if err != nil {
		// Fall back to full fmtName (might be versioned)
		reader, err = formatReg.NewReader(fmtName)
		if err != nil {
			return fmt.Errorf("no reader for format %q: %w", fmtName, err)
		}
	}

	// Apply merged config if available
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

	doc := &model.RawDocument{
		URI:          inputPath,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     encoding,
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

	flowTools, err := buildFlowTools(flowName)
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
		outputPath = fmt.Sprintf("%s_%s%s", base, targetLang, ext)
	}

	writer, err := formatReg.NewWriter(resolvedFmtName)
	if err != nil {
		// Fall back to full fmtName (might be versioned)
		writer, err = formatReg.NewWriter(fmtName)
		if err != nil {
			return fmt.Errorf("no writer for format %q: %w", fmtName, err)
		}
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return fmt.Errorf("set output: %w", err)
	}

	if ocs, ok := writer.(loader.OriginalContentSetter); ok {
		ocs.SetOriginalContent(inputContent)
	}

	writer.SetLocale(model.LocaleID(targetLang))

	ch := make(chan *model.Part, len(outputParts))
	for _, p := range outputParts {
		ch <- p
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	writer.Close()

	if !quiet {
		return output.Print(cmd, output.FlowRunOutput{
			FlowName:   flowName,
			InputPath:  inputPath,
			OutputPath: outputPath,
		})
	}
	return nil
}

// runMultipleFiles processes multiple input files in parallel using tool factories.
func runMultipleFiles(ctx context.Context, cmd *cobra.Command, flowName string, inputPaths []string, concurrency int) error {
	// Build FlowItems for each input path.
	items := make([]*flow.FlowItem, len(inputPaths))
	for i, p := range inputPaths {
		items[i] = &flow.FlowItem{
			Input: &model.RawDocument{
				URI:          p,
				SourceLocale: model.LocaleID(sourceLang),
				Encoding:     encoding,
			},
			TargetLocale: model.LocaleID(targetLang),
		}
	}

	// Build flow with tool factories so each document gets a fresh tool chain.
	fb := flow.NewFlow(flowName)
	factories, err := buildFlowToolFactories(flowName)
	if err != nil {
		return err
	}
	for _, f := range factories {
		fb.AddToolFactory(f)
	}
	f2 := fb.Build()

	// Set up executor with concurrency.
	var opts []flow.ExecutorOption
	opts = append(opts, flow.WithMaxConcurrency(concurrency))

	executor := flow.NewFlowExecutor(opts...)

	if err := executor.Execute(ctx, f2, items); err != nil {
		return fmt.Errorf("flow execution error: %w", err)
	}

	if !quiet {
		return output.Print(cmd, output.FlowRunOutput{
			FlowName:       flowName,
			FilesProcessed: len(inputPaths),
		})
	}

	return nil
}

var flowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available flows",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Try to find .kapi/ project
		proj, err := findProject()
		if err == nil {
			// List flows from .kapi/flows/
			return listProjectFlows(cmd, proj)
		}

		// List built-in flows
		builtinFlows := []output.FlowInfo{
			{Name: "ai-translate", Description: "Translate content using AI/LLM"},
			{Name: "ai-translate-qa", Description: "Translate + quality check using AI/LLM"},
			{Name: "pseudo-translate", Description: "Generate pseudo-translations for testing"},
			{Name: "qa-check", Description: "Run rule-based quality checks on translations"},
			{Name: "tm-leverage", Description: "Pre-fill translations from translation memory"},
			{Name: "segmentation", Description: "Split source text into sentence segments"},
		}

		out := output.FlowsListOutput{
			Flows: builtinFlows,
			Total: len(builtinFlows),
		}
		return output.Print(cmd, out)
	},
}

func listProjectFlows(cmd *cobra.Command, proj *project.Project) error {
	// List YAML files in flows/ directory
	entries, err := os.ReadDir(proj.FlowsDirPath())
	if err != nil {
		return fmt.Errorf("reading flows directory: %w", err)
	}

	var flows []output.FlowInfo
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".yaml" {
			flowName := entry.Name()[:len(entry.Name())-5] // Remove .yaml extension
			flowPath := filepath.Join(proj.FlowsDirPath(), entry.Name())

			flowInfo := output.FlowInfo{
				Name: flowName,
				Path: flowPath,
			}

			// Try to load flow to get description
			flowDef, err := loadFlowDefinition(flowPath)
			if err == nil {
				flowInfo.Description = flowDef.Description
				flowInfo.Steps = len(flowDef.Steps)
			}

			flows = append(flows, flowInfo)
		}
	}

	out := output.FlowsListOutput{
		Flows: flows,
		Total: len(flows),
	}
	return output.Print(cmd, out)
}

func init() {
	addProcessingFlags(flowRunCmd)
	flowRunCmd.Flags().StringSliceP("input", "i", nil, "input file path(s); repeat for multiple files")
	flowRunCmd.Flags().StringP("output", "o", "", "output file path (single-file mode only)")
	flowRunCmd.Flags().IntP("concurrency", "j", 0, "number of files to process at once (0 = auto)")
	flowRunCmd.Flags().String("provider", "anthropic", "AI provider (anthropic, openai, ollama)")
	flowRunCmd.Flags().String("api-key", "", "API key for the AI provider")
	flowRunCmd.Flags().String("model", "", "AI model name")

	flowCmd.AddCommand(flowRunCmd)
	flowCmd.AddCommand(flowListCmd)
	rootCmd.AddCommand(flowCmd)
}

func buildFlowTools(flowName string) ([]tool.Tool, error) {
	switch flowName {
	case "ai-translate":
		p := getProvider()
		return []tool.Tool{
			tools.NewAITranslateTool(p, tools.AITranslateConfig{
				SourceLocale: model.LocaleID(sourceLang),
				TargetLocale: model.LocaleID(targetLang),
			}),
		}, nil
	case "ai-translate-qa":
		p := getProvider()
		return []tool.Tool{
			tools.NewAITranslateTool(p, tools.AITranslateConfig{
				SourceLocale: model.LocaleID(sourceLang),
				TargetLocale: model.LocaleID(targetLang),
			}),
			tools.NewAIQACheckTool(p, tools.AIQAConfig{
				SourceLocale: model.LocaleID(sourceLang),
				TargetLocale: model.LocaleID(targetLang),
			}),
		}, nil
	case "pseudo-translate":
		return []tool.Tool{
			libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
				TargetLocale: model.LocaleID(targetLang),
			}),
		}, nil
	case "qa-check":
		return []tool.Tool{
			libtools.NewQACheckTool(libtools.NewQACheckConfig(model.LocaleID(targetLang))),
		}, nil
	case "segmentation":
		return []tool.Tool{
			libtools.NewSegmentationTool(&libtools.SegmentationConfig{
				TargetLocale: model.LocaleID(targetLang),
			}),
		}, nil
	case "tm-leverage":
		return []tool.Tool{
			libtools.NewTMLeverageTool(&libtools.TMLeverageConfig{
				SourceLocale:   model.LocaleID(sourceLang),
				TargetLocale:   model.LocaleID(targetLang),
				FuzzyThreshold: 70,
				Provider:       libtools.NullTMProvider{},
			}),
		}, nil
	default:
		return nil, fmt.Errorf("unknown flow: %q", flowName)
	}
}

// buildFlowToolFactories returns ToolFactory functions for each tool in the named flow.
// Each factory creates a fresh instance so parallel documents don't share state.
func buildFlowToolFactories(flowName string) ([]flow.ToolFactory, error) {
	switch flowName {
	case "ai-translate":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				p := getProvider()
				return tools.NewAITranslateTool(p, tools.AITranslateConfig{
					SourceLocale: model.LocaleID(sourceLang),
					TargetLocale: model.LocaleID(targetLang),
				}), nil
			},
		}, nil
	case "ai-translate-qa":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				p := getProvider()
				return tools.NewAITranslateTool(p, tools.AITranslateConfig{
					SourceLocale: model.LocaleID(sourceLang),
					TargetLocale: model.LocaleID(targetLang),
				}), nil
			},
			func() (tool.Tool, error) {
				p := getProvider()
				return tools.NewAIQACheckTool(p, tools.AIQAConfig{
					SourceLocale: model.LocaleID(sourceLang),
					TargetLocale: model.LocaleID(targetLang),
				}), nil
			},
		}, nil
	case "pseudo-translate":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				return libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
					TargetLocale: model.LocaleID(targetLang),
				}), nil
			},
		}, nil
	case "qa-check":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				return libtools.NewQACheckTool(libtools.NewQACheckConfig(model.LocaleID(targetLang))), nil
			},
		}, nil
	case "segmentation":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				return libtools.NewSegmentationTool(&libtools.SegmentationConfig{
					TargetLocale: model.LocaleID(targetLang),
				}), nil
			},
		}, nil
	case "tm-leverage":
		return []flow.ToolFactory{
			func() (tool.Tool, error) {
				return libtools.NewTMLeverageTool(&libtools.TMLeverageConfig{
					SourceLocale:   model.LocaleID(sourceLang),
					TargetLocale:   model.LocaleID(targetLang),
					FuzzyThreshold: 70,
					Provider:       libtools.NullTMProvider{},
				}), nil
			},
		}, nil
	default:
		return nil, fmt.Errorf("unknown flow: %q", flowName)
	}
}

func getLocalPresets() map[string]preset.LocalFormatPreset {
	proj, err := findProject()
	if err != nil {
		return nil
	}
	if len(proj.Config.FormatPresets) == 0 {
		return nil
	}
	result := make(map[string]preset.LocalFormatPreset, len(proj.Config.FormatPresets))
	for name, lp := range proj.Config.FormatPresets {
		result[name] = preset.LocalFormatPreset{
			Description: lp.Description,
			Base:        lp.Base,
			Config:      lp.Config,
		}
	}
	return result
}

func hasProjectOverrides(formatName string) bool {
	proj, err := findProject()
	if err != nil {
		return false
	}
	_, ok := proj.Config.FormatPresets[formatName]
	return ok
}

func getProvider() provider.LLMProvider {
	// For now, return a mock provider. In production, this would
	// look up API key and provider from flags/config.
	return provider.NewMockProvider()
}
