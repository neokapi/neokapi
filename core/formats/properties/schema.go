package properties

import "github.com/gokapi/gokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the Java Properties format's parameters.
func (c *Config) Schema() *schema.FilterSchema {
	return &schema.FilterSchema{
		Title:       "Java Properties Format",
		Description: "Configuration for the Java .properties format reader/writer",
		Type:        "object",
		FilterMeta: schema.FilterSchemaMeta{
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
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"separator": {
				Type:        "string",
				Default:     "=",
				Description: "Key-value separator character (typically '=' or ':')",
			},
		},
	}
}
