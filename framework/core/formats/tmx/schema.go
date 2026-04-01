package tmx

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the TMX format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "TMX (Translation Memory eXchange)",
		Description: "TMX translation memory format — no configurable parameters",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "tmx",
			Extensions: []string{".tmx"},
			MimeTypes:  []string{"application/x-tmx+xml"},
		},
	}
}
