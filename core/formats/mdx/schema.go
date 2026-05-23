package mdx

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the MDX format's parameters.
// MDX honours the same Markdown-prose extraction toggles as the markdown
// format (they are forwarded to the delegated markdown reader); the
// MDX-specific constructs (ESM, JSX, expressions) are always opaque and
// have no parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "MDX Format",
		Description: "Configuration for the MDX (.mdx) format reader/writer. MDX is CommonMark Markdown extended with ESM imports/exports, JSX components, and {expression} braces. Markdown prose is extracted using the same toggles as the Markdown format; ESM, JSX, and expressions are preserved verbatim and never translated.",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "mdx",
			Extensions: []string{".mdx"},
			MimeTypes:  []string{"text/mdx"},
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
			{
				ID:     "inlineCodes",
				Label:  "Inline Codes",
				Fields: []string{"useCodeFinder", "codeFinderRules"},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"translateCodeBlocks": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate Code Blocks",
				Description: "If true, fenced and indented code blocks are translatable. If false, emitted as non-translatable data.",
			}),
			"translateFrontMatter": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate YAML Front Matter",
				Description: "If true, YAML front matter values are translatable. If false, emitted as non-translatable data.",
			}),
			"translateImageAlt": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Translate Image Alt Text",
				Description: "If true, image alt text is included in translatable content.",
			}),
			"translateURLs": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate Hyperlink URLs",
				Description: "If true, link and image URLs are translatable.",
			}),
			"translateBlockQuotes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Translate Block Quotes",
				Description: "If true, blockquote content is translatable.",
			}),
			"translateHTMLBlocks": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate HTML Blocks",
				Description: "If true, raw HTML blocks are translatable. If false, emitted as non-translatable data.",
			}),
			"useCodeFinder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Inline Code Detection",
				Description: "Enable regex-based inline code detection within translatable text.",
			}),
			"codeFinderRules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within translatable text.",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "useCodeFinder", Eq: true},
			}),
		},
	}
}
