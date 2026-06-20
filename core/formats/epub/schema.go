package epub

import (
	coreschema "github.com/neokapi/neokapi/core/schema"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Schema returns the JSON Schema metadata for the EPUB format's parameters.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "EPUB Format",
		Description: "Configuration for the EPUB e-book format reader/writer",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "epub",
			Extensions: []string{".epub"},
			MimeTypes:  []string{"application/epub+zip"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Content Extraction",
				Fields: []string{
					"extractNonTranslatableContent",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Default:     true,
				Title:       "Extract non-translatable content",
				Description: "If true (default), non-translatable contextual content such as verbatim code listings (<pre>/<code>/<kbd>/<samp>) in the direct-XHTML fallback extractor is surfaced as content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep it in skeleton.",
			}),
		},
	}
}
