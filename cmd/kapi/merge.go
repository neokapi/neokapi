package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/spf13/cobra"
)

var mergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge translations back into original format",
	Long:  `Merge translated XLIFF content back into the original document format.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inputPath, _ := cmd.Flags().GetString("input")
		outputPath, _ := cmd.Flags().GetString("output")
		outputFormat, _ := cmd.Flags().GetString("output-format")

		if inputPath == "" {
			return fmt.Errorf("--input (-i) is required")
		}
		if outputPath == "" {
			return fmt.Errorf("--output (-o) is required")
		}
		if targetLang == "" {
			return fmt.Errorf("--target-lang is required")
		}

		ctx := context.Background()

		// Detect input format (should be XLIFF)
		inputFormat := formatFlag
		if inputFormat == "" {
			ext := filepath.Ext(inputPath)
			detected, err := formatReg.Detector().DetectByExtension(ext)
			if err != nil {
				return fmt.Errorf("unable to detect format for %q: %w", inputPath, err)
			}
			inputFormat = detected
		}

		// Detect output format
		if outputFormat == "" {
			ext := filepath.Ext(outputPath)
			detected, err := formatReg.Detector().DetectByExtension(ext)
			if err != nil {
				return fmt.Errorf("unable to detect output format for %q: %w", outputPath, err)
			}
			outputFormat = detected
		}

		if !quiet {
			fmt.Printf("Merging %s → %s (%s, locale: %s)\n", inputPath, outputPath, outputFormat, targetLang)
		}

		// Read XLIFF
		reader, err := formatReg.NewReader(inputFormat)
		if err != nil {
			return fmt.Errorf("no reader for format %q: %w", inputFormat, err)
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

		// Write in output format
		writer, err := formatReg.NewWriter(outputFormat)
		if err != nil {
			return fmt.Errorf("no writer for format %q: %w", outputFormat, err)
		}

		if err := writer.SetOutput(outputPath); err != nil {
			return fmt.Errorf("set output: %w", err)
		}
		writer.SetLocale(model.LocaleID(targetLang))

		ch := make(chan *model.Part, len(parts))
		for _, p := range parts {
			ch <- p
		}
		close(ch)

		if err := writer.Write(ctx, ch); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		writer.Close()

		if !quiet {
			fmt.Println("Merge complete")
		}
		return nil
	},
}

func init() {
	mergeCmd.Flags().StringP("input", "i", "", "input translated file (e.g., XLIFF)")
	mergeCmd.Flags().StringP("output", "o", "", "output file path")
	mergeCmd.Flags().String("output-format", "", "output format (auto-detected from extension)")
	rootCmd.AddCommand(mergeCmd)
}
