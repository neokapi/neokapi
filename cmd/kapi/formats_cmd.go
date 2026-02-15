package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/plugin/loader"
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
	formatsCmd.AddCommand(formatsInfoCmd)
	formatsCmd.AddCommand(formatsSchemaCmd)
	rootCmd.AddCommand(formatsCmd)
}

// formatsInfoCmd shows detailed information about a specific format
var formatsInfoCmd = &cobra.Command{
	Use:   "info <format>",
	Short: "Show detailed information about a format",
	Long: `Show detailed information about a specific format, including its
parameters schema, default values, and supported options.

Examples:
  kapi formats info okf_json       Show JSON filter parameters
  kapi formats info html           Show HTML filter parameters
  kapi formats info okapi-json     Show bridge format info`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filterID := args[0]
		
		// Try to find format info
		info := formatReg.FormatInfo(filterID)
		
		// Try to find schema
		var schema *loader.FilterSchema
		if pluginLoader != nil {
			schema, _ = pluginLoader.Schemas().GetSchema(filterID)
		}
		
		if info == nil && schema == nil {
			fmt.Fprintf(os.Stderr, "Format not found: %s\n", filterID)
			os.Exit(1)
		}
		
		// Print header
		title := filterID
		if schema != nil && schema.Title != "" {
			title = schema.Title
		} else if info != nil && info.DisplayName != "" {
			title = info.DisplayName
		}
		fmt.Printf("%s\n", title)
		fmt.Println(strings.Repeat("=", len(title)))
		fmt.Println()
		
		// Print schema metadata if available
		if schema != nil {
			fmt.Printf("Filter ID:      %s\n", schema.FilterMeta.ID)
			fmt.Printf("Java Class:     %s\n", schema.FilterMeta.Class)
			fmt.Printf("Schema Version: %s\n", schema.Version)
			if len(schema.FilterMeta.Extensions) > 0 {
				fmt.Printf("Extensions:     %s\n", strings.Join(schema.FilterMeta.Extensions, ", "))
			}
			if len(schema.FilterMeta.MimeTypes) > 0 {
				fmt.Printf("MIME Types:     %s\n", strings.Join(schema.FilterMeta.MimeTypes, ", "))
			}
			fmt.Println()
			
			// Print parameters grouped by category
			if len(schema.Groups) > 0 {
				for _, group := range schema.Groups {
					fmt.Printf("%s\n", group.Label)
					fmt.Println(strings.Repeat("-", len(group.Label)))
					if group.Description != "" {
						fmt.Printf("  %s\n\n", group.Description)
					}
					
					for _, fieldName := range group.Fields {
						prop, ok := schema.Properties[fieldName]
						if !ok {
							continue
						}
						printParameter(fieldName, prop)
					}
					fmt.Println()
				}
				
				// Print ungrouped parameters
				groupedFields := make(map[string]bool)
				for _, group := range schema.Groups {
					for _, field := range group.Fields {
						groupedFields[field] = true
					}
				}
				
				var ungrouped []string
				for name := range schema.Properties {
					if !groupedFields[name] {
						ungrouped = append(ungrouped, name)
					}
				}
				
				if len(ungrouped) > 0 {
					fmt.Println("Other Parameters")
					fmt.Println("----------------")
					for _, name := range ungrouped {
						printParameter(name, schema.Properties[name])
					}
				}
			} else {
				// No groups - print all parameters
				fmt.Println("Parameters")
				fmt.Println("----------")
				for name, prop := range schema.Properties {
					printParameter(name, prop)
				}
			}
		} else if info != nil {
			// No schema - print basic info
			fmt.Printf("Format:     %s\n", info.Name)
			if info.DisplayName != "" {
				fmt.Printf("Display:    %s\n", info.DisplayName)
			}
			if info.Source != "" {
				fmt.Printf("Source:     %s\n", info.Source)
			}
			fmt.Printf("Has Reader: %v\n", info.HasReader)
			fmt.Printf("Has Writer: %v\n", info.HasWriter)
			if len(info.Extensions) > 0 {
				fmt.Printf("Extensions: %s\n", strings.Join(info.Extensions, ", "))
			}
			if len(info.MimeTypes) > 0 {
				fmt.Printf("MIME Types: %s\n", strings.Join(info.MimeTypes, ", "))
			}
			fmt.Println()
			fmt.Println("No parameter schema available for this format.")
		}
	},
}

// printParameter prints a single parameter with its metadata
func printParameter(name string, prop loader.PropertySchema) {
	typeStr := prop.Type
	if prop.OkapiFormat != "" {
		typeStr = prop.OkapiFormat
	}
	
	defaultStr := ""
	if prop.Default != nil {
		switch v := prop.Default.(type) {
		case bool:
			defaultStr = fmt.Sprintf(" (default: %v)", v)
		case string:
			if v != "" {
				defaultStr = fmt.Sprintf(" (default: %q)", v)
			}
		case float64:
			if v == float64(int(v)) {
				defaultStr = fmt.Sprintf(" (default: %d)", int(v))
			} else {
				defaultStr = fmt.Sprintf(" (default: %v)", v)
			}
		}
	}
	
	fmt.Printf("  %-24s %-10s%s\n", name, typeStr, defaultStr)
	if prop.Description != "" {
		// Word-wrap description at 60 chars
		desc := prop.Description
		indent := "                            "
		if len(desc) > 60 {
			fmt.Printf("%s%s\n", indent, desc)
		} else {
			fmt.Printf("%s%s\n", indent, desc)
		}
	}
}

// formatsSchemaCmd outputs the raw JSON Schema for a format
var formatsSchemaCmd = &cobra.Command{
	Use:   "schema <format>",
	Short: "Output JSON Schema for a format",
	Long: `Output the raw JSON Schema for a format's parameters.
This can be used for tooling integration, validation, or code generation.

Examples:
  kapi formats schema okf_json > okf_json.schema.json
  kapi formats schema html | jq .properties`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filterID := args[0]
		
		if pluginLoader == nil {
			fmt.Fprintf(os.Stderr, "No plugins loaded - schema not available\n")
			os.Exit(1)
		}
		
		rawJSON, ok := pluginLoader.Schemas().GetSchemaJSON(filterID)
		if !ok {
			fmt.Fprintf(os.Stderr, "Schema not found for format: %s\n", filterID)
			fmt.Fprintf(os.Stderr, "Use 'kapi formats' to list available formats.\n")
			os.Exit(1)
		}
		
		// Pretty-print the JSON
		var prettyJSON map[string]interface{}
		if err := json.Unmarshal(rawJSON, &prettyJSON); err != nil {
			// Fall back to raw output
			fmt.Println(string(rawJSON))
			return
		}
		
		output, err := json.MarshalIndent(prettyJSON, "", "  ")
		if err != nil {
			fmt.Println(string(rawJSON))
			return
		}
		fmt.Println(string(output))
	},
}
