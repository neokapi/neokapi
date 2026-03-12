package csv

import "github.com/gokapi/gokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the CSV format's parameters.
func (c *Config) Schema() *schema.FilterSchema {
	return &schema.FilterSchema{
		Title:       "CSV Format",
		Description: "Configuration for the CSV/TSV format reader/writer",
		Type:        "object",
		FilterMeta: schema.FilterSchemaMeta{
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
			"separator": {
				Type:        "string",
				Default:     ",",
				Description: "Field delimiter character",
			},
			"hasHeader": {
				Type:        "boolean",
				Default:     true,
				Description: "If true, the first row is treated as headers (non-translatable)",
			},
			"translatableColumns": {
				Type:        "array",
				Description: "Column indices (0-based) to extract as translatable content. If empty, all columns are translatable.",
			},
			"keyColumns": {
				Type:        "array",
				Description: "Column indices (0-based) that provide the block ID (source ID / record ID).",
			},
			"commentColumns": {
				Type:        "array",
				Description: "Column indices (0-based) that contain comments or notes.",
			},
			"trimValues": {
				Type:        "boolean",
				Default:     false,
				Description: "If true, leading and trailing whitespace is removed from cell values.",
			},
			"columnNamesRow": {
				Type:        "integer",
				Default:     0,
				Description: "1-based row number containing column names. 0 means auto-detect.",
			},
			"valuesStartRow": {
				Type:        "integer",
				Default:     0,
				Description: "1-based row number where data values begin. 0 means auto-detect.",
			},
		},
	}
}
