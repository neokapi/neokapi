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
					"preserveSpaceByDefault", "includeExtensions", "includeIts",
					"ignoreInputSegmentation", "fallbackToID", "forceUniqueIds",
				},
			},
			{
				ID:    "states",
				Label: "Translation State Handling",
				Fields: []string{
					"useTranslationTargetState", "targetStateValue",
					"editAltTrans", "addAltTrans",
				},
			},
			{
				ID:    "output",
				Label: "Output",
				Fields: []string{
					"addTargetLanguage", "overrideTargetLanguage",
					"allowEmptyTargets", "alwaysAddTargets",
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
			"preserveSpaceByDefault": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Preserve whitespace by default",
				Description: "Treat all content as xml:space=\"preserve\" unless explicitly overridden",
			}),
			"includeExtensions": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Include XLIFF extension elements",
				Description: "Include non-standard namespace extension elements in extracted content",
			}),
			"includeIts": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Include ITS markup",
				Description: "Include ITS (Internationalization Tag Set) markup in extracted content",
			}),
			"ignoreInputSegmentation": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Ignore input segmentation",
				Description: "Ignore seg-source/mrk segmentation from the input XLIFF and treat the full source as a single segment",
			}),
			"fallbackToID": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Use trans-unit ID as name if no resname",
				Description: "Use the trans-unit id attribute as the block name when no resname attribute is present",
			}),
			"forceUniqueIds": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enforce unique trans-unit IDs",
				Description: "Rewrite trans-unit IDs to enforce uniqueness across the file",
			}),

			// States
			"useTranslationTargetState": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Update target state attribute",
				Description: "Update the state attribute on target elements when writing translated XLIFF",
			}),
			"targetStateValue": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Default:     "needs-translation",
				Title:       "Target state value",
				Description: "State value to set on target elements when useTranslationTargetState is enabled",
			}),
			"editAltTrans": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Edit alt-trans elements",
				Description: "Allow editing of existing alt-trans elements",
			}),
			"addAltTrans": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Add alt-trans elements",
				Description: "Allow addition of new alt-trans elements",
			}),

			// Output
			"addTargetLanguage": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Add target-language attribute",
				Description: "Add the target-language attribute to file elements if not already present",
			}),
			"overrideTargetLanguage": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Override target language",
				Description: "Override the target-language attribute in the output document with the pipeline target locale",
			}),
			"allowEmptyTargets": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Allow empty targets",
				Description: "Allow target elements with empty content to be written",
			}),
			"alwaysAddTargets": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Add targets for monolingual XLIFF",
				Description: "Add target elements for monolingual XLIFF files that have no target in the source",
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
