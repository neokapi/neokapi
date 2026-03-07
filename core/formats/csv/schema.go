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
					"separator", "hasHeader",
				},
			},
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"translatableColumns",
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
		},
	}
}
