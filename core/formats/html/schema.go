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
				Fields: []string{"extractNonTranslatableContent", "elements", "attributes"},
			},
			{
				ID:     "inlineCodes",
				Label:  "Inline Codes",
				Fields: []string{"useCodeFinder", "codeFinderRules"},
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
					"preserveWhitespace": {
						PropertySchema: coreschema.PropertySchema{
							Type:        "boolean",
							Default:     false,
							Title:       "Preserve whitespace",
							Description: "Preserve original whitespace in extracted text instead of collapsing it",
						},
						FlattenPath: "preserveWhitespace",
					},
				},
			},
			"extractNonTranslatableContent": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "boolean",
					Default:     true,
					Title:       "Extract non-translatable content",
					Description: "If true (default), renderable non-translatable contextual content -- the <noscript> fallback subtree and JSON data islands (<script type=\"application/ld+json\"|\"application/json\">) -- is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Generic executable <script> and <style> always stay opaque.",
				},
				FlattenPath: "extractNonTranslatableContent",
			},
			"elements": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "object",
					Title:       "Element Rules",
					Description: "Element extraction rules -- maps element names to their rule configuration (ruleTypes, conditions, idAttributes, translatableAttributes)",
				},
				FlattenPath: "elements",
			},
			"attributes": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "object",
					Title:       "Attribute Rules",
					Description: "Global attribute extraction rules -- maps attribute names to their rule configuration (ruleTypes, allElementsExcept, onlyTheseElements, conditions)",
				},
				FlattenPath: "attributes",
			},
			"useCodeFinder": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "boolean",
					Default:     false,
					Title:       "Enable inline code detection",
					Description: "Enable regex-based detection of inline codes (placeholders, variables, tags) within translatable text",
				},
				FlattenPath: "useCodeFinder",
			},
			"codeFinderRules": {
				PropertySchema: coreschema.PropertySchema{
					Type:        "array",
					Title:       "Code finder rules",
					Description: "Regex patterns that match inline codes within translatable text",
					Widget:      "code-finder",
					Visible:     &coreschema.ConditionExpr{Field: "useCodeFinder", Eq: true},
				},
				FlattenPath: "codeFinderRules",
			},
		},
	}
}
