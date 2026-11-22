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
				"use_full_key_path": true,
				"subfilter":         "html",
				"subfilter_rules":   "_html$",
			},
			"Chrome Extension": {
				"extract_all_pairs": false,
				"exceptions":        "^message$",
				"note_rules":        "^description$",
			},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"extract_all_pairs", "exceptions", "extract_isolated_strings",
					"extract_non_translatable_content", "extraction_rules",
				},
			},
			{
				ID:    "naming",
				Label: "Block Naming",
				Fields: []string{
					"use_key_as_name", "use_full_key_path", "use_leading_slash_on_key_path",
					"id_rules", "use_id_stack",
				},
			},
			{
				ID:    "subfilters",
				Label: "Subfilters",
				Fields: []string{
					"subfilters", "subfilter", "subfilter_rules",
				},
			},
			{
				ID:    "metadata",
				Label: "Metadata",
				Fields: []string{
					"note_rules", "generic_meta_rules", "maxwidth_rules", "maxwidth_size_unit",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"escape_forward_slashes",
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
			"extract_all_pairs": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract all key/string pairs",
				Default:     true,
				Description: "Extract all key-value pairs for translation. When false, use extraction_rules to specify which paths to extract.",
			}),
			"exceptions": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Key exception pattern",
				Description: "Regex pattern for key names. When extract_all_pairs is true, matching keys are excluded. When false, matching keys are included.",
				Widget:      "regex",
			}),
			"extract_isolated_strings": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract strings without associated key",
				Default:     false,
				Description: "Extract string values in arrays (without keys) as translatable blocks.",
			}),
			"extract_non_translatable_content": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable string content (isolated array strings and values excluded by the extraction rules) is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep it in skeleton.",
			}),
			"extraction_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Extraction rules",
				Description: "Regex pattern for JSON paths to extract. Overrides extract_all_pairs when specified.",
				Widget:      "regex",
				Placeholder: "/messages/.*|/labels/.*",
			}),
			"use_key_as_name": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Use the key as the resname",
				Default:     true,
				Description: "Use JSON key as the Block (TextUnit) name for reference.",
			}),
			"use_full_key_path": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Use the full key path",
				Default:     false,
				Description: "Use full path (/key1/key2) instead of immediate key as name.",
			}),
			"use_leading_slash_on_key_path": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Include leading \"/\" on key path",
				Default:     true,
				Description: "Include leading slash in key path names.",
			}),
			"id_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "ID rules",
				Description: "Regex for keys whose values become Block IDs (overrides use_key_as_name).",
				Widget:      "regex",
				Placeholder: "/id|/key",
			}),
			"use_id_stack": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Use the full path of IDs for resname",
				Default:     false,
				Description: "Build TextUnit IDs from nested key stack.",
			}),
			"escape_forward_slashes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Escape forward slashes",
				Default:     true,
				Description: "Escape forward slashes (\\/) in translated values, and in edited values of a document that spells no slash.",
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
			"subfilter_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Sub-filter rules",
				Description: "Regex matching keys for which values should be processed with the sub-filter.",
				Widget:      "regex",
				Visible:     &coreschema.ConditionExpr{Field: "subfilter", Empty: new(false)},
			}),
			"note_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Note rules",
				Description: "Regex for keys whose values become translator notes.",
				Widget:      "regex",
				Placeholder: "/description|/comment",
			}),
			"generic_meta_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Generic metadata rules",
				Description: "Regex for keys whose values become generic metadata.",
				Widget:      "regex",
			}),
			"maxwidth_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Maxwidth rules",
				Description: "Regex for keys whose values set max width constraints.",
				Widget:      "regex",
			}),
			"maxwidth_size_unit": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Maxwidth size unit",
				Default:     "pixel",
				Description: "Size unit for maxwidth.",
				Options:     []coreschema.OptionItem{{Value: "pixel", Label: "Pixel"}, {Value: "char", Label: "Character"}},
			}),
			"use_code_finder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Enable Inline Code Detection",
				Default:     false,
				Description: "Enable pattern-based detection of inline codes (placeholders, tags, etc.).",
			}),
			"code_finder_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within translatable text.",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "use_code_finder", Eq: true},
			}),
		},
	}
}
