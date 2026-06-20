package odf

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the ODF format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Open Document Format",
		Description: "Configuration for the OpenDocument (ODT/ODS/ODP/ODG) format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "odf",
			Extensions: []string{".odt", ".ods", ".odp", ".odg", ".odf"},
			MimeTypes: []string{
				"application/vnd.oasis.opendocument.text",
				"application/vnd.oasis.opendocument.spreadsheet",
				"application/vnd.oasis.opendocument.presentation",
			},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Content Extraction",
				Fields: []string{
					"translateNotes", "translateHiddenContent",
					"extractNonTranslatableContent",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"translateNotes": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Translate Presentation Notes",
				Description: "If true, presentation speaker notes are extracted as translatable content.",
			}),
			"translateHiddenContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     false,
				Title:       "Translate Hidden Content",
				Description: "If true, hidden content is extracted as translatable content.",
			}),
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable contextual content — image accessibility text (svg:title/svg:desc alt-text and long descriptions) and form-control display strings (form:label/form:title/form:help-text) — is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep it in skeleton.",
			}),
		},
	}
}
