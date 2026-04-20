package mo

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the MO format writer.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "MO Format (GNU Gettext, binary)",
		Description: "Writer for compiled gettext MO catalogs. Consumes Blocks and emits a binary catalog keyed by (msgctxt, msgid). msgctxt is taken from Block.Properties[\"context\"] when present, else Block.Name, else Block.ID.",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "mo",
			Extensions: []string{".mo"},
			MimeTypes:  []string{"application/x-gettext-translation"},
		},
	}
}
