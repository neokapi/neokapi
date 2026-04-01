package yaml

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the YAML format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "YAML Format",
		Description: "YAML format reader/writer — no configurable parameters",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "yaml",
			Extensions: []string{".yaml", ".yml"},
			MimeTypes:  []string{"application/x-yaml", "text/yaml"},
		},
	}
}
