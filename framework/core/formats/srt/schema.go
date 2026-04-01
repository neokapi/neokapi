package srt

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the SRT format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "SRT Subtitles",
		Description: "SubRip subtitle format — no configurable parameters",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "srt",
			Extensions: []string{".srt"},
			MimeTypes:  []string{"application/x-subrip"},
		},
	}
}
