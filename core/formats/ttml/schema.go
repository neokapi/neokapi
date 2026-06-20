package ttml

import (
	"github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// Schema returns the JSON Schema metadata for the TTML format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "TTML Filter",
		Description: "Timed Text Markup Language (W3C standard) subtitle format",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "ttml",
			Extensions: []string{".ttml", ".dfxp"},
			MimeTypes:  []string{"application/ttml+xml"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"mergeAdjacentCaptions", "escapeBR",
					"extractNonTranslatableContent",
				},
			},
			{
				ID:    "output",
				Label: "Output settings",
				Fields: []string{
					"maxCharsPerLine", "maxLinesPerCaption",
					"cjkCharsPerLine", "splitWords",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"mergeAdjacentCaptions": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Merge adjacent captions",
				Description: "Merge adjacent <p> elements ending with trailing punctuation into one block.",
				Default:     false,
			}),
			"escapeBR": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Escape line breaks",
				Description: "When true, <br/> elements are removed and text is joined with spaces; when false, <br/> is preserved as literal text.",
				Default:     true,
			}),
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable head metadata such as ttm:copyright and ttm:agent is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep it in skeleton.",
			}),
			"maxCharsPerLine": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Title:       "Max characters per line",
				Description: "Maximum number of characters per line in output. Set to 0 to disable.",
				Default:     0,
			}),
			"maxLinesPerCaption": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Title:       "Max lines per caption",
				Description: "Maximum number of lines per caption in output. Set to 0 to disable.",
				Default:     0,
			}),
			"cjkCharsPerLine": schema.Prop(coreschema.PropertySchema{
				Type:        "integer",
				Title:       "Max characters per line for CJK",
				Description: "Maximum number of characters per line for CJK languages. Set to 0 to disable.",
				Default:     0,
			}),
			"splitWords": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Split words at character limit",
				Description: "Split words so that they don't go over the character limit per line.",
				Default:     false,
			}),
		},
	}
}
