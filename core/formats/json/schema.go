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
				Title:       "Extract all key/string pairs",
				Default:     true,
				Description: "Extract all key-value pairs for translation. When false, use extractionRules to specify which paths to extract.",
			}),
			"exceptions": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Key exception pattern",
				Description: "Regex pattern for key names. When extractAllPairs is true, matching keys are excluded. When false, matching keys are included.",
				Widget:      "regex",
			}),
			"extractIsolatedStrings": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract strings without associated key",
				Default:     false,
				Description: "Extract string values in arrays (without keys) as translatable blocks.",
			}),
			"extractionRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Extraction rules",
				Description: "Regex pattern for JSON paths to extract. Overrides extractAllPairs when specified.",
				Widget:      "regex",
				Placeholder: "/messages/.*|/labels/.*",
			}),
			"useKeyAsName": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Use the key as the resname",
				Default:     true,
				Description: "Use JSON key as the Block (TextUnit) name for reference.",
			}),
			"useFullKeyPath": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Use the full key path",
				Default:     false,
				Description: "Use full path (/key1/key2) instead of immediate key as name.",
			}),
			"useLeadingSlashOnKeyPath": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Include leading \"/\" on key path",
				Default:     true,
				Description: "Include leading slash in key path names.",
			}),
			"idRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "ID rules",
				Description: "Regex for keys whose values become Block IDs (overrides useKeyAsName).",
				Widget:      "regex",
				Placeholder: "/id|/key",
			}),
			"useIdStack": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Use the full path of IDs for resname",
				Default:     false,
				Description: "Build TextUnit IDs from nested key stack.",
			}),
			"escapeForwardSlashes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Escape forward slashes",
				Default:     true,
				Description: "Escape forward slashes in output JSON (\\/).",
			}),
			"subfilters": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Subfilter mappings",
				Description: "Array of {pattern, format} mappings for embedded content in specific keys.",
			}),
			"subfilter": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Subfilter format",
				Description: "Global subfilter format name (e.g., 'html') applied to all or matching extracted strings.",
			}),
			"subfilterRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Sub-filter rules",
				Description: "Regex matching keys for which values should be processed with the sub-filter.",
				Widget:      "regex",
				Visible:     &coreschema.ConditionExpr{Field: "subfilter", Empty: new(false)},
			}),
			"noteRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Note rules",
				Description: "Regex for keys whose values become translator notes.",
				Widget:      "regex",
				Placeholder: "/description|/comment",
			}),
			"genericMetaRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Generic metadata rules",
				Description: "Regex for keys whose values become generic metadata.",
				Widget:      "regex",
			}),
			"maxwidthRules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Maxwidth rules",
				Description: "Regex for keys whose values set max width constraints.",
				Widget:      "regex",
			}),
			"maxwidthSizeUnit": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Maxwidth size unit",
				Default:     "pixel",
				Description: "Size unit for maxwidth.",
				Options:     []coreschema.OptionItem{{Value: "pixel", Label: "Pixel"}, {Value: "char", Label: "Character"}},
			}),
			"useCodeFinder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Enable Inline Code Detection",
				Default:     false,
				Description: "Enable pattern-based detection of inline codes (placeholders, tags, etc.).",
			}),
			"codeFinderRules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within translatable text.",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "useCodeFinder", Eq: true},
			}),
		},
	}
}
