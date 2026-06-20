package rtf

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the RTF format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "RTF Format",
		Description: "Configuration for the Rich Text Format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "rtf",
			Extensions: []string{".rtf"},
			MimeTypes:  []string{"application/rtf", "text/rtf"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Content Extraction",
				Fields: []string{
					"extractNonTranslatableContent",
					"extractHeadersFooters",
					"extractAnnotations",
					"extractBookmarks",
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
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable contextual content — headers/footers, \\info title/doccomm metadata, and \\xe/\\tc index/TOC entries — is surfaced as role-tagged content blocks (visible to ingestion/LLM consumers, skipped by machine translation) and \\annotation review comments ride as note metadata. Disable to keep all of it in skeleton.",
			}),
			"extractHeadersFooters": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate Headers and Footers",
				Description: "If true, text in \\header* and \\footer* destinations is extracted as translatable content. If false (default), it is surfaced as non-translatable content when extractNonTranslatableContent is on, otherwise kept in skeleton.",
			}),
			"extractAnnotations": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate Annotations",
				Description: "If true, text in \\annotation (review comment) destinations is extracted as translatable content. If false (default), the comment text rides as block note metadata when extractNonTranslatableContent is on.",
			}),
			"extractBookmarks": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate Bookmarks",
				Description: "If true, \\bkmkstart / \\bkmkend bookmark labels are extracted as translatable text. Off by default — bookmarks are structural anchors, not user-visible copy.",
			}),
			"useCodeFinder": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Enable Inline Code Detection",
				Description: "Enable regex-based inline code detection within extracted text.",
			}),
			"codeFinderRules": schema.Prop(coreschema.PropertySchema{
				Type:        "array",
				Title:       "Code Finder Rules",
				Description: "Regex patterns that match inline codes within extracted text.",
				Widget:      "code-finder",
				Visible:     &coreschema.ConditionExpr{Field: "useCodeFinder", Eq: true},
			}),
		},
	}
}
