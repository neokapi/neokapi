package json

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the JSON format's parameters.
func (c *Config) Schema() *schema.FilterSchema {
	return &schema.FilterSchema{
		Title:       "JSON Format",
		Description: "Configuration for the native JSON format reader/writer",
		Type:        "object",
		FilterMeta: schema.FilterSchemaMeta{
			ID:         "json",
			Extensions: []string{".json"},
			MimeTypes:  []string{"application/json"},
			Configurations: []schema.FilterConfiguration{
				{
					ConfigID:    "json-i18next",
					Name:        "i18next",
					Description: "i18next JSON resource files with full key paths and HTML subfilter for *_html keys",
					Parameters: map[string]any{
						"useFullKeyPath": true,
						"subfilter":      "html",
						"subfilterRules": "_html$",
					},
				},
				{
					ConfigID:    "json-chrome-extension",
					Name:        "Chrome Extension",
					Description: "Chrome extension messages.json format (extract 'message' keys, 'description' as notes)",
					Parameters: map[string]any{
						"extractAllPairs": false,
						"exceptions":      "^message$",
						"noteRules":       "^description$",
					},
				},
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
			"extractAllPairs": {
				Type:        "boolean",
				Default:     true,
				Description: "Extract all string key-value pairs as translatable blocks",
			},
			"exceptions": {
				Type:        "string",
				Description: "Regex pattern for key names. When extractAllPairs is true, matching keys are excluded. When false, matching keys are included.",
			},
			"extractIsolatedStrings": {
				Type:        "boolean",
				Default:     false,
				Description: "Extract standalone string values inside arrays as translatable blocks",
			},
			"useKeyAsName": {
				Type:        "boolean",
				Default:     true,
				Description: "Use the JSON key as the block name/ID",
			},
			"useFullKeyPath": {
				Type:        "boolean",
				Default:     false,
				Description: "Use hierarchical paths (parent/child) as block names",
			},
			"useLeadingSlashOnKeyPath": {
				Type:        "boolean",
				Default:     true,
				Description: "Prepend / to full key paths",
			},
			"escapeForwardSlashes": {
				Type:        "boolean",
				Default:     true,
				Description: "Escape / as \\/ in JSON output",
			},
			"subfilters": {
				Type:        "array",
				Description: "Array of {pattern, format} mappings for embedded content in specific keys",
			},
			"subfilter": {
				Type:        "string",
				Description: "Global subfilter format name (e.g., 'html') applied to all or matching extracted strings",
			},
			"subfilterRules": {
				Type:        "string",
				Description: "Regex pattern for key names processed by the subfilter. Only used when subfilter is set.",
			},
			"noteRules": {
				Type:        "string",
				Description: "Regex pattern for key names whose values become notes on the next translatable block",
			},
			"idRules": {
				Type:        "string",
				Description: "Regex pattern for key names whose values are used as block names/IDs",
			},
			"useIdStack": {
				Type:        "boolean",
				Default:     false,
				Description: "Stack IDs for nested structures, producing compound IDs",
			},
			"genericMetaRules": {
				Type:        "string",
				Description: "Regex pattern for key names whose values become metadata annotations",
			},
			"extractionRules": {
				Type:        "string",
				Description: "Regex pattern limiting which keys are extracted. Only matching keys are extracted.",
				Widget:      "regexBuilder",
			},
			"maxwidthRules": {
				Type:        "string",
				Description: "Regex pattern for key names whose numeric values set MAX_WIDTH on the next block",
			},
			"maxwidthSizeUnit": {
				Type:        "string",
				Default:     "pixel",
				Description: "Size unit for maxwidth: 'pixel' or 'char'",
			},
			"useCodeFinder": {
				Type:        "boolean",
				Default:     false,
				Description: "Enable regex-based inline code detection",
			},
			"codeFinderRules": {
				Type:        "array",
				Description: "Regex patterns that match inline codes within translatable text",
			},
		},
	}
}
