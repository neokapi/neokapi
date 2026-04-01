package csv

import (
	coreschema "github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the CSV format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "CSV Format",
		Description: "Configuration for the CSV/TSV format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "csv",
			Extensions: []string{".csv", ".tsv"},
			MimeTypes:  []string{"text/csv"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "parsing",
				Label: "Parsing",
				Fields: []string{
					"separator", "hasHeader", "trimValues",
					"columnNamesRow", "valuesStartRow",
				},
			},
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"translatableColumns", "keyColumns", "commentColumns",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"separator": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     ",",
				Description: "Field delimiter character",
			}),
			"hasHeader": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Description: "If true, the first row is treated as headers (non-translatable)",
			}),
			"translatableColumns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Description: "Column indices (0-based) to extract as translatable content. If empty, all columns are translatable.",
			}),
			"keyColumns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Description: "Column indices (0-based) that provide the block ID (source ID / record ID).",
			}),
			"commentColumns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Description: "Column indices (0-based) that contain comments or notes.",
			}),
			"trimValues": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Description: "If true, leading and trailing whitespace is removed from cell values.",
			}),
			"columnNamesRow": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Default:     0,
				Description: "1-based row number containing column names. 0 means auto-detect.",
			}),
			"valuesStartRow": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Default:     0,
				Description: "1-based row number where data values begin. 0 means auto-detect.",
			}),
		},
	}
}
