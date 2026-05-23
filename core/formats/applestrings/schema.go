package applestrings

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the Apple Strings format's
// parameters. The format is structure-driven, so it exposes only a couple of
// behavioural toggles.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Apple Strings",
		Description: "Configuration for the native Apple Strings (.strings + .stringsdict) reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "applestrings",
			Extensions: []string{".strings", ".stringsdict"},
			MimeTypes:  []string{"text/plain"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"extractComments", "protectPlaceholders",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"extractComments": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract comments as notes",
				Default:     true,
				Description: "Surface a /* ... */ or // ... comment preceding a .strings entry as a translator note on the emitted block.",
			}),
			"protectPlaceholders": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Protect format placeholders",
				Default:     true,
				Description: "Lift printf-style specifiers (%@, %lld, %1$@) and .stringsdict %#@var@ tokens into inline codes so they are never translated.",
			}),
		},
	}
}
