package markdown

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the Markdown format's parameters.
func (c *Config) Schema() *schema.FilterSchema {
	return &schema.FilterSchema{
		Title:       "Markdown Format",
		Description: "Configuration for the Markdown format reader/writer",
		Type:        "object",
		FilterMeta: schema.FilterSchemaMeta{
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
			"translateCodeBlocks": {
				Type:        "boolean",
				Default:     false,
				Description: "If true, fenced/indented code blocks are translatable. If false, emitted as non-translatable data.",
			},
			"translateFrontMatter": {
				Type:        "boolean",
				Default:     false,
				Description: "If true, YAML front matter values are translatable. If false, emitted as non-translatable data.",
			},
			"translateImageAlt": {
				Type:        "boolean",
				Default:     true,
				Description: "If true, image alt text is included in translatable content.",
			},
			"translateURLs": {
				Type:        "boolean",
				Default:     false,
				Description: "If true, link and image URLs are translatable.",
			},
			"translateBlockQuotes": {
				Type:        "boolean",
				Default:     true,
				Description: "If true, blockquote content is translatable.",
			},
			"translateHTMLBlocks": {
				Type:        "boolean",
				Default:     false,
				Description: "If true, raw HTML blocks are translatable. If false, emitted as non-translatable data.",
			},
		},
	}
}
