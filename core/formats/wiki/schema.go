package wiki

import (
	"github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// Schema returns the JSON Schema metadata for the Wiki format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Wiki Filter",
		Description: "Configuration for the Wiki (MediaWiki/DokuWiki) format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "wiki",
			Extensions: []string{".wiki", ".mediawiki"},
			MimeTypes:  []string{"text/x-wiki"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "syntax",
				Label: "Syntax",
				Fields: []string{
					"variant", "preserveWhitespace", "extractNonTranslatableContent",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"variant": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Wiki variant",
				Default:     "dokuwiki",
				Description: "Wiki markup variant: mediawiki or dokuwiki. Default is dokuwiki to match the okf_wiki bridge contract.",
				Options: []coreschema.OptionItem{
					{Value: "mediawiki", Label: "MediaWiki"},
					{Value: "dokuwiki", Label: "DokuWiki"},
				},
			}),
			"preserveWhitespace": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Preserve Whitespace",
				Default:     false,
				Description: "Preserve original whitespace in wiki markup instead of normalizing it.",
			}),
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable contextual content such as DokuWiki <code>/<file>/<html>/<php> block bodies and indented code blocks is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep it in skeleton.",
			}),
		},
	}
}
