package xml

import (
	coreschema "github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the XML format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "XML Format",
		Description: "Configuration for the generic XML format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "xml",
			Extensions: []string{".xml"},
			MimeTypes:  []string{"text/xml", "application/xml"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction Rules",
				Fields: []string{
					"translatableElements", "translatableAttributes",
				},
			},
			{
				ID:    "subfilters",
				Label: "Subfilters",
				Fields: []string{
					"subfilters",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"translatableElements": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Description: "Element names whose text content is translatable. If empty, all text content is translatable.",
			}),
			"translatableAttributes": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Description: "Attribute names that are translatable.",
			}),
			"subfilters": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Description: "Array of {pattern, format} mappings for embedded content. Patterns use dot-separated element paths with glob support.",
			}),
		},
	}
}
