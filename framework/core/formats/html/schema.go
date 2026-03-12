package html

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the HTML format's parameters.
// The schema mirrors the okf_html bridge schema structure so that presets
// and configurations are interchangeable between the native and bridge formats.
func (c *Config) Schema() *schema.FilterSchema {
	return &schema.FilterSchema{
		Title:       "HTML Format",
		Description: "Configuration for the native HTML format reader/writer",
		Type:        "object",
		FilterMeta: schema.FilterSchemaMeta{
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
				Fields: []string{"elements", "attributes"},
			},
			{
				ID:     "inlineCodes",
				Label:  "Inline Codes",
				Fields: []string{"useCodeFinder", "codeFinderRules"},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"parser": {
				Type:        "object",
				Description: "Parser settings",
				Properties: map[string]schema.PropertySchema{
					"preserveWhitespace": {
						Type:        "boolean",
						Default:     false,
						Description: "Preserve significant whitespace in text nodes",
						FlattenPath: "preserveWhitespace",
					},
				},
			},
			"elements": {
				Type:        "object",
				Description: "Map of element names to extraction rules (ruleTypes, conditions, idAttributes, translatableAttributes)",
				FlattenPath: "elements",
			},
			"attributes": {
				Type:        "object",
				Description: "Map of attribute names to extraction rules (ruleTypes, allElementsExcept, onlyTheseElements, conditions)",
				FlattenPath: "attributes",
			},
			"useCodeFinder": {
				Type:        "boolean",
				Default:     false,
				Description: "Enable regex-based inline code detection",
				FlattenPath: "useCodeFinder",
			},
			"codeFinderRules": {
				Type:        "array",
				Description: "Regex patterns that match inline codes within translatable text",
				FlattenPath: "codeFinderRules",
			},
		},
	}
}
