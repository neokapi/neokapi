package json

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the JSON format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "JSON Format",
		Description: "Configuration for the native JSON format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "json",
			Extensions: []string{".json"},
			MimeTypes:  []string{"application/json"},
		},
		Presets: map[string]map[string]any{
			"i18next": {
				"useFullKeyPath": true,
				"subfilter":      "html",
				"subfilterRules": "_html$",
			},
			"Chrome Extension": {
				"extractAllPairs": false,
				"exceptions":      "^message$",
				"noteRules":       "^description$",
			},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"extractAllPairs", "exceptions", "extractIsolatedStrings",
					"extractionRules",
				},
			},
			{
				ID:    "naming",
				Label: "Block Naming",
				Fields: []string{
					"useKeyAsName", "useFullKeyPath", "useLeadingSlashOnKeyPath",
					"idRules", "useIdStack",
				},
			},
			{
				ID:    "subfilters",
				Label: "Subfilters",
				Fields: []string{
					"subfilters", "subfilter", "subfilterRules",
				},
			},
			{
				ID:    "metadata",
				Label: "Metadata",
				Fields: []string{
					"noteRules", "genericMetaRules", "maxwidthRules", "maxwidthSizeUnit",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"escapeForwardSlashes",
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
			"extractAllPairs": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Description: "Extract all string key-value pairs as translatable blocks",
			}),
			"exceptions": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Description: "Regex pattern for key names. When extractAllPairs is true, matching keys are excluded. When false, matching keys are included.",
				Widget:      "regex",
			}),
			"extractIsolatedStrings": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Description: "Extract standalone string values inside arrays as translatable blocks",
			}),
			"useKeyAsName": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Description: "Use the JSON key as the block name/ID",
			}),
			"useFullKeyPath": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Description: "Use hierarchical paths (parent/child) as block names",
			}),
			"useLeadingSlashOnKeyPath": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Description: "Prepend / to full key paths",
			}),
			"escapeForwardSlashes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Description: "Escape / as \\/ in JSON output",
			}),
			"subfilters": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Description: "Array of {pattern, format} mappings for embedded content in specific keys",
			}),
			"subfilter": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Description: "Global subfilter format name (e.g., 'html') applied to all or matching extracted strings",
			}),
			"subfilterRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Description: "Regex pattern for key names processed by the subfilter. Only used when subfilter is set.",
				Widget:      "regex",
				Visible:     &coreschema.ConditionExpr{Field: "subfilter", Empty: boolPtr(false)},
			}),
			"noteRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Description: "Regex pattern for key names whose values become notes on the next translatable block",
				Widget:      "regex",
			}),
			"idRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Description: "Regex pattern for key names whose values are used as block names/IDs",
				Widget:      "regex",
			}),
			"useIdStack": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Description: "Stack IDs for nested structures, producing compound IDs",
			}),
			"genericMetaRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Description: "Regex pattern for key names whose values become metadata annotations",
				Widget:      "regex",
			}),
			"extractionRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Description: "Regex pattern limiting which keys are extracted. Only matching keys are extracted.",
				Widget:      "regex",
			}),
			"maxwidthRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Description: "Regex pattern for key names whose numeric values set MAX_WIDTH on the next block",
				Widget:      "regex",
			}),
			"maxwidthSizeUnit": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     "pixel",
				Description: "Size unit for maxwidth",
				Enum:        []any{"pixel", "char"},
			}),
			"useCodeFinder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Description: "Enable regex-based inline code detection",
			}),
			"codeFinderRules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Description: "Regex patterns that match inline codes within translatable text",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "useCodeFinder", Eq: true},
			}),
		},
	}
}

func boolPtr(v bool) *bool { return &v }
