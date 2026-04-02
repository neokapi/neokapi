package tmx

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the TMX format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "TMX (Translation Memory eXchange)",
		Description: "Configuration for the native TMX translation memory format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "tmx",
			Extensions: []string{".tmx"},
			MimeTypes:  []string{"application/x-tmx+xml"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"processAllTargets", "exitOnInvalid",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"escapeGT",
				},
			},
			{
				ID:    "inlineCodes",
				Label: "Inline Codes",
				Fields: []string{
					"useCodeFinder", "codeFinderRules",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			// Extraction
			"processAllTargets": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Read all target entries",
				Description: "Read all target language TUVs from each TU. When false, only the first target is read.",
			}),
			"exitOnInvalid": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Exit on invalid TUs",
				Description: "Stop processing when encountering invalid TUs. When false, invalid TUs are skipped.",
			}),

			// Output
			"escapeGT": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Escape greater-than characters",
				Description: "Escape > as &gt; in output XML",
			}),

			// Inline codes
			"useCodeFinder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable inline code detection",
				Description: "Enable regex-based inline code detection in translatable text",
			}),
			"codeFinderRules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code finder rules",
				Description: "Regex patterns that match inline codes within translatable text",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "useCodeFinder", Eq: true},
			}),
		},
	}
}
