package xliff2

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the XLIFF 2.0 format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "XLIFF 2.0",
		Description: "XLIFF 2.0/2.1 bilingual exchange format with segment support — no configurable parameters",
		Type:        "object",
		FormatMeta: schema.FormatSchemaMeta{
			ID:         "xliff2",
			Extensions: []string{".xlf", ".xliff"},
			MimeTypes:  []string{"application/xliff+xml"},
		},
	}
}
