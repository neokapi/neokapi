package androidxml

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the Android string-resources
// reader/writer parameters. The format is schema-driven, so it exposes only a
// few behavioural toggles.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Android String Resources",
		Description: "Configuration for the native Android string-resources (res/values/strings.xml) reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "androidxml",
			Extensions: []string{".xml"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"extractComments", "skipNonTranslatable", "skipResourceReferences",
					"extractNonTranslatableContent",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"extractComments": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract comments as notes",
				Default:     true,
				Description: "Surface an XML comment immediately preceding an entry as a translator note on the emitted block. Comments always round-trip verbatim regardless of this setting.",
			}),
			"skipNonTranslatable": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Skip translatable=\"false\" entries",
				Default:     true,
				Description: "Exclude <string>/<string-array>/<plurals> entries marked translatable=\"false\" from extraction. Such resources are developer-owned and round-trip verbatim.",
			}),
			"skipResourceReferences": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Skip resource references",
				Default:     true,
				Description: "Exclude <string> values that are a bare resource reference (e.g. @string/foo, ?attr/bar). A reference is an alias, not translatable UI text.",
			}),
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Surface non-translatable content",
				Default:     true,
				Description: "Surface <string>/<string-array>/<plurals> entries marked translatable=\"false\" as non-translatable content blocks (visible to ingestion, skipped by MT). When off, such entries stay in opaque skeleton and round-trip verbatim. Bare resource references are always skeleton.",
			}),
		},
	}
}
