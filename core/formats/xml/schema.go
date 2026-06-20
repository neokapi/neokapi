package xml

import (
	"github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// Schema returns the JSON Schema metadata for the XML format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "XML Format",
		Description: "Configuration for the generic XML format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "xml",
			Extensions: []string{".xml"},
			MimeTypes:  []string{"text/xml", "application/xml"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "parser",
				Label: "Parser Settings",
				Fields: []string{
					"preserveWhitespace",
				},
			},
			{
				ID:    "extraction",
				Label: "Extraction Rules",
				Fields: []string{
					"translatableElements", "translatableAttributes",
					"excludeByDefault", "inlineElements", "excludedElements",
					"preserveWhitespaceElements", "groupElements",
					"idAttributes", "extractNonTranslatableContent",
				},
			},
			{
				ID:    "advanced",
				Label: "Advanced",
				Fields: []string{
					"elements", "attributes", "blockTypeMap",
				},
			},
			{
				ID:    "subfilters",
				Label: "Subfilters",
				Fields: []string{
					"subfilters",
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
				Description: "Preserve original whitespace in text content instead of collapsing it",
			}),

			// Extraction
			"translatableElements": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Translatable elements",
				Description: "Element names whose text content is translatable. If empty, all text content is translatable.",
			}),
			"translatableAttributes": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Translatable attributes",
				Description: "Attribute names that are translatable across all elements",
			}),
			"excludeByDefault": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Exclude by default",
				Description: "Exclude all elements unless explicitly included by an element rule with INCLUDE",
			}),
			"inlineElements": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Inline elements",
				Description: "Element names treated as inline (spans within text) rather than block-level",
			}),
			"excludedElements": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Excluded elements",
				Description: "Element names whose content is excluded from extraction",
			}),
			"preserveWhitespaceElements": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Preserve whitespace elements",
				Description: "Element names that preserve whitespace regardless of the global setting",
			}),
			"groupElements": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Group elements",
				Description: "Element names that produce group/layer boundaries in the output",
			}),
			"idAttributes": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "ID attributes",
				Description: "Attribute names used to extract block IDs from elements",
			}),
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable contextual content such as whitelist-excluded element text and verbatim code/pre subtrees is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep it in skeleton.",
			}),

			// Advanced
			"elements": schema.Prop(coreschema.PropertySchema{
				Type:        "object",
				Title:       "Element rules",
				Description: "Advanced element-specific processing rules with conditions, inline marking, and translatable attribute mappings",
			}),
			"attributes": schema.Prop(coreschema.PropertySchema{
				Type:        "object",
				Title:       "Attribute rules",
				Description: "Advanced attribute-specific processing rules with element scope constraints",
			}),
			"blockTypeMap": schema.Prop(coreschema.PropertySchema{
				Type:        "object",
				Title:       "Block type map",
				Description: "Map of element names to block type strings for semantic type annotation",
			}),

			// Subfilters
			"subfilters": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Subfilters",
				Description: "Array of {pattern, format} mappings for embedded content. Patterns use dot-separated element paths with glob support.",
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
