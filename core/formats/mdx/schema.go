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
					"extract_non_translatable_content",
					"translate_code_blocks", "translate_front_matter", "front_matter_keys",
					"translate_image_alt", "translate_urls",
					"translate_block_quotes", "translate_html_blocks",
				},
			},
			{
				ID:     "inlineCodes",
				Label:  "Inline Codes",
				Fields: []string{"use_code_finder", "code_finder_rules"},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"extract_non_translatable_content": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Surface Non-Translatable Content",
				Description: "If true, MDX-specific non-translatable content (block-level JSX text children, GFM table cell prose, and markdown spans that cannot be reconstructed faithfully) is surfaced as non-translatable content blocks (visible to ingestion, skipped by MT) while staying byte-faithful on round-trip. If false, those constructs stay opaque.",
			}),
			"translate_code_blocks": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate Code Blocks",
				Description: "If true, fenced and indented code blocks are translatable. If false, emitted as non-translatable data.",
			}),
			"translate_front_matter": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate YAML Front Matter",
				Description: "If true, YAML front matter values are translatable. If false, emitted as non-translatable data.",
			}),
			"front_matter_keys": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Front Matter Keys",
				Description: "Front matter keys to extract when translate_front_matter is on (empty = every scalar value). Set the prose-bearing keys, e.g. title and description.",
				Visible:     &coreschema.ConditionExpr{Field: "translate_front_matter", Eq: true},
			}),
			"translate_image_alt": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Translate Image Alt Text",
				Description: "If true, image alt text is included in translatable content.",
			}),
			"translate_urls": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate Hyperlink URLs",
				Description: "If true, link and image URLs are translatable.",
			}),
			"translate_block_quotes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Translate Block Quotes",
				Description: "If true, blockquote content is translatable.",
			}),
			"translate_html_blocks": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate HTML Blocks",
				Description: "If true, raw HTML blocks are translatable. If false, emitted as non-translatable data.",
			}),
			"use_code_finder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Inline Code Detection",
				Description: "Enable regex-based inline code detection within translatable text.",
			}),
			"code_finder_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within translatable text.",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "use_code_finder", Eq: true},
			}),
		},
	}
}
