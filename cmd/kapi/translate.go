package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/asgeirf/gokapi/ai/provider"
	"github.com/asgeirf/gokapi/ai/tools"
	"github.com/asgeirf/gokapi/core/model"
	"github.com/spf13/cobra"
)

var translateCmd = &cobra.Command{
	Use:   "translate",
	Short: "Translate a file using AI",
	Long:  `Translate a file using an AI/LLM provider. Supports multiple providers.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inputPath, _ := cmd.Flags().GetString("input")
		outputPath, _ := cmd.Flags().GetString("output")
		providerName, _ := cmd.Flags().GetString("provider")
		apiKey, _ := cmd.Flags().GetString("api-key")
		modelName, _ := cmd.Flags().GetString("model")

		if inputPath == "" {
			return fmt.Errorf("--input (-i) is required")
		}
		if targetLang == "" {
			return fmt.Errorf("--target-lang is required")
		}

		ctx := context.Background()

		// Detect format
		fmtName := formatFlag
		if fmtName == "" {
			ext := filepath.Ext(inputPath)
			detected, err := formatReg.Detector().DetectByExtension(ext)
			if err != nil {
				return fmt.Errorf("unable to detect format: %w", err)
			}
			fmtName = detected
		}

		// Set output path
		if outputPath == "" {
			ext := filepath.Ext(inputPath)
			base := inputPath[:len(inputPath)-len(ext)]
			outputPath = fmt.Sprintf("%s_%s%s", base, targetLang, ext)
		}

		if !quiet {
			fmt.Printf("Translating %s → %s (provider: %s, %s → %s)\n",
				inputPath, outputPath, providerName, sourceLang, targetLang)
		}

		// Create provider
		p := createProvider(providerName, apiKey, modelName)

		// Read input
		reader, err := formatReg.NewReader(fmtName)
		if err != nil {
			return fmt.Errorf("no reader for format %q: %w", fmtName, err)
		}

		f, err := os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("open input: %w", err)
		}

		doc := &model.RawDocument{
			URI:          inputPath,
			SourceLocale: model.LocaleID(sourceLang),
			Encoding:     encoding,
			Reader:       f,
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

		// Translate blocks
		translateTool := tools.NewAITranslateTool(p, tools.AITranslateConfig{
			SourceLocale: model.LocaleID(sourceLang),
			TargetLocale: model.LocaleID(targetLang),
		})

		in := make(chan *model.Part, len(parts))
		out := make(chan *model.Part, len(parts))
		for _, p := range parts {
			in <- p
		}
		close(in)

		if err := translateTool.Process(ctx, in, out); err != nil {
			return fmt.Errorf("translation error: %w", err)
		}
		close(out)

		var translated []*model.Part
		for p := range out {
			translated = append(translated, p)
		}

		// Write output
		writer, err := formatReg.NewWriter(fmtName)
		if err != nil {
			return fmt.Errorf("no writer for format %q: %w", fmtName, err)
		}

		if err := writer.SetOutput(outputPath); err != nil {
			return fmt.Errorf("set output: %w", err)
		}
		writer.SetLocale(model.LocaleID(targetLang))

		ch := make(chan *model.Part, len(translated))
		for _, p := range translated {
			ch <- p
		}
		close(ch)

		if err := writer.Write(ctx, ch); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		writer.Close()

		if !quiet {
			blocks := 0
			for _, p := range translated {
				if p.Type == model.PartBlock {
					blocks++
				}
			}
			fmt.Printf("Translated %d block(s)\n", blocks)
		}

		return nil
	},
}

func init() {
	translateCmd.Flags().StringP("input", "i", "", "input file path")
	translateCmd.Flags().StringP("output", "o", "", "output file path")
	translateCmd.Flags().String("provider", "anthropic", "LLM provider (anthropic, openai, ollama)")
	translateCmd.Flags().String("api-key", "", "API key for LLM provider")
	translateCmd.Flags().String("model", "", "LLM model name")
	rootCmd.AddCommand(translateCmd)
}

func createProvider(name, apiKey, modelName string) provider.LLMProvider {
	cfg := provider.Config{
		APIKey: apiKey,
		Model:  modelName,
	}

	switch name {
	case "anthropic":
		return provider.NewAnthropicProvider(cfg)
	case "openai":
		return provider.NewOpenAIProvider(cfg)
	case "ollama":
		return provider.NewOllamaProvider(cfg)
	default:
		// Fall back to mock for unknown providers
		return provider.NewMockProvider()
	}
}
