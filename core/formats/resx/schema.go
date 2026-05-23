package resx

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the .NET RESX / .resw format's
// parameters. The format is schema-driven, so it exposes only a couple of
// behavioural toggles.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       ".NET RESX",
		Description: "Configuration for the native .NET RESX / .resw (Microsoft ResX 2.0) reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "resx",
			Extensions: []string{".resx", ".resw"},
			MimeTypes:  []string{"text/microsoft-resx"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"extractComments", "skipNameDataReferences",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"extractComments": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract comments as notes",
				Default:     true,
				Description: "Surface each string <data> entry's sibling <comment> element as a translator note on the emitted block. Comments always round-trip verbatim regardless of this setting.",
			}),
			"skipNameDataReferences": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Skip name-reference entries",
				Default:     true,
				Description: "Exclude designer name-reference entries (those whose name starts with \">\", e.g. \">>control.Name\") from extraction. These carry WinForms field names, not UI strings.",
			}),
		},
	}
}
