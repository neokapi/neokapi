package markdown

import (
	coreschema "github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the Markdown format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Markdown Format",
		Description: "Configuration for the Markdown format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "markdown",
			Extensions: []string{".md", ".markdown"},
			MimeTypes:  []string{"text/markdown"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Content Extraction",
				Fields: []string{
					"translateCodeBlocks", "translateFrontMatter",
					"translateImageAlt", "translateURLs",
					"translateBlockQuotes", "translateHTMLBlocks",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"translateCodeBlocks": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Description: "If true, fenced/indented code blocks are translatable. If false, emitted as non-translatable data.",
			}),
			"translateFrontMatter": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Description: "If true, YAML front matter values are translatable. If false, emitted as non-translatable data.",
			}),
			"translateImageAlt": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Description: "If true, image alt text is included in translatable content.",
			}),
			"translateURLs": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Description: "If true, link and image URLs are translatable.",
			}),
			"translateBlockQuotes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Description: "If true, blockquote content is translatable.",
			}),
			"translateHTMLBlocks": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Description: "If true, raw HTML blocks are translatable. If false, emitted as non-translatable data.",
			}),
		},
	}
}
