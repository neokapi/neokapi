package openxml

import (
	"github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// Schema returns the JSON Schema metadata for the OpenXML format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "OpenXML Filter",
		Description: "Configuration for the DOCX/PPTX/XLSX format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
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
				ID:    "word",
				Label: "Word Options",
				Fields: []string{
					"translateDocProperties", "translateHiddenText",
					"translateHeadersFooters", "translateFootnotes",
					"translateComments", "translateHyperlinks",
					"automaticallyAcceptRevisions",
				},
			},
			{
				ID:    "excel",
				Label: "Excel Options",
				Fields: []string{
					"translateSheetNames", "translateSharedStrings",
					"excludedSheets", "excludedColumns",
				},
			},
			{
				ID:    "powerpoint",
				Label: "PowerPoint Options",
				Fields: []string{
					"translateSlideNotes", "translateSlideMasters",
					"translateHiddenSlides", "translateCharts",
					"translateDiagrams", "includedSlides",
				},
			},
			{
				ID:    "general",
				Label: "General",
				Fields: []string{
					"aggressiveCleanup", "tabAsCharacter",
					"ignoreSoftHyphenTag", "replaceNoBreakHyphenTag",
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
					"complexFieldDefinitionsToExtract",
					"fontMappings", "extractRunFontsInfo",
					"replaceLineSeparator", "lineSeparatorReplacement",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			// Word options
			"translateDocProperties": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate document properties",
				Default:     true,
				Description: "Extract title, subject, keywords from document properties.",
			}),
			"translateHiddenText": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate hidden text",
				Default:     false,
				Description: "Extract text with the vanish (hidden) property in Word documents.",
			}),
			"translateHeadersFooters": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate headers and footers",
				Default:     true,
				Description: "Extract text from headers and footers in Word documents.",
			}),
			"translateFootnotes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate footnotes and endnotes",
				Default:     true,
				Description: "Extract footnotes and endnotes from Word documents.",
			}),
			"translateComments": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate comments",
				Default:     false,
				Description: "Extract comment text from Word documents.",
			}),
			"translateHyperlinks": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract external hyperlinks",
				Default:     true,
				Description: "Extract hyperlink text for translation.",
			}),
			"automaticallyAcceptRevisions": schema.Prop(coreschema.PropertySchema{
				Type:    "boolean",
				Title:   "Accept revisions automatically",
				Default: true,
				Description: "Automatically accept tracked changes before extraction. " +
					"When true (default, matching Okapi), inserted runs are kept and " +
					"deleted runs are dropped; rows marked with <w:trPr><w:del/> " +
					"(ECMA-376 §17.13.5.13) are removed entirely; rows marked with " +
					"<w:trPr><w:ins/> (§17.13.5.16) are kept.",
			}),
			// Excel options
			"translateSheetNames": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate sheet names",
				Default:     false,
				Description: "Extract sheet names in Excel workbooks.",
			}),
			"translateSharedStrings": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate shared strings",
				Default:     true,
				Description: "Extract shared strings from Excel workbooks.",
			}),
			"excludedSheets": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Excluded sheets",
				Description: "Sheet names to exclude from extraction.",
			}),
			"excludedColumns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Excluded columns",
				Description: "Column letters to exclude (e.g., \"A\", \"C\", \"AA\").",
			}),
			// PowerPoint options
			"translateSlideNotes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate slide notes",
				Default:     true,
				Description: "Extract speaker notes in PowerPoint presentations.",
			}),
			"translateSlideMasters": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate master slides",
				Default:     false,
				Description: "Extract text from slide masters in PowerPoint.",
			}),
			"translateHiddenSlides": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate hidden slides",
				Default:     false,
				Description: "Extract content from hidden slides in PowerPoint.",
			}),
			"translateCharts": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate chart text",
				Default:     false,
				Description: "Extract strings from embedded charts.",
			}),
			"translateDiagrams": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate diagram data",
				Default:     false,
				Description: "Extract text from SmartArt diagrams.",
			}),
			"includedSlides": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Included slide numbers",
				Description: "If non-empty, only extract these slide numbers (1-based).",
			}),
			// General options
			"aggressiveCleanup": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Aggressive code cleanup",
				Default:     true,
				Description: "Strip revision IDs, proofing errors, and other noise before merging runs.",
			}),
			"tabAsCharacter": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Add tab as character",
				Default:     false,
				Description: "Treat tab elements as tab characters instead of placeholder spans.",
			}),
			"ignoreSoftHyphenTag": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Ignore soft hyphen tags",
				Default:     false,
				Description: "Ignore soft hyphen tags in the document.",
			}),
			"replaceNoBreakHyphenTag": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Replace no-break hyphen tags",
				Default:     false,
				Description: "Replace no-break hyphen tags with the non-breaking hyphen character.",
			}),
			// Style/color filtering
			"excludeColors": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Exclude specific font colors",
				Description: "Font colors to exclude (hex RGB, e.g., \"FF0000\" for red).",
			}),
			"excludeHighlightColors": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Exclude highlight colors",
				Description: "Highlight colors to exclude (e.g., \"yellow\", \"red\").",
			}),
			"includeHighlightColors": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Include highlight colors",
				Description: "If non-empty, only extract text with these highlight colors.",
			}),
			"excludeStyles": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Exclude styles",
				Description: "Paragraph/character style names to exclude.",
			}),
			"includeStyles": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Include styles",
				Description: "If non-empty, only extract text with these styles.",
			}),
			// Inline codes
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
			// Advanced
			"complexFieldDefinitionsToExtract": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Complex field definitions to extract",
				Description: "Field instruction prefixes to extract (e.g., \"HYPERLINK\", \"REF\").",
			}),
			"fontMappings": schema.Prop(coreschema.PropertySchema{
				Type:        "object",
				Title:       "Font mappings",
				Description: "Font name to script group mapping (e.g., \"MS Gothic\": \"ja\").",
			}),
			"extractRunFontsInfo": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract run fonts info",
				Default:     false,
				Description: "Emit font metadata as annotations on blocks.",
			}),
			"replaceLineSeparator": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Add line separator as character",
				Default:     false,
				Description: "Replace Unicode line separator (U+2028) in output.",
			}),
			"lineSeparatorReplacement": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Line separator replacement string",
				Default:     "\n",
				Description: "Replacement string for line separator characters.",
				Visible:     &coreschema.ConditionExpr{Field: "replaceLineSeparator", Eq: true},
			}),
		},
	}
}
