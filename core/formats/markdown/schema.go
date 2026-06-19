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
					"translateCodeBlocks", "extractNonTranslatableContent",
					"translateFrontMatter", "frontMatterKeys",
					"translateImageAlt", "translateURLs",
					"translateBlockQuotes", "translateHTMLBlocks",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"unescapeBackslashCharacters",
				},
			},
			{
				ID:    "subfilters",
				Label: "Subfilters",
				Fields: []string{
					"subfilter",
				},
			},
			{
				ID:    "inlineCodes",
				Label: "Inline Codes",
				Fields: []string{
					"useCodeFinder", "codeFinderRules",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"translateCodeBlocks": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate Code Blocks",
				Description: "If true, fenced and indented code blocks are translatable. If false, emitted as non-translatable data.",
			}),
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable contextual content such as code blocks is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep it in skeleton.",
			}),
			"translateFrontMatter": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate YAML Front Matter",
				Description: "If true, YAML front matter values are translatable. If false, emitted as non-translatable data.",
			}),
			"frontMatterKeys": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Front Matter Keys",
				Description: "Front matter keys to extract when translateFrontMatter is on (empty = every scalar value). Set the prose-bearing keys, e.g. title and description.",
				Visible:     &coreschema.ConditionExpr{Field: "translateFrontMatter", Eq: true},
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
			"unescapeBackslashCharacters": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Support Backslash Escaping",
				Description: "Parse backslash-escaped punctuation in source documents",
			}),
			"subfilter": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "HTML Subfilter",
				Description: "Subfilter format to apply to HTML content within Markdown (e.g., 'html')",
			}),
			"useCodeFinder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Inline Code Detection",
				Description: "Enable regex-based inline code detection within translatable text",
			}),
			"codeFinderRules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within translatable text",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "useCodeFinder", Eq: true},
			}),
		},
	}
}
