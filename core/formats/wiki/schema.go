package wiki

import (
	coreschema "github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/format/schema"
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
					"variant", "preserveWhitespace",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"variant": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Wiki variant",
				Default:     "mediawiki",
				Description: "Wiki markup variant: mediawiki or dokuwiki.",
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
		},
	}
}
