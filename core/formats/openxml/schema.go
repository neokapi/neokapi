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
					"translate_doc_properties", "translate_hidden_text",
					"translate_headers_footers", "translate_footnotes",
					"translate_comments", "translate_hyperlinks",
					"automatically_accept_revisions",
				},
			},
			{
				ID:    "excel",
				Label: "Excel Options",
				Fields: []string{
					"translate_sheet_names", "translate_shared_strings",
					"excluded_sheets", "excluded_columns",
				},
			},
			{
				ID:    "powerpoint",
				Label: "PowerPoint Options",
				Fields: []string{
					"translate_slide_notes", "translate_slide_masters",
					"translate_hidden_slides", "translate_charts",
					"translate_diagrams", "included_slides",
				},
			},
			{
				ID:    "general",
				Label: "General",
				Fields: []string{
					"aggressive_cleanup", "tab_as_character",
					"ignore_soft_hyphen_tag", "replace_no_break_hyphen_tag",
					"extract_non_translatable_content",
				},
			},
			{
				ID:        "styles",
				Label:     "Style & Color Filtering",
				Collapsed: true,
				Fields: []string{
					"exclude_colors", "exclude_highlight_colors",
					"include_highlight_colors", "exclude_styles", "include_styles",
				},
			},
			{
				ID:        "inlineCodes",
				Label:     "Inline Codes",
				Collapsed: true,
				Fields: []string{
					"use_code_finder", "code_finder_rules",
				},
			},
			{
				ID:        "advanced",
				Label:     "Advanced",
				Collapsed: true,
				Fields: []string{
					"complex_field_definitions_to_extract",
					"font_mappings", "extract_run_fonts_info",
					"replace_line_separator", "line_separator_replacement",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			// Word options
			"translate_doc_properties": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate document properties",
				Default:     true,
				Description: "Extract title, subject, keywords from document properties.",
			}),
			"translate_hidden_text": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate hidden text",
				Default:     false,
				Description: "Extract text with the vanish (hidden) property in Word documents.",
			}),
			"translate_headers_footers": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate headers and footers",
				Default:     true,
				Description: "Extract text from headers and footers in Word documents.",
			}),
			"translate_footnotes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate footnotes and endnotes",
				Default:     true,
				Description: "Extract footnotes and endnotes from Word documents.",
			}),
			"translate_comments": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate comments",
				Default:     false,
				Description: "Extract comment text from Word documents.",
			}),
			"translate_hyperlinks": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract external hyperlinks",
				Default:     true,
				Description: "Extract hyperlink text for translation.",
			}),
			"automatically_accept_revisions": schema.Prop(coreschema.PropertySchema{
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
			"translate_sheet_names": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate sheet names",
				Default:     false,
				Description: "Extract sheet names in Excel workbooks.",
			}),
			"translate_shared_strings": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate shared strings",
				Default:     true,
				Description: "Extract shared strings from Excel workbooks.",
			}),
			"excluded_sheets": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Excluded sheets",
				Description: "Sheet names to exclude from extraction.",
			}),
			"excluded_columns": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Excluded columns",
				Description: "Column letters to exclude (e.g., \"A\", \"C\", \"AA\").",
			}),
			// PowerPoint options
			"translate_slide_notes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate slide notes",
				Default:     true,
				Description: "Extract speaker notes in PowerPoint presentations.",
			}),
			"translate_slide_masters": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate master slides",
				Default:     false,
				Description: "Extract text from slide masters in PowerPoint.",
			}),
			"translate_hidden_slides": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate hidden slides",
				Default:     false,
				Description: "Extract content from hidden slides in PowerPoint.",
			}),
			"translate_charts": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate chart text",
				Default:     false,
				Description: "Extract strings from embedded charts.",
			}),
			"translate_diagrams": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Translate diagram data",
				Default:     false,
				Description: "Extract text from SmartArt diagrams.",
			}),
			"included_slides": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Included slide numbers",
				Description: "If non-empty, only extract these slide numbers (1-based).",
			}),
			// General options
			"aggressive_cleanup": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Aggressive code cleanup",
				Default:     true,
				Description: "Strip revision IDs, proofing errors, and other noise before merging runs.",
			}),
			"tab_as_character": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Add tab as character",
				Default:     false,
				Description: "Treat tab elements as tab characters instead of placeholder spans.",
			}),
			"ignore_soft_hyphen_tag": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Ignore soft hyphen tags",
				Default:     false,
				Description: "Ignore soft hyphen tags in the document.",
			}),
			"replace_no_break_hyphen_tag": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Replace no-break hyphen tags",
				Default:     false,
				Description: "Replace no-break hyphen tags with the non-breaking hyphen character.",
			}),
			"extract_non_translatable_content": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract non-translatable content",
				Default:     true,
				Description: "If true (default), non-translatable contextual content is surfaced: image/shape alt-text (descr/title on docPr/cNvPr) as content blocks (visible to ingestion/LLM consumers, skipped by machine translation), and PowerPoint/Excel comment text as data parts, instead of being hidden in opaque skeleton. Disable to keep it opaque.",
			}),
			// Style/color filtering
			"exclude_colors": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Exclude specific font colors",
				Description: "Font colors to exclude (hex RGB, e.g., \"FF0000\" for red).",
			}),
			"exclude_highlight_colors": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Exclude highlight colors",
				Description: "Highlight colors to exclude (e.g., \"yellow\", \"red\").",
			}),
			"include_highlight_colors": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Include highlight colors",
				Description: "If non-empty, only extract text with these highlight colors.",
			}),
			"exclude_styles": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Exclude styles",
				Description: "Paragraph/character style names to exclude.",
			}),
			"include_styles": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Include styles",
				Description: "If non-empty, only extract text with these styles.",
			}),
			// Inline codes
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
			// Advanced
			"complex_field_definitions_to_extract": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Complex field definitions to extract",
				Description: "Field instruction prefixes to extract (e.g., \"HYPERLINK\", \"REF\").",
			}),
			"font_mappings": schema.Prop(coreschema.PropertySchema{
				Type:        "object",
				Title:       "Font mappings",
				Description: "Font name to script group mapping (e.g., \"MS Gothic\": \"ja\").",
			}),
			"extract_run_fonts_info": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract run fonts info",
				Default:     false,
				Description: "Emit font metadata as annotations on blocks.",
			}),
			"replace_line_separator": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Add line separator as character",
				Default:     false,
				Description: "Replace Unicode line separator (U+2028) in output.",
			}),
			"line_separator_replacement": schema.Prop(coreschema.PropertySchema{
				Type:        "string",
				Title:       "Line separator replacement string",
				Default:     "\n",
				Description: "Replacement string for line separator characters.",
				Visible:     &coreschema.ConditionExpr{Field: "replace_line_separator", Eq: true},
			}),
		},
	}
}
