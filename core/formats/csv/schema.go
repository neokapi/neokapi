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
					"separator", "text_qualifier", "has_header",
					"column_names_row", "values_start_row",
				},
			},
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"translatable_columns", "key_columns", "comment_columns",
					"columns", "trim_values", "extract_non_translatable_content",
				},
			},
			{
				ID:    "inlineCodes",
				Label: "Inline Codes",
				Fields: []string{
					"use_code_finder", "code_finder_rules",
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
			"text_qualifier": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     "\"",
				Title:       "Text Qualifier",
				Description: "Character used to quote field values containing delimiters or newlines",
			}),
			"has_header": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Has Header Row",
				Description: "If true, the first row is treated as headers (non-translatable)",
			}),
			"translatable_columns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Translatable Columns",
				Description: "Column indices (0-based) to extract as translatable content. If empty, all columns are translatable.",
			}),
			"key_columns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Key Columns",
				Description: "Column indices (0-based) that provide the block ID (source ID / record ID)",
			}),
			"comment_columns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Comment Columns",
				Description: "Column indices (0-based) that contain comments or notes",
			}),
			"columns": schema.Prop(coreschema.PropertySchema{
				Type:        "object",
				Title:       "Column Roles",
				Description: "Column-role mapping for bilingual translation tables: an object with 0-based column indices for key, source and target (e.g. {key: 0, source: 1, target: 2}). On merge only the target column is rewritten.",
			}),
			"trim_values": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Trim Values",
				Description: "If true, leading and trailing whitespace is removed from cell values",
			}),
			"extract_non_translatable_content": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable contextual content such as preamble rows and non-translatable column cells is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of opaque data. Disable to keep it as plain data parts.",
			}),
			"column_names_row": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Default:     0,
				Title:       "Column Names Row",
				Description: "1-based row number containing column names. 0 means auto-detect.",
			}),
			"values_start_row": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Default:     0,
				Title:       "Values Start Row",
				Description: "1-based row number where data values begin. 0 means auto-detect.",
			}),
			"use_code_finder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Inline Code Detection",
				Description: "Enable regex-based inline code detection within cell values",
			}),
			"code_finder_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within translatable text",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "use_code_finder", Eq: true},
			}),
		},
	}
}
