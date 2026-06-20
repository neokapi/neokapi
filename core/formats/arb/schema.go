package arb

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the Flutter ARB format's
// parameters. The format is structure-driven, so it exposes only a single
// behavioural toggle.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Flutter ARB",
		Description: "Configuration for the native Flutter Application Resource Bundle (.arb) reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "arb",
			Extensions: []string{".arb"},
			MimeTypes:  []string{"application/json"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"descriptionNotes",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"descriptionNotes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Surface descriptions as notes",
				Default:     true,
				Description: "Surface the human-facing context in a resource's sibling \"@<id>\" attributes object as developer notes: the resource \"description\" plus each placeholder's \"example\"/\"description\" hint. The attributes object is always preserved byte-faithfully on round-trip regardless of this setting.",
			}),
		},
	}
}
