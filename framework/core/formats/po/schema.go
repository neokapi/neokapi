package po

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the PO format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "PO Format (GNU Gettext)",
		Description: "Configuration for the PO/POT format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "po",
			Extensions: []string{".po", ".pot"},
			MimeTypes:  []string{"text/x-gettext-translation"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"preserveUntranslated",
				},
			},
			{
				ID:    "mode",
				Label: "Processing Mode",
				Fields: []string{
					"bilingualMode",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"wrapContent", "allowEmptyOutputTarget",
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
			"preserveUntranslated": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Preserve Untranslated Entries",
				Description: "If true, entries with empty msgstr are emitted as blocks. If false, they are skipped.",
			}),
			"bilingualMode": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Bilingual Mode",
				Description: "In bilingual mode (default), msgid is the source text and msgstr is the target. In monolingual mode, msgid is treated as an ID.",
			}),
			"wrapContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Wrap Long Content Lines",
				Description: "Wrap long content lines in output at approximately 80 characters",
			}),
			"allowEmptyOutputTarget": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Allow Empty Target Output",
				Description: "Allow empty target in output when no translation is available",
			}),
			"useCodeFinder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Inline Code Detection",
				Description: "Enable regex-based inline code detection within PO message text",
			}),
			"codeFinderRules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within translatable text",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "useCodeFinder", Eq: true},
			}),
		},
	}
}
