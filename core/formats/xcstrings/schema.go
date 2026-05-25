package xcstrings

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the Apple String Catalog
// format's parameters. The format is schema-driven, so it exposes only a
// couple of behavioural toggles.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Apple String Catalog",
		Description: "Configuration for the native Apple String Catalog (.xcstrings) reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "xcstrings",
			Extensions: []string{".xcstrings"},
			MimeTypes:  []string{"application/json"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"extractStale", "markTranslatedState",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"extractStale": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract stale entries",
				Default:     true,
				Description: "Emit entries whose extractionState is \"stale\" (the source string no longer appears in the code base) as translatable blocks.",
			}),
			"markTranslatedState": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "State for new translations",
				Default:     "translated",
				Description: "The stringUnit state value written for a localization populated for the first time. Existing states are preserved verbatim on round-trip.",
			}),
		},
	}
}
