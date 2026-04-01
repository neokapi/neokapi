package ttml

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the TTML format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "TTML Subtitles",
		Description: "Timed Text Markup Language (W3C standard) subtitle format",
		Type:        "object",
		FormatMeta: schema.FormatSchemaMeta{
			ID:         "ttml",
			Extensions: []string{".ttml", ".dfxp"},
			MimeTypes:  []string{"application/ttml+xml"},
		},
		Properties: map[string]schema.PropertySchema{
			"mergeAdjacentCaptions": {
				Type:        "boolean",
				Description: "Merge adjacent <p> elements ending with trailing punctuation into one block",
				Default:     false,
			},
			"escapeBR": {
				Type:        "boolean",
				Description: "When true, <br/> elements are removed and text is joined with spaces; when false, <br/> is preserved as literal text",
				Default:     true,
			},
		},
	}
}
