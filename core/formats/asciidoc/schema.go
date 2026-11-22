package asciidoc

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the AsciiDoc format's
// parameters. The format is grammar-driven, so it exposes only a couple of
// behavioural toggles.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "AsciiDoc",
		Description: "Configuration for the native AsciiDoc (.adoc) reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "asciidoc",
			Extensions: []string{".adoc", ".asciidoc", ".adfm", ".asc"},
			MimeTypes:  []string{"text/asciidoc"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"extract_block_titles", "extract_table_cells",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"extract_block_titles": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract block titles",
				Default:     true,
				Description: "Emit AsciiDoc block titles (`.Title`) as translatable blocks. When false they are preserved verbatim as skeleton.",
			}),
			"extract_table_cells": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract table cells",
				Default:     true,
				Description: "Emit `|===` table cells as translatable blocks grouped into table / table-row groups. When false the table is preserved verbatim.",
			}),
		},
	}
}
