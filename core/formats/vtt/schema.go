package vtt

import "github.com/gokapi/gokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the WebVTT format.
func (c *Config) Schema() *schema.FilterSchema {
	return &schema.FilterSchema{
		Title:       "WebVTT Subtitles",
		Description: "Web Video Text Tracks format — no configurable parameters",
		Type:        "object",
		FilterMeta: schema.FilterSchemaMeta{
			ID:         "vtt",
			Extensions: []string{".vtt"},
			MimeTypes:  []string{"text/vtt"},
		},
	}
}
