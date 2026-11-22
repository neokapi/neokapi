package vtt

import (
	"github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// Schema returns the JSON Schema metadata for the WebVTT format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "VTT Filter",
		Description: "Web Video Text Tracks (WebVTT) subtitle format",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "vtt",
			Extensions: []string{".vtt"},
			MimeTypes:  []string{"text/vtt"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Content Extraction",
				Fields: []string{
					"extract_non_translatable_content",
				},
			},
			{
				ID:    "output",
				Label: "Output settings",
				Fields: []string{
					"max_chars_per_line", "max_lines_per_caption",
					"cjk_chars_per_line", "split_words",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"extract_non_translatable_content": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable contextual content such as a STYLE block's embedded CSS is surfaced as a content block (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden. Disable to keep the legacy opaque behavior.",
			}),
			"max_chars_per_line": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Title:       "Max characters per line",
				Description: "Maximum number of characters per line in output. Set to 0 to disable.",
				Default:     0,
			}),
			"max_lines_per_caption": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Title:       "Max lines per caption",
				Description: "Maximum number of lines per caption in output. Set to 0 to disable.",
				Default:     0,
			}),
			"cjk_chars_per_line": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Title:       "Max characters per line for CJK",
				Description: "Maximum number of characters per line for CJK languages. Set to 0 to disable.",
				Default:     0,
			}),
			"split_words": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Split words at character limit",
				Description: "Split words so that they don't go over the character limit per line.",
				Default:     false,
			}),
		},
	}
}
