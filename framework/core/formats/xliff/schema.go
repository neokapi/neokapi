package xliff

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the XLIFF 1.2 format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "XLIFF 1.2",
		Description: "XLIFF 1.2 bilingual exchange format — no configurable parameters",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "xliff",
			Extensions: []string{".xlf", ".xliff"},
			MimeTypes:  []string{"application/xliff+xml"},
		},
	}
}
