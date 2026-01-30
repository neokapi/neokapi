package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gokapi/gokapi/core/kaz"
	"github.com/gokapi/gokapi/core/model"
	"github.com/spf13/cobra"
)

var packCmd = &cobra.Command{
	Use:   "pack",
	Short: "Pack source items into a .kaz package",
	Long: `Pack one or more source items into a .kaz translation package.
Each item is parsed using the appropriate format reader to extract
translatable blocks. The package includes block indices, preview HTML,
and the original source items.

Example:
  kapi pack -i page.html -i readme.md --source-lang en --target-lang fr,de -o project.kaz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inputs, _ := cmd.Flags().GetStringSlice("input")
		outputPath, _ := cmd.Flags().GetString("output")

		if len(inputs) == 0 {
			return fmt.Errorf("at least one --input (-i) is required")
		}
		if outputPath == "" {
			return fmt.Errorf("--output (-o) is required")
		}

		// Ensure .kaz extension
		if !strings.HasSuffix(strings.ToLower(outputPath), ".kaz") {
			outputPath += ".kaz"
		}

		// Parse target languages
		targets := parseTargetLangs(cmd)
		if len(targets) == 0 {
			return fmt.Errorf("--target-lang is required (comma-separated)")
		}

		ctx := context.Background()
		var packItems []kaz.PackItem

		for _, inputPath := range inputs {
			// Detect format
			inputFormat := formatFlag
			if inputFormat == "" {
				ext := filepath.Ext(inputPath)
				detected, err := formatReg.Detector().DetectByExtension(ext)
				if err != nil {
					if !quiet {
						fmt.Fprintf(os.Stderr, "Skipping %q: %v\n", inputPath, err)
					}
					continue
				}
				inputFormat = detected
			}

			// Read source bytes
			data, err := os.ReadFile(inputPath)
			if err != nil {
				return fmt.Errorf("read %q: %w", inputPath, err)
			}

			// Parse with format reader
			reader, err := formatReg.NewReader(inputFormat)
			if err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "Skipping %q: no reader for %q\n", inputPath, inputFormat)
				}
				continue
			}

			doc := &model.RawDocument{
				URI:          inputPath,
				SourceLocale: model.LocaleID(sourceLang),
				Encoding:     encoding,
				Reader:       io.NopCloser(bytes.NewReader(data)),
			}

			if err := reader.Open(ctx, doc); err != nil {
				return fmt.Errorf("open %q: %w", inputPath, err)
			}

			var parts []*model.Part
			for result := range reader.Read(ctx) {
				if result.Error != nil {
					reader.Close()
					return fmt.Errorf("read %q: %w", inputPath, result.Error)
				}
				parts = append(parts, result.Part)
			}
			reader.Close()

			itemName := filepath.Base(inputPath)
			packItems = append(packItems, kaz.PackItem{
				Name:        itemName,
				Type:        "file",
				Format:      inputFormat,
				SourceBytes: data,
				Parts:       parts,
			})

			if !quiet {
				blockCount := 0
				for _, pt := range parts {
					if pt.Type == model.PartBlock {
						blockCount++
					}
				}
				fmt.Printf("  %s (%s): %d blocks\n", itemName, inputFormat, blockCount)
			}
		}

		if len(packItems) == 0 {
			return fmt.Errorf("no items to pack")
		}

		// Derive project name from output path
		name := filepath.Base(outputPath)
		name = strings.TrimSuffix(name, ".kaz")

		// Create output file
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("create %q: %w", outputPath, err)
		}
		defer f.Close()

		err = kaz.Pack(f, kaz.PackOptions{
			Name:          name,
			SourceLocale:  sourceLang,
			TargetLocales: targets,
			Items:         packItems,
		})
		if err != nil {
			return fmt.Errorf("pack: %w", err)
		}

		if !quiet {
			fmt.Printf("Packed %d item(s) → %s\n", len(packItems), outputPath)
		}
		return nil
	},
}

// parseTargetLangs splits the target-lang flag by comma.
func parseTargetLangs(cmd *cobra.Command) []string {
	raw := targetLang
	if raw == "" {
		return nil
	}
	var targets []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			targets = append(targets, t)
		}
	}
	return targets
}

func init() {
	packCmd.Flags().StringSliceP("input", "i", nil, "input file path(s) (repeatable)")
	packCmd.Flags().StringP("output", "o", "", "output .kaz file path")
	rootCmd.AddCommand(packCmd)
}
