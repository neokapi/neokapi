package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/asgeirf/gokapi/core/kaz"
	"gopkg.in/yaml.v3"
	"github.com/spf13/cobra"
)

var unpackCmd = &cobra.Command{
	Use:   "unpack",
	Short: "Unpack a .kaz package to a directory",
	Long: `Unpack a .kaz translation package to a directory, extracting
the manifest, source items, block indices, and preview HTML.

Example:
  kapi unpack -i project.kaz -o ./unpacked/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inputPath, _ := cmd.Flags().GetString("input")
		outputDir, _ := cmd.Flags().GetString("output")

		if inputPath == "" {
			return fmt.Errorf("--input (-i) is required")
		}
		if outputDir == "" {
			return fmt.Errorf("--output (-o) is required")
		}

		// Read and unpack .kaz file
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return fmt.Errorf("read %q: %w", inputPath, err)
		}

		pkg, err := kaz.UnpackFromBytes(data)
		if err != nil {
			return fmt.Errorf("unpack %q: %w", inputPath, err)
		}

		// Create output directory structure
		dirs := []string{
			outputDir,
			filepath.Join(outputDir, "items"),
			filepath.Join(outputDir, "blocks"),
			filepath.Join(outputDir, "preview"),
		}
		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("mkdir %q: %w", dir, err)
			}
		}

		// Write manifest
		manifestData, err := yaml.Marshal(pkg.Manifest)
		if err != nil {
			return fmt.Errorf("marshal manifest: %w", err)
		}
		manifestPath := filepath.Join(outputDir, "manifest.yaml")
		if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}
		if !quiet {
			fmt.Printf("  manifest.yaml\n")
		}

		// Write source items
		for name, content := range pkg.Items {
			itemPath := filepath.Join(outputDir, "items", name)
			if err := os.WriteFile(itemPath, content, 0644); err != nil {
				return fmt.Errorf("write item %q: %w", name, err)
			}
			if !quiet {
				fmt.Printf("  items/%s (%d bytes)\n", name, len(content))
			}
		}

		// Write block indices
		for name, blockIndex := range pkg.Blocks {
			blockData, err := json.MarshalIndent(blockIndex, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal blocks for %q: %w", name, err)
			}
			blockPath := filepath.Join(outputDir, "blocks", name+".json")
			if err := os.WriteFile(blockPath, blockData, 0644); err != nil {
				return fmt.Errorf("write blocks %q: %w", name, err)
			}
			if !quiet {
				fmt.Printf("  blocks/%s.json (%d blocks)\n", name, len(blockIndex.Blocks))
			}
		}

		// Write previews
		for name, html := range pkg.Previews {
			previewPath := filepath.Join(outputDir, "preview", name+".html")
			if err := os.WriteFile(previewPath, []byte(html), 0644); err != nil {
				return fmt.Errorf("write preview %q: %w", name, err)
			}
			if !quiet {
				fmt.Printf("  preview/%s.html\n", name)
			}
		}

		if !quiet {
			fmt.Printf("Unpacked %s → %s (%d items)\n", inputPath, outputDir, len(pkg.Manifest.Items))
		}
		return nil
	},
}

func init() {
	unpackCmd.Flags().StringP("input", "i", "", "input .kaz file path")
	unpackCmd.Flags().StringP("output", "o", "", "output directory path")
	rootCmd.AddCommand(unpackCmd)
}
