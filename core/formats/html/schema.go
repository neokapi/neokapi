package html

import (
	"github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// Schema returns the JSON Schema metadata for the HTML format's parameters.
// The schema mirrors the okf_html bridge schema structure so that presets
// and configurations are interchangeable between the native and bridge formats.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "HTML Format",
		Description: "Configuration for the native HTML format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "html",
			Extensions: []string{".html", ".htm"},
			MimeTypes:  []string{"text/html"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:     "parser",
				Label:  "Parser Settings",
				Fields: []string{"parser"},
			},
			{
				ID:     "extraction",
				Label:  "Extraction Rules",
				Fields: []string{"extract_non_translatable_content", "elements", "attributes"},
			},
			{
				ID:     "inlineCodes",
				Label:  "Inline Codes",
				Fields: []string{"use_code_finder", "code_finder_rules"},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"parser": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "object",
					Title:       "Parser behavior",
					Description: "Settings that control how the HTML parser reads input",
				},
				Properties: map[string]schema.PropertySchema{
					"preserve_whitespace": {
						PropertySchema: coreschema.PropertySchema{
							Type:        "boolean",
							Default:     false,
							Title:       "Preserve whitespace",
							Description: "Preserve original whitespace in extracted text instead of collapsing it",
						},
						FlattenPath: "preserve_whitespace",
					},
				},
			},
			"extract_non_translatable_content": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "boolean",
					Default:     true,
					Title:       "Extract non-translatable content",
					Description: "If true (default), renderable non-translatable contextual content -- the <noscript> fallback subtree and JSON data islands (<script type=\"application/ld+json\"|\"application/json\">) -- is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Generic executable <script> and <style> always stay opaque.",
				},
				FlattenPath: "extract_non_translatable_content",
			},
			"elements": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "object",
					Title:       "Element Rules",
					Description: "Element extraction rules -- maps element names to their rule configuration (rule_types, conditions, id_attributes, translatable_attributes)",
				},
				FlattenPath: "elements",
			},
			"attributes": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "object",
					Title:       "Attribute Rules",
					Description: "Global attribute extraction rules -- maps attribute names to their rule configuration (rule_types, all_elements_except, only_these_elements, conditions)",
				},
				FlattenPath: "attributes",
			},
			"use_code_finder": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "boolean",
					Default:     false,
					Title:       "Enable inline code detection",
					Description: "Enable regex-based detection of inline codes (placeholders, variables, tags) within translatable text",
				},
				FlattenPath: "use_code_finder",
			},
			"code_finder_rules": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "array",
					Title:       "Code finder rules",
					Description: "Regex patterns that match inline codes within translatable text",
					Widget:      "code-finder",
					Visible:     &coreschema.ConditionExpr{Field: "use_code_finder", Eq: true},
				},
				FlattenPath: "code_finder_rules",
			},
		},
	}
}
