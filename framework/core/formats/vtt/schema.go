package vtt

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the WebVTT format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "WebVTT Subtitles",
		Description: "Web Video Text Tracks format — no configurable parameters",
		Type:        "object",
		FormatMeta: schema.FormatSchemaMeta{
			ID:         "vtt",
			Extensions: []string{".vtt"},
			MimeTypes:  []string{"text/vtt"},
		},
	}
}
