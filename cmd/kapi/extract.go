package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/spf13/cobra"
)

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract translatable content to XLIFF",
	Long:  `Extract translatable content from source files and write to XLIFF format.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inputPath, _ := cmd.Flags().GetString("input")
		outputPath, _ := cmd.Flags().GetString("output")

		if inputPath == "" {
			return fmt.Errorf("--input (-i) is required")
		}
		if outputPath == "" {
			outputPath = inputPath + ".xlf"
		}

		ctx := context.Background()

		// Detect format
		fmtName := formatFlag
		if fmtName == "" {
			ext := filepath.Ext(inputPath)
			detected, err := formatReg.Detector().DetectByExtension(ext)
			if err != nil {
				return fmt.Errorf("unable to detect format for %q: %w", inputPath, err)
			}
			fmtName = detected
		}

		if !quiet {
			fmt.Printf("Extracting %s (%s) → %s\n", inputPath, fmtName, outputPath)
		}

		// Open reader
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

		// Collect parts
		var parts []*model.Part
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				return fmt.Errorf("read error: %w", result.Error)
			}
			parts = append(parts, result.Part)
		}

		// Write as XLIFF
		writer, err := formatReg.NewWriter("xliff")
		if err != nil {
			return fmt.Errorf("no XLIFF writer: %w", err)
		}

		if err := writer.SetOutput(outputPath); err != nil {
			return fmt.Errorf("set output: %w", err)
		}
		writer.SetLocale(model.LocaleID(sourceLang))

		ch := make(chan *model.Part, len(parts))
		for _, p := range parts {
			ch <- p
		}
		close(ch)

		if err := writer.Write(ctx, ch); err != nil {
			return fmt.Errorf("write XLIFF: %w", err)
		}
		writer.Close()

		if !quiet {
			blocks := 0
			for _, p := range parts {
				if p.Type == model.PartBlock {
					blocks++
				}
			}
			fmt.Printf("Extracted %d translatable block(s)\n", blocks)
		}

		return nil
	},
}

func init() {
	extractCmd.Flags().StringP("input", "i", "", "input file path")
	extractCmd.Flags().StringP("output", "o", "", "output XLIFF file path")
	rootCmd.AddCommand(extractCmd)
}
