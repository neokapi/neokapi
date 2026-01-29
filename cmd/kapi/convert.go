package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/plugin/loader"
	"github.com/spf13/cobra"
)

var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Convert between data formats",
	Long:  `Convert a file from one localization format to another.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inputPath, _ := cmd.Flags().GetString("input")
		outputPath, _ := cmd.Flags().GetString("output")

		if inputPath == "" {
			return fmt.Errorf("--input (-i) is required")
		}
		if outputPath == "" {
			return fmt.Errorf("--output (-o) is required")
		}

		ctx := context.Background()

		// Detect input format
		inputFormat := formatFlag
		if inputFormat == "" {
			ext := filepath.Ext(inputPath)
			detected, err := formatReg.Detector().DetectByExtension(ext)
			if err != nil {
				return fmt.Errorf("unable to detect input format for %q: %w", inputPath, err)
			}
			inputFormat = detected
		}

		// Detect output format
		outExt := filepath.Ext(outputPath)
		outputFormat, err := formatReg.Detector().DetectByExtension(outExt)
		if err != nil {
			return fmt.Errorf("unable to detect output format for %q: %w", outputPath, err)
		}

		if !quiet {
			fmt.Printf("Converting %s (%s) → %s (%s)\n", inputPath, inputFormat, outputPath, outputFormat)
		}

		// Read input
		reader, err := formatReg.NewReader(inputFormat)
		if err != nil {
			return fmt.Errorf("no reader for format %q: %w", inputFormat, err)
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

		// Write output
		writer, err := formatReg.NewWriter(outputFormat)
		if err != nil {
			return fmt.Errorf("no writer for format %q: %w", outputFormat, err)
		}

		if err := writer.SetOutput(outputPath); err != nil {
			return fmt.Errorf("set output: %w", err)
		}

		if ocs, ok := writer.(loader.OriginalContentSetter); ok {
			ocs.SetOriginalContent(inputContent)
		}

		locale := model.LocaleID(sourceLang)
		if targetLang != "" {
			locale = model.LocaleID(targetLang)
		}
		writer.SetLocale(locale)

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
			fmt.Printf("Converted %d part(s)\n", len(parts))
		}
		return nil
	},
}

func init() {
	convertCmd.Flags().StringP("input", "i", "", "input file path")
	convertCmd.Flags().StringP("output", "o", "", "output file path")
	rootCmd.AddCommand(convertCmd)
}
