package odf

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the ODF format's parameters.
// The exposed keys mirror exactly those accepted by Config.ApplyMap.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Open Document Format",
		Description: "Configuration for the OpenDocument (ODF) format reader/writer (odt, ods, odp)",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "odf",
			Extensions: []string{".odt", ".ods", ".odp", ".odg", ".odf"},
			MimeTypes: []string{
				"application/vnd.oasis.opendocument.text",
				"application/vnd.oasis.opendocument.spreadsheet",
				"application/vnd.oasis.opendocument.presentation",
			},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"translateNotes", "translateHiddenContent",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"translateNotes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Translate Notes",
				Description: "Extract presentation notes and annotations for translation",
			}),
			"translateHiddenContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate Hidden Content",
				Description: "Extract hidden content (hidden text, hidden slides) for translation",
			}),
		},
	}
}
