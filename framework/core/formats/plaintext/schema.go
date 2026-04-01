package plaintext

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the Plain Text format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Plain Text Format",
		Description: "Configuration for the plain text format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "plaintext",
			Extensions: []string{".txt"},
			MimeTypes:  []string{"text/plain"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "segmentation",
				Label: "Segmentation",
				Fields: []string{
					"segmentByLine",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"segmentByLine": {
				Type:        "boolean",
				Default:     true,
				Description: "If true, each line becomes a separate block. If false, paragraphs (separated by blank lines) are blocks.",
			},
		},
	}
}
