package openxml

import "github.com/neokapi/neokapi/core/format/schema"

// Schema returns the JSON Schema metadata for the OpenXML format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Office Open XML Format",
		Description: "Configuration for the DOCX/PPTX/XLSX format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatSchemaMeta{
			ID:         "openxml",
			Extensions: []string{".docx", ".pptx", ".xlsx"},
			MimeTypes: []string{
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
				"application/vnd.openxmlformats-officedocument.presentationml.presentation",
				"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "common",
				Label: "Common Extraction",
				Fields: []string{
					"translateDocProperties", "translateHiddenText",
					"translateHeadersFooters", "translateFootnotes",
					"translateComments", "translateHyperlinks",
				},
			},
			{
				ID:    "formatting",
				Label: "Formatting Control",
				Fields: []string{
					"aggressiveCleanup", "tabAsCharacter",
				},
			},
			{
				ID:        "pptx",
				Label:     "PowerPoint Options",
				Collapsed: true,
				Fields: []string{
					"translateSlideNotes", "translateSlideMasters",
					"translateHiddenSlides", "translateCharts",
					"translateDiagrams", "includedSlides",
				},
			},
			{
				ID:        "xlsx",
				Label:     "Excel Options",
				Collapsed: true,
				Fields: []string{
					"translateSheetNames", "translateSharedStrings",
					"excludedSheets", "excludedColumns",
				},
			},
			{
				ID:        "styles",
				Label:     "Style & Color Filtering",
				Collapsed: true,
				Fields: []string{
					"excludeColors", "excludeHighlightColors",
					"includeHighlightColors", "excludeStyles", "includeStyles",
				},
			},
			{
				ID:        "inlineCodes",
				Label:     "Inline Codes",
				Collapsed: true,
				Fields: []string{
					"useCodeFinder", "codeFinderRules",
				},
			},
			{
				ID:        "advanced",
				Label:     "Advanced",
				Collapsed: true,
				Fields: []string{
					"complexFieldDefinitionsToExtract", "optimiseWordStyles",
					"fontMappings", "extractRunFontsInfo",
					"replaceLineSeparator", "lineSeparatorReplacement",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			// Common extraction
			"translateDocProperties": {
				Type:        "boolean",
				Default:     true,
				Description: "Extract title, subject, keywords from document properties",
			},
			"translateHiddenText": {
				Type:        "boolean",
				Default:     false,
				Description: "Extract text with the vanish (hidden) property",
			},
			"translateHeadersFooters": {
				Type:        "boolean",
				Default:     true,
				Description: "Extract headers and footers",
			},
			"translateFootnotes": {
				Type:        "boolean",
				Default:     true,
				Description: "Extract footnotes and endnotes",
			},
			"translateComments": {
				Type:        "boolean",
				Default:     false,
				Description: "Extract comment text",
			},
			"translateHyperlinks": {
				Type:        "boolean",
				Default:     true,
				Description: "Extract hyperlink text",
			},
			// Formatting control
			"aggressiveCleanup": {
				Type:        "boolean",
				Default:     true,
				Description: "Strip revision IDs, proofing errors, and other noise before merging runs",
			},
			"tabAsCharacter": {
				Type:        "boolean",
				Default:     false,
				Description: "Treat tab elements as tab characters instead of placeholder spans",
			},
			// PPTX options
			"translateSlideNotes": {
				Type:        "boolean",
				Default:     true,
				Description: "Extract slide notes",
			},
			"translateSlideMasters": {
				Type:        "boolean",
				Default:     false,
				Description: "Extract text from slide masters",
			},
			"translateHiddenSlides": {
				Type:        "boolean",
				Default:     false,
				Description: "Extract content from hidden slides",
			},
			"translateCharts": {
				Type:        "boolean",
				Default:     false,
				Description: "Extract strings from embedded charts",
			},
			"translateDiagrams": {
				Type:        "boolean",
				Default:     false,
				Description: "Extract text from SmartArt diagrams",
			},
			"includedSlides": {
				Type:        "array",
				Description: "If non-empty, only extract these slide numbers (1-based)",
			},
			// XLSX options
			"translateSheetNames": {
				Type:        "boolean",
				Default:     false,
				Description: "Extract sheet names as translatable content",
			},
			"translateSharedStrings": {
				Type:        "boolean",
				Default:     true,
				Description: "Extract shared strings",
			},
			"excludedSheets": {
				Type:        "array",
				Description: "Sheet names to exclude from extraction",
			},
			"excludedColumns": {
				Type:        "array",
				Description: "Column letters to exclude (e.g., \"A\", \"C\", \"AA\")",
			},
			// Style/color filtering
			"excludeColors": {
				Type:        "array",
				Description: "Font colors to exclude (hex RGB, e.g., \"FF0000\" for red)",
			},
			"excludeHighlightColors": {
				Type:        "array",
				Description: "Highlight colors to exclude (e.g., \"yellow\", \"red\")",
			},
			"includeHighlightColors": {
				Type:        "array",
				Description: "If non-empty, only extract text with these highlight colors",
			},
			"excludeStyles": {
				Type:        "array",
				Description: "Paragraph/character style names to exclude",
			},
			"includeStyles": {
				Type:        "array",
				Description: "If non-empty, only extract text with these styles",
			},
			// Code finder
			"useCodeFinder": {
				Type:        "boolean",
				Default:     false,
				Description: "Enable regex-based inline code detection",
			},
			"codeFinderRules": {
				Type:        "array",
				Description: "Regex patterns that match inline codes within translatable text",
			},
			// Advanced
			"complexFieldDefinitionsToExtract": {
				Type:        "array",
				Description: "Field instruction prefixes to extract (e.g., \"HYPERLINK\", \"REF\")",
			},
			"optimiseWordStyles": {
				Type:        "boolean",
				Default:     false,
				Description: "Resolve style inheritance and strip redundant run properties",
			},
			"fontMappings": {
				Type:        "object",
				Description: "Font name to script group mapping (e.g., \"MS Gothic\": \"ja\")",
			},
			"extractRunFontsInfo": {
				Type:        "boolean",
				Default:     false,
				Description: "Emit font metadata as annotations on blocks",
			},
			"replaceLineSeparator": {
				Type:        "boolean",
				Default:     false,
				Description: "Replace Unicode line separator (U+2028) in output",
			},
			"lineSeparatorReplacement": {
				Type:        "string",
				Default:     "\n",
				Description: "Replacement string for line separator characters",
			},
		},
	}
}
