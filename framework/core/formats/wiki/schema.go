package wiki

import (
	coreschema "github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the Wiki format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Wiki Format",
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
					"variant",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"variant": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     "mediawiki",
				Description: "Wiki markup variant: mediawiki or dokuwiki.",
			}),
		},
	}
}
