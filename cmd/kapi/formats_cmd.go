package main

import (
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/registry"
	"github.com/spf13/cobra"
)

var (
	fmtMime string
	fmtExt  string
)

var formatsCmd = &cobra.Command{
	Use:   "formats",
	Short: "List available data formats",
	Long: `List all available data formats with metadata.

Use --mime or --ext to filter by MIME type or file extension.

Examples:
  kapi formats                     List all formats
  kapi formats --mime text/html    Find formats handling text/html
  kapi formats --ext .docx         Find formats handling .docx files`,
	Run: func(cmd *cobra.Command, args []string) {
		infos := formatReg.FormatInfos()

		if fmtMime != "" || fmtExt != "" {
			infos = filterFormats(infos, fmtMime, fmtExt)
			if len(infos) == 0 {
				fmt.Println("No formats found matching the given criteria.")
				return
			}
		}

		printFormatsTable(infos)
	},
}

func filterFormats(infos []registry.FormatInfo, mime, ext string) []registry.FormatInfo {
	mime = strings.ToLower(mime)
	ext = strings.ToLower(ext)
	var result []registry.FormatInfo
	for _, info := range infos {
		if mime != "" && !containsLower(info.MimeTypes, mime) {
			continue
		}
		if ext != "" && !containsLower(info.Extensions, ext) {
			continue
		}
		result = append(result, info)
	}
	return result
}

func printFormatsTable(infos []registry.FormatInfo) {
	fmt.Println("Available formats:")
	fmt.Println()
	fmt.Printf("  %-20s %-22s %-6s %-6s %-12s %-20s %s\n",
		"FORMAT", "DISPLAY NAME", "READ", "WRITE", "SOURCE", "EXTENSIONS", "MIME TYPES")
	fmt.Printf("  %-20s %-22s %-6s %-6s %-12s %-20s %s\n",
		"------", "------------", "----", "-----", "------", "----------", "----------")
	for _, info := range infos {
		read := "-"
		write := "-"
		if info.HasReader {
			read = "yes"
		}
		if info.HasWriter {
			write = "yes"
		}
		displayName := info.DisplayName
		if len(displayName) > 20 {
			displayName = displayName[:17] + "..."
		}
		exts := strings.Join(info.Extensions, ", ")
		if len(exts) > 18 {
			exts = exts[:15] + "..."
		}
		mimes := strings.Join(info.MimeTypes, ", ")
		if len(mimes) > 40 {
			mimes = mimes[:37] + "..."
		}
		fmt.Printf("  %-20s %-22s %-6s %-6s %-12s %-20s %s\n",
			info.Name, displayName, read, write, info.Source, exts, mimes)
	}
	fmt.Printf("\nTotal: %d format(s)\n", len(infos))
}

func containsLower(slice []string, val string) bool {
	for _, s := range slice {
		if strings.ToLower(s) == val {
			return true
		}
	}
	return false
}

func init() {
	formatsCmd.Flags().StringVar(&fmtMime, "mime", "", "filter by MIME type (e.g., text/html)")
	formatsCmd.Flags().StringVar(&fmtExt, "ext", "", "filter by file extension (e.g., .docx)")
	rootCmd.AddCommand(formatsCmd)
}
