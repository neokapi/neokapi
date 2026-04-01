package properties

import (
	coreschema "github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the Java Properties format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Java Properties Format",
		Description: "Configuration for the Java .properties format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "properties",
			Extensions: []string{".properties"},
			MimeTypes:  []string{"text/x-java-properties"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "parsing",
				Label: "Parsing",
				Fields: []string{
					"separator",
					"useJavaEscapes",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"separator": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     "=",
				Description: "Key-value separator character (typically '=' or ':')",
			}),
			"useJavaEscapes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Description: "Decode additional Java escapes (\\: \\= \\# \\!) in values",
			}),
		},
	}
}
