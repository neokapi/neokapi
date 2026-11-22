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
					"preserve_untranslated",
				},
			},
			{
				ID:    "mode",
				Label: "Processing Mode",
				Fields: []string{
					"bilingual_mode",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"wrap_content", "allow_empty_output_target",
				},
			},
			{
				ID:    "inlineCodes",
				Label: "Inline Codes",
				Fields: []string{
					"use_code_finder", "code_finder_rules",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"preserve_untranslated": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Preserve Untranslated Entries",
				Description: "If true, entries with empty msgstr are emitted as blocks. If false, they are skipped.",
			}),
			"bilingual_mode": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Bilingual Mode",
				Description: "In bilingual mode (default), msgid is the source text and msgstr is the target. In monolingual mode, msgid is treated as an ID.",
			}),
			"wrap_content": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Wrap Long Content Lines",
				Description: "Wrap long content lines in output at approximately 80 characters",
			}),
			"allow_empty_output_target": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Allow Empty Target Output",
				Description: "Allow empty target in output when no translation is available",
			}),
			"use_code_finder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Inline Code Detection",
				Description: "Enable regex-based inline code detection within PO message text",
			}),
			"code_finder_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within translatable text",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "use_code_finder", Eq: true},
			}),
		},
	}
}
