package po

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the PO format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "PO Format (GNU Gettext)",
		Description: "Configuration for the PO/POT format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatSchemaMeta{
			ID:         "po",
			Extensions: []string{".po", ".pot"},
			MimeTypes:  []string{"text/x-gettext-translation"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"preserveUntranslated",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"preserveUntranslated": {
				Type:        "boolean",
				Default:     true,
				Description: "If true, entries with empty msgstr are emitted as blocks. If false, they are skipped.",
			},
		},
	}
}
