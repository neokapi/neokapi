package designtokens

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the design-tokens format's
// parameters. The DTCG layout is fixed by the specification, so the format
// exposes only the single localization-scoping toggle on top of the generic
// JSON reader it delegates to.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Design Tokens (DTCG)",
		Description: "Configuration for the W3C Design Tokens Community Group (DTCG) token format. Only $description documentation is translatable; token values and structure pass through untouched.",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID: formatID,
			// The unique .tokens extension is claimed. The generic .json
			// extension and application/json MIME are NOT claimed: they are
			// owned by the json format and DTCG files commonly use the
			// .tokens.json double extension that resolves to json.
			Extensions: []string{formatExt},
			MimeTypes:  []string{},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"extractDescriptions",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"extractDescriptions": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract $description documentation",
				Default:     true,
				Description: "Extract $description values (human-readable token and group documentation) as translatable blocks. This is the only natural-language field in DTCG; $value is always a typed design value (colour, dimension, font name, …) and is never extracted. Disable to read the file as fully non-translatable structure.",
			}),
		},
	}
}
