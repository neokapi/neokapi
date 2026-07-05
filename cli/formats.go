package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/spf13/cobra"
)

// NewFormatsCmd creates the formats command group (list, info, schema).
func NewFormatsCmd(a *App) *cobra.Command {
	var fmtMime, fmtExt string

	formatsCmd := &cobra.Command{
		Use:     "formats",
		Short:   "List supported file formats",
		GroupID: "advanced",
		Long: `List all file formats that can be read and written.

Use --mime or --ext to filter by MIME type or file extension.`,
		Example: `  kapi formats
  kapi formats --ext .json
  kapi formats --mime text/html`,
		RunE: func(cmd *cobra.Command, args []string) error {
			infos := a.FormatReg.FormatInfos()

			if fmtMime != "" || fmtExt != "" {
				infos = FilterFormats(infos, fmtMime, fmtExt)
				if len(infos) == 0 {
					if output.ResolveFormat(cmd) == output.FormatJSON {
						return output.Print(cmd, output.FormatsListOutput{})
					}
					fmt.Println("No formats found matching the given criteria.")
					return nil
				}
			}

			// Hide versioned entries (e.g., "okf_html@2.8.0") when a
			// bare-name alias exists (e.g., "okf_html"). This keeps the
			// list clean — users see "okf_html" rather than duplicates.
			infos = DeduplicateVersionedFormats(infos)

			t := a.T()
			out := output.FormatsListOutput{
				Formats: make([]output.FormatInfo, 0, len(infos)),
				Total:   len(infos),
			}
			for _, info := range infos {
				name := string(info.Name)
				out.Formats = append(out.Formats, output.FormatInfo{
					Name:        name,
					DisplayName: t.T(i18n.Scope("formats."+name+".DisplayName"), info.DisplayName),
					HasReader:   info.HasReader,
					HasWriter:   info.HasWriter,
					Generative:  info.Generative,
					Interchange: info.Interchange,
					Editable:    info.Editable,
					RoundTrip:   info.RoundTrip,
					Source:      info.Source,
					Extensions:  info.Extensions,
					MimeTypes:   info.MimeTypes,
				})
			}
			return output.Print(cmd, out)
		},
	}

	formatsCmd.Flags().StringVar(&fmtMime, "mime", "", "filter by MIME type (e.g., text/html)")
	formatsCmd.Flags().StringVar(&fmtExt, "ext", "", "filter by file extension (e.g., .docx)")

	formatsCmd.AddCommand(newFormatsInfoCmd(a))
	formatsCmd.AddCommand(newFormatsSchemaCmd(a))

	return formatsCmd
}

func newFormatsInfoCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:     "info <format>",
		Short:   "Show detailed information about a format",
		Example: "  kapi formats info json\n  kapi formats info okf_html",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filterID := args[0]
			info := a.FormatReg.FormatInfo(registry.FormatID(filterID))

			var filterSchema *schema.FormatSchema
			if a.SchemaReg != nil {
				filterSchema, _ = a.SchemaReg.GetSchema(filterID)
			}

			if info == nil && filterSchema == nil {
				return fmt.Errorf("format not found: %s", filterID)
			}

			out := output.FormatInfoOutput{Name: filterID}

			if filterSchema != nil {
				out.DisplayName = filterSchema.Title
				out.FilterID = filterSchema.FormatMeta.ID
				out.Class = filterSchema.FormatMeta.Class
				out.Version = filterSchema.Version
				out.HasSchema = true
				out.Extensions = filterSchema.FormatMeta.Extensions
				out.MimeTypes = filterSchema.FormatMeta.MimeTypes

				if len(filterSchema.Groups) > 0 {
					groupedFields := make(map[string]bool)
					for _, group := range filterSchema.Groups {
						g := output.FormatInfoGroup{
							Label:       group.Label,
							Description: group.Description,
						}
						for _, fieldName := range group.Fields {
							groupedFields[fieldName] = true
							prop, ok := filterSchema.Properties[fieldName]
							if !ok {
								continue
							}
							g.Parameters = append(g.Parameters, ToFormatInfoParam(fieldName, prop))
						}
						out.Groups = append(out.Groups, g)
					}

					var ungroupedParams []output.FormatInfoParam
					for name, prop := range filterSchema.Properties {
						if !groupedFields[name] {
							ungroupedParams = append(ungroupedParams, ToFormatInfoParam(name, prop))
						}
					}
					if len(ungroupedParams) > 0 {
						out.Groups = append(out.Groups, output.FormatInfoGroup{
							Label:      "Other Parameters",
							Parameters: ungroupedParams,
						})
					}
				} else {
					var params []output.FormatInfoParam
					for name, prop := range filterSchema.Properties {
						params = append(params, ToFormatInfoParam(name, prop))
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
				if out.DisplayName == "" {
					out.DisplayName = info.DisplayName
				}
				out.Source = info.Source
				out.HasReader = info.HasReader
				out.HasWriter = info.HasWriter
				out.Editable = info.Editable
				out.RoundTrip = info.RoundTrip
				if len(out.Extensions) == 0 {
					out.Extensions = info.Extensions
				}
				if len(out.MimeTypes) == 0 {
					out.MimeTypes = info.MimeTypes
				}
			}

			// Check if the format declares a config kind.
			if reader, err := a.FormatReg.NewReader(registry.FormatID(filterID)); err == nil {
				if cfg := reader.Config(); cfg != nil {
					if ckp, ok := cfg.(format.ConfigKindProvider); ok {
						out.ConfigKind = string(ckp.ConfigKind())
					}
				}
			}

			return output.Print(cmd, out)
		},
	}
}

func newFormatsSchemaCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:     "schema <format>",
		Short:   "Output JSON Schema for a format",
		Example: "  kapi formats schema json\n  kapi formats schema okf_html",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filterID := args[0]

			if a.SchemaReg == nil {
				return errors.New("no schema registry available")
			}

			rawJSON, ok := a.SchemaReg.GetSchemaJSON(filterID)
			if !ok {
				return fmt.Errorf("schema not found for format: %s\nUse 'formats' to list available formats", filterID)
			}

			var prettyJSON map[string]any
			if err := json.Unmarshal(rawJSON, &prettyJSON); err != nil {
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
}
