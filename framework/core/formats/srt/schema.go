package srt

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the SRT format.
func (c *Config) Schema() *schema.FilterSchema {
	return &schema.FilterSchema{
		Title:       "SRT Subtitles",
		Description: "SubRip subtitle format — no configurable parameters",
		Type:        "object",
		FilterMeta: schema.FilterSchemaMeta{
			ID:         "srt",
			Extensions: []string{".srt"},
			MimeTypes:  []string{"application/x-subrip"},
		},
	}
}
