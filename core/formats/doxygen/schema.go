package doxygen

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the Doxygen format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Doxygen Comments",
		Description: "Extracts translatable text from Doxygen/Javadoc comments in source code",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "doxygen",
			Extensions: []string{".c", ".cpp", ".h", ".java", ".m", ".py"},
			MimeTypes:  []string{"text/x-doxygen-txt"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "parser",
				Label: "Parser Settings",
				Fields: []string{
					"preserveWhitespace", "extractNonTranslatableContent",
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
			// Parser
			"preserveWhitespace": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Preserve whitespace",
				Description: "Preserve original whitespace in extracted comment text instead of normalizing it",
			}),
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable contextual content such as \\code…\\endcode and \\verbatim…\\endverbatim region bodies is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep it in skeleton.",
			}),

			// Inline codes
			"useCodeFinder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable inline code detection",
				Description: "Enable regex-based detection of inline codes within translatable text",
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
