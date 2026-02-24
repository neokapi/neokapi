package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/plugin/loader"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/kapi/cmd/kapi/output"
	"github.com/spf13/cobra"
)

var (
	fmtMime string
	fmtExt  string
)

var formatsCmd = &cobra.Command{
	Use:   "formats",
	Short: "List supported file formats",
	Long: `List all file formats that kapi can read and write.

Use --mime or --ext to filter by MIME type or file extension.

Examples:
  kapi formats                     List all formats
  kapi formats --mime text/html    Find formats handling text/html
  kapi formats --ext .docx         Find formats handling .docx files`,
	RunE: func(cmd *cobra.Command, args []string) error {
		infos := formatReg.FormatInfos()

		if fmtMime != "" || fmtExt != "" {
			infos = filterFormats(infos, fmtMime, fmtExt)
			if len(infos) == 0 {
				if output.GetFormat(cmd) == output.FormatJSON {
					return output.Print(cmd, output.FormatsListOutput{})
				}
				fmt.Println("No formats found matching the given criteria.")
				return nil
			}
		}

		out := output.FormatsListOutput{
			Formats: make([]output.FormatInfo, len(infos)),
			Total:   len(infos),
		}
		for i, info := range infos {
			out.Formats[i] = output.FormatInfo{
				Name:        info.Name,
				DisplayName: info.DisplayName,
				HasReader:   info.HasReader,
				HasWriter:   info.HasWriter,
				Source:      info.Source,
				Extensions:  info.Extensions,
				MimeTypes:   info.MimeTypes,
			}
		}
		return output.Print(cmd, out)
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
	RunE: func(cmd *cobra.Command, args []string) error {
		filterID := args[0]

		// Try to find format info
		info := formatReg.FormatInfo(filterID)

		// Try to find schema
		var schema *loader.FilterSchema
		if pluginLoader != nil {
			schema, _ = pluginLoader.Schemas().GetSchema(filterID)
		}

		if info == nil && schema == nil {
			return fmt.Errorf("format not found: %s", filterID)
		}

		out := output.FormatInfoOutput{
			Name: filterID,
		}

		if schema != nil {
			// Populate from schema metadata
			out.DisplayName = schema.Title
			out.FilterID = schema.FilterMeta.ID
			out.Class = schema.FilterMeta.Class
			out.Version = schema.Version
			out.HasSchema = true
			out.Extensions = schema.FilterMeta.Extensions
			out.MimeTypes = schema.FilterMeta.MimeTypes

			// Build parameter groups
			if len(schema.Groups) > 0 {
				groupedFields := make(map[string]bool)
				for _, group := range schema.Groups {
					g := output.FormatInfoGroup{
						Label:       group.Label,
						Description: group.Description,
					}
					for _, fieldName := range group.Fields {
						groupedFields[fieldName] = true
						prop, ok := schema.Properties[fieldName]
						if !ok {
							continue
						}
						g.Parameters = append(g.Parameters, toFormatInfoParam(fieldName, prop))
					}
					out.Groups = append(out.Groups, g)
				}

				// Collect ungrouped parameters
				var ungroupedParams []output.FormatInfoParam
				for name, prop := range schema.Properties {
					if !groupedFields[name] {
						ungroupedParams = append(ungroupedParams, toFormatInfoParam(name, prop))
					}
				}
				if len(ungroupedParams) > 0 {
					out.Groups = append(out.Groups, output.FormatInfoGroup{
						Label:      "Other Parameters",
						Parameters: ungroupedParams,
					})
				}
			} else {
				// No groups - put all parameters in a single group
				var params []output.FormatInfoParam
				for name, prop := range schema.Properties {
					params = append(params, toFormatInfoParam(name, prop))
				}
				if len(params) > 0 {
					out.Groups = append(out.Groups, output.FormatInfoGroup{
						Label:      "Parameters",
						Parameters: params,
					})
				}
			}
		}

		if info != nil {
			// Fill in fields from format info (schema fields take precedence if set)
			if out.DisplayName == "" {
				out.DisplayName = info.DisplayName
			}
			out.Source = info.Source
			out.HasReader = info.HasReader
			out.HasWriter = info.HasWriter
			if len(out.Extensions) == 0 {
				out.Extensions = info.Extensions
			}
			if len(out.MimeTypes) == 0 {
				out.MimeTypes = info.MimeTypes
			}
		}

		return output.Print(cmd, out)
	},
}

// toFormatInfoParam converts a loader.PropertySchema to an output.FormatInfoParam.
func toFormatInfoParam(name string, prop loader.PropertySchema) output.FormatInfoParam {
	typeStr := prop.Type
	if prop.OkapiFormat != "" {
		typeStr = prop.OkapiFormat
	}
	return output.FormatInfoParam{
		Name:        name,
		Type:        typeStr,
		Default:     prop.Default,
		Description: prop.Description,
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
	RunE: func(cmd *cobra.Command, args []string) error {
		filterID := args[0]

		if pluginLoader == nil {
			return fmt.Errorf("no plugins loaded - schema not available")
		}

		rawJSON, ok := pluginLoader.Schemas().GetSchemaJSON(filterID)
		if !ok {
			return fmt.Errorf("schema not found for format: %s\nUse 'kapi formats' to list available formats", filterID)
		}

		// Pretty-print the JSON
		var prettyJSON map[string]any
		if err := json.Unmarshal(rawJSON, &prettyJSON); err != nil {
			// Fall back to raw output
			fmt.Println(string(rawJSON))
			return nil
		}

		prettyOut, err := json.MarshalIndent(prettyJSON, "", "  ")
		if err != nil {
			fmt.Println(string(rawJSON))
			return nil
		}
		fmt.Println(string(prettyOut))
		return nil
	},
}
