package yaml

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the YAML format.
func (c *Config) Schema() *schema.FilterSchema {
	return &schema.FilterSchema{
		Title:       "YAML Format",
		Description: "YAML format reader/writer — no configurable parameters",
		Type:        "object",
		FilterMeta: schema.FilterSchemaMeta{
			ID:         "yaml",
			Extensions: []string{".yaml", ".yml"},
			MimeTypes:  []string{"application/x-yaml", "text/yaml"},
		},
	}
}
