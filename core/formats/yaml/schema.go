package yaml

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the YAML format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "YAML Format",
		Description: "Configuration for the native YAML format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "yaml",
			Extensions: []string{".yaml", ".yml"},
			MimeTypes:  []string{"application/x-yaml", "text/yaml"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"extract_non_strings", "key_path_patterns",
				},
			},
			{
				ID:    "subfilters",
				Label: "Subfilters",
				Fields: []string{
					"subfilter",
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
			"extract_non_strings": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Extract Non-String Values",
				Description: "Extract non-string scalar values (booleans, numbers, nulls) as translatable blocks",
			}),
			"key_path_patterns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Key Path Patterns",
				Description: "When non-empty, only keys matching one of these glob patterns are extracted. Supports * (single key) and ** (any depth).",
			}),
			"subfilter": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Subfilter Format",
				Description: "Sub-filter to apply to scalar values (e.g., 'html' to process HTML within YAML values)",
			}),
			"use_code_finder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Inline Code Detection",
				Description: "Enable regex-based inline code detection within string values",
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
