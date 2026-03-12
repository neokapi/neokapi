package xliff2

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the XLIFF 2.0 format.
func (c *Config) Schema() *schema.FilterSchema {
	return &schema.FilterSchema{
		Title:       "XLIFF 2.0",
		Description: "XLIFF 2.0/2.1 bilingual exchange format with segment support — no configurable parameters",
		Type:        "object",
		FilterMeta: schema.FilterSchemaMeta{
			ID:         "xliff2",
			Extensions: []string{".xlf", ".xliff"},
			MimeTypes:  []string{"application/xliff+xml"},
		},
	}
}
