package xliff

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the XLIFF 1.2 format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "XLIFF 1.2",
		Description: "Configuration for the native XLIFF 1.2 bilingual exchange format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "xliff",
			Extensions: []string{".xlf", ".xliff"},
			MimeTypes:  []string{"application/xliff+xml"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction",
				Fields: []string{
					"preserve_space_by_default", "include_extensions", "include_its",
					"ignore_input_segmentation", "fallback_to_id", "force_unique_ids",
				},
			},
			{
				ID:    "states",
				Label: "Translation State Handling",
				Fields: []string{
					"use_translation_target_state", "target_state_value",
					"edit_alt_trans", "add_alt_trans",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"add_target_language", "override_target_language",
					"allow_empty_targets", "always_add_targets",
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
			// Extraction
			"preserve_space_by_default": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Preserve whitespace by default",
				Description: "Treat all content as xml:space=\"preserve\" unless explicitly overridden",
			}),
			"include_extensions": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Include XLIFF extension elements",
				Description: "Include non-standard namespace extension elements in extracted content",
			}),
			"include_its": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Include ITS markup",
				Description: "Include ITS (Internationalization Tag Set) markup in extracted content",
			}),
			"ignore_input_segmentation": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Ignore input segmentation",
				Description: "Ignore seg-source/mrk segmentation from the input XLIFF and treat the full source as a single segment",
			}),
			"fallback_to_id": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Use trans-unit ID as name if no resname",
				Description: "Use the trans-unit id attribute as the block name when no resname attribute is present",
			}),
			"force_unique_ids": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enforce unique trans-unit IDs",
				Description: "Rewrite trans-unit IDs to enforce uniqueness across the file",
			}),

			// States
			"use_translation_target_state": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Update target state attribute",
				Description: "Update the state attribute on target elements when writing translated XLIFF",
			}),
			"target_state_value": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     "needs-translation",
				Title:       "Target state value",
				Description: "State value to set on target elements when use_translation_target_state is enabled",
			}),
			"edit_alt_trans": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Edit alt-trans elements",
				Description: "Allow editing of existing alt-trans elements",
			}),
			"add_alt_trans": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Add alt-trans elements",
				Description: "Allow addition of new alt-trans elements",
			}),

			// Output
			"add_target_language": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Add target-language attribute",
				Description: "Add the target-language attribute to file elements if not already present",
			}),
			"override_target_language": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Override target language",
				Description: "Override the target-language attribute in the output document with the pipeline target locale",
			}),
			"allow_empty_targets": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Allow empty targets",
				Description: "Allow target elements with empty content to be written",
			}),
			"always_add_targets": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Add targets for monolingual XLIFF",
				Description: "Add target elements for monolingual XLIFF files that have no target in the source",
			}),

			// Inline codes
			"use_code_finder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable inline code detection",
				Description: "Enable regex-based inline code detection in translatable text",
			}),
			"code_finder_rules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code finder rules",
				Description: "Regex patterns that match inline codes within translatable text",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "use_code_finder", Eq: true},
			}),
		},
	}
}
