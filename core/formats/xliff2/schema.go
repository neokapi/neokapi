package xliff2

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the XLIFF 2.0 format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "XLIFF 2.0",
		Description: "Configuration for the native XLIFF 2.0/2.1 bilingual exchange format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "xliff2",
			Extensions: []string{".xlf", ".xliff"},
			MimeTypes:  []string{"application/xliff+xml"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"forceUniqueIds", "ignoreTagTypeMatch",
				},
			},
			{
				ID:    "states",
				Label: "Translation State Handling",
				Fields: []string{
					"discardInvalidTargets",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"writeOriginalData",
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
			"forceUniqueIds": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Ensure unique tag IDs",
				Description: "Ensure inline tag IDs are unique within each unit",
			}),
			"ignoreTagTypeMatch": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Ignore tag type mismatch",
				Description: "Ignore tag type mismatch between source and target segments",
			}),

			// States
			"discardInvalidTargets": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Discard invalid targets",
				Description: "Discard targets that fail validation rather than rejecting the entire file",
			}),

			// Output
			"writeOriginalData": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Include original data in output",
				Description: "Output includes original data when available",
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
