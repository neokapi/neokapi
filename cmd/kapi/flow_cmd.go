package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/asgeirf/gokapi/ai/provider"
	"github.com/asgeirf/gokapi/ai/tools"
	"github.com/asgeirf/gokapi/core/flow"
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
	"github.com/asgeirf/gokapi/plugin/loader"
	"github.com/spf13/cobra"
)

var flowCmd = &cobra.Command{
	Use:   "flow",
	Short: "Manage and execute flows",
}

var flowRunCmd = &cobra.Command{
	Use:   "run [flow-name]",
	Short: "Execute a named flow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		flowName := args[0]
		inputPath, _ := cmd.Flags().GetString("input")

		if inputPath == "" {
			return fmt.Errorf("--input (-i) is required")
		}
		if targetLang == "" {
			return fmt.Errorf("--target-lang is required")
		}

		ctx := context.Background()

		// Detect input format
		fmtName := formatFlag
		if fmtName == "" {
			ext := filepath.Ext(inputPath)
			detected, err := formatReg.Detector().DetectByExtension(ext)
			if err != nil {
				return fmt.Errorf("unable to detect format: %w", err)
			}
			fmtName = detected
		}

		// Read input
		reader, err := formatReg.NewReader(fmtName)
		if err != nil {
			return fmt.Errorf("no reader for format %q: %w", fmtName, err)
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

		// Build flow from name
		flowTools, err := buildFlowTools(flowName)
		if err != nil {
			return err
		}

		fb := flow.NewFlow(flowName)
		for _, t := range flowTools {
			fb.AddTool(t)
		}
		f2 := fb.Build()

		// Execute flow
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

		// Write output
		outputPath, _ := cmd.Flags().GetString("output")
		if outputPath == "" {
			ext := filepath.Ext(inputPath)
			base := inputPath[:len(inputPath)-len(ext)]
			outputPath = fmt.Sprintf("%s_%s%s", base, targetLang, ext)
		}

		writer, err := formatReg.NewWriter(fmtName)
		if err != nil {
			return fmt.Errorf("no writer for format %q: %w", fmtName, err)
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
			fmt.Printf("Flow %q completed: %s → %s\n", flowName, inputPath, outputPath)
		}
		return nil
	},
}

var flowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available flows",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Available flows:")
		fmt.Println()
		fmt.Printf("  %-25s %s\n", "FLOW", "DESCRIPTION")
		fmt.Printf("  %-25s %s\n", "----", "-----------")
		fmt.Printf("  %-25s %s\n", "ai-translate", "Translate content using AI/LLM")
		fmt.Printf("  %-25s %s\n", "ai-translate-qa", "Translate + quality check using AI/LLM")
		fmt.Printf("  %-25s %s\n", "pseudo-translate", "Generate pseudo-translations for testing")
	},
}

func init() {
	flowRunCmd.Flags().StringP("input", "i", "", "input file path")
	flowRunCmd.Flags().StringP("output", "o", "", "output file path")
	flowRunCmd.Flags().String("provider", "anthropic", "LLM provider (anthropic, openai, ollama)")
	flowRunCmd.Flags().String("api-key", "", "API key for LLM provider")
	flowRunCmd.Flags().String("model", "", "LLM model name")

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
			newPseudoTranslateTool(),
		}, nil
	default:
		return nil, fmt.Errorf("unknown flow: %q", flowName)
	}
}

func getProvider() provider.LLMProvider {
	// For now, return a mock provider. In production, this would
	// look up API key and provider from flags/config.
	return provider.NewMockProvider()
}

func newPseudoTranslateTool() tool.Tool {
	t := &tool.BaseTool{
		ToolName:        "pseudo-translate",
		ToolDescription: "Generates pseudo-translations for testing",
	}
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok || !block.Translatable {
			return part, nil
		}
		pseudo := "[" + block.SourceText() + "]"
		block.SetTargetText(model.LocaleID(targetLang), pseudo)
		return part, nil
	}
	return t
}
