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
				ID:    "format",
				Label: "CSV Format Settings",
				Fields: []string{
					"separator", "textQualifier", "hasHeader",
					"columnNamesRow", "valuesStartRow",
				},
			},
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"translatableColumns", "keyColumns", "commentColumns",
					"trimValues", "extractNonTranslatableContent",
				},
			},
			{
				ID:    "inlineCodes",
				Label: "Inline Codes",
				Fields: []string{
					"useCodeFinder", "codeFinderRules",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"separator": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     ",",
				Title:       "Field Delimiter",
				Description: "Field delimiter character (comma for CSV, tab for TSV)",
			}),
			"textQualifier": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     "\"",
				Title:       "Text Qualifier",
				Description: "Character used to quote field values containing delimiters or newlines",
			}),
			"hasHeader": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Has Header Row",
				Description: "If true, the first row is treated as headers (non-translatable)",
			}),
			"translatableColumns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Translatable Columns",
				Description: "Column indices (0-based) to extract as translatable content. If empty, all columns are translatable.",
			}),
			"keyColumns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Key Columns",
				Description: "Column indices (0-based) that provide the block ID (source ID / record ID)",
			}),
			"commentColumns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Comment Columns",
				Description: "Column indices (0-based) that contain comments or notes",
			}),
			"trimValues": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Trim Values",
				Description: "If true, leading and trailing whitespace is removed from cell values",
			}),
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable contextual content such as preamble rows and non-translatable column cells is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of opaque data. Disable to keep it as plain data parts.",
			}),
			"columnNamesRow": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Default:     0,
				Title:       "Column Names Row",
				Description: "1-based row number containing column names. 0 means auto-detect.",
			}),
			"valuesStartRow": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Default:     0,
				Title:       "Values Start Row",
				Description: "1-based row number where data values begin. 0 means auto-detect.",
			}),
			"useCodeFinder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Inline Code Detection",
				Description: "Enable regex-based inline code detection within cell values",
			}),
			"codeFinderRules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within translatable text",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "useCodeFinder", Eq: true},
			}),
		},
	}
}
